// Command motion-detector runs the detector headlessly, without the desktop UI.
//
// It exists so the engine can be confirmed working on its own, and it doubles
// as the way to run the detector on a machine with no display.
//
//	go run ./cmd/motiond -list-cameras
//	go run ./cmd/motiond
//	go run ./cmd/motiond -camera 1 -no-telegram
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"motion-detector/internal/config"
	"motion-detector/internal/detector"
	"motion-detector/internal/media"
	"motion-detector/internal/notify"
)

func main() {
	cameraIndex := flag.Int("camera", -1, "camera index; -1 picks the first usable device")
	listCameras := flag.Bool("list-cameras", false, "list usable cameras and exit")
	disableTelegram := flag.Bool("no-telegram", false, "run detection without sending alerts")
	flag.Parse()

	if *listCameras {
		reportCameras()
		return
	}

	if err := run(*cameraIndex, *disableTelegram); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func reportCameras() {
	cameras := detector.AvailableCameras()
	if len(cameras) == 0 {
		fmt.Println("No usable cameras found.")
		fmt.Println("A device node can exist without being able to capture — virtual")
		fmt.Println("webcams such as Iriun or OBS appear even when not streaming.")
		return
	}

	fmt.Printf("Usable cameras (%d):\n", len(cameras))
	for _, camera := range cameras {
		fmt.Printf("  index %d — %dx%d\n", camera.Index, camera.Width, camera.Height)
	}
}

func run(requestedCamera int, disableTelegram bool) error {
	settings, err := config.Load()
	if err != nil {
		return err
	}

	applyEnvironmentCredentials(&settings)

	if requestedCamera >= 0 {
		settings.Detection.CameraIndex = requestedCamera
	} else {
		resolved, found := detector.FirstUsableCamera()
		if !found {
			// Falling through to the default index here would fail later with
			// an opaque backend error instead of naming the actual problem.
			return fmt.Errorf(
				"no usable camera found\n" +
					"  • another program may already hold the camera — a previous run of this\n" +
					"    tool keeps the device until it exits\n" +
					"  • virtual webcams (Iriun, OBS) present a device node without streaming\n" +
					"  • run with -list-cameras to see what was detected",
			)
		}
		settings.Detection.CameraIndex = resolved
	}

	fmt.Printf("Media directory: %s\n", settings.Storage.MediaDirectory)
	fmt.Printf("Camera index:    %d\n", settings.Detection.CameraIndex)

	// Both directories are pruned. Passing only the video directory left
	// snapshots accumulating forever, which on a busy camera is the larger
	// share of the files even though each one is smaller.
	prunedTotal := 0
	for _, directory := range []string{settings.VideoDirectory(), settings.SnapshotDirectory()} {
		removed, err := media.Prune(directory, settings.Storage.RetentionDays)
		if err != nil {
			fmt.Printf("Retention pruning failed for %s: %v\n", directory, err)
			continue
		}
		prunedTotal += removed
	}
	if prunedTotal > 0 {
		fmt.Printf("Pruned %d files older than %d days\n", prunedTotal, settings.Storage.RetentionDays)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	engine := detector.New(settings)

	var notifier notify.Notifier
	if !disableTelegram && settings.ConfiguredForAlerts() {
		telegram := notify.NewTelegram(settings.Telegram.BotToken, settings.Telegram.ChatID)
		if err := telegram.Verify(ctx); err != nil {
			fmt.Printf("Telegram unavailable, continuing without alerts: %v\n", err)
		} else {
			fmt.Println("Telegram connected")
			notifier = telegram
			defer telegram.Close()

			go forwardCommands(ctx, telegram, engine, settings)
		}
	} else if !disableTelegram {
		fmt.Println("Telegram not configured; running detection only.")
		fmt.Println("Set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID to enable alerts.")
	}

	go reportEvents(ctx, engine, notifier)

	fmt.Println("Watching for motion. Press Ctrl+C to stop.")

	return engine.Run(ctx)
}

// applyEnvironmentCredentials lets credentials come from the environment when
// config.json has none, so an existing .env still works via:
//
//	set -a; source .env; set +a
func applyEnvironmentCredentials(settings *config.Config) {
	if settings.Telegram.BotToken == "" {
		settings.Telegram.BotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	}
	if settings.Telegram.ChatID == "" {
		settings.Telegram.ChatID = strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	}
}

// reportEvents prints every detector event and forwards the interesting ones to
// the notifier.
func reportEvents(ctx context.Context, engine *detector.Detector, notifier notify.Notifier) {
	for event := range engine.Events() {
		stamp := event.At.Format("15:04:05")

		switch event.Kind {
		case detector.EventCameraOpened:
			fmt.Printf("[%s] camera opened: %s\n", stamp, event.Detail)

		case detector.EventMotionStarted:
			fmt.Printf("[%s] motion started (night=%v)\n", stamp, event.Night)

		case detector.EventMotionStopped:
			fmt.Printf("[%s] motion stopped\n", stamp)

		case detector.EventSnapshotSaved:
			fmt.Printf("[%s] snapshot %s\n", stamp, event.MediaPath)
			deliver(ctx, notifier, describeMotion(event.Night), event.MediaPath, notify.AttachmentImage)

		case detector.EventClipStarted:
			fmt.Printf("[%s] recording started: %s\n", stamp, event.MediaPath)

		case detector.EventClipFinished:
			if event.Err != nil {
				fmt.Printf("[%s] recording failed: %v\n", stamp, event.Err)
				continue
			}
			if event.Detail != "" {
				fmt.Printf("[%s] %s\n", stamp, event.Detail)
			}
			fmt.Printf("[%s] recording finished: %s\n", stamp, event.MediaPath)

			caption := "🎥 Sustained motion recorded"
			if event.Manual {
				caption = "📹 Manual recording finished"
			}
			deliver(ctx, notifier, caption, event.MediaPath, notify.AttachmentVideo)

		case detector.EventFailure:
			fmt.Printf("[%s] failure: %v\n", stamp, event.Err)
		}
	}
}

func describeMotion(night bool) string {
	if night {
		return "🌙 Night motion"
	}
	return "☀️ Day motion"
}

func deliver(ctx context.Context, notifier notify.Notifier, caption, path string, kind notify.AttachmentKind) {
	if notifier == nil {
		return
	}

	// Sending happens on its own goroutine: an upload must never delay the
	// event reader, which would let the detector's event buffer overflow.
	go func() {
		attachment := &notify.Attachment{Path: path, Kind: kind}
		if err := notifier.Send(ctx, caption, attachment); err != nil {
			fmt.Printf("sending %s failed: %v\n", path, err)
		}
	}()
}

// forwardCommands drives the detector from messages the user sends their bot.
func forwardCommands(ctx context.Context, notifier notify.Notifier, engine *detector.Detector, settings config.Config) {
	commands, err := notifier.Commands(ctx)
	if err != nil {
		fmt.Printf("command polling unavailable: %v\n", err)
		return
	}

	// Recordings offered in the most recent listing, so a numeric reply can be
	// resolved back to a file.
	var offered []media.Recording

	for incoming := range commands {
		switch {
		case incoming.Command == notify.CommandStartRecording:
			fmt.Println("command: start recording")
			engine.StartManualRecording()
			notifier.Send(ctx, "📹 Recording started. Send /stop to end.", nil)

		case incoming.Command == notify.CommandStopRecording:
			fmt.Println("command: stop recording")
			engine.StopManualRecording()

		case incoming.Command == notify.CommandListRecordings:
			fmt.Println("command: list recordings")
			recordings, err := media.ListVideos(settings.VideoDirectory())
			if err != nil {
				notifier.Send(ctx, fmt.Sprintf("⚠️ Could not list recordings: %v", err), nil)
				continue
			}
			offered = recordings
			notifier.Send(ctx, formatListing(recordings), nil)

		case incoming.Selection != nil:
			index := incoming.Selection.Index
			if index < 1 || index > len(offered) {
				notifier.Send(ctx, fmt.Sprintf("⚠️ Choose a number between 1 and %d", len(offered)), nil)
				continue
			}

			chosen := offered[index-1]
			fmt.Printf("command: send recording %s\n", chosen.Name)

			attachment := &notify.Attachment{Path: chosen.Path, Kind: notify.AttachmentVideo}
			if err := notifier.Send(ctx, chosen.Name, attachment); err != nil {
				notifier.Send(ctx, fmt.Sprintf("❌ Could not send %s: %v", chosen.Name, err), nil)
			}
		}
	}
}

func formatListing(recordings []media.Recording) string {
	if len(recordings) == 0 {
		return "⚠️ No recordings yet"
	}

	var listing strings.Builder
	listing.WriteString("📹 Recordings, newest first:\n\n")

	for index, recording := range recordings {
		listing.WriteString(fmt.Sprintf(
			"%d. %s\n   %s (%.1f MB)\n\n",
			index+1,
			recording.Name,
			recording.CreatedAt.Format(time.DateTime),
			recording.SizeMegabytes(),
		))
	}

	listing.WriteString(fmt.Sprintf("Reply with a number from 1 to %d to receive it.", len(recordings)))

	return listing.String()
}
