package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"spareeye/internal/app"
	"spareeye/internal/detector"
	"spareeye/internal/onvif"
)

// App is the surface bound into the webview. It holds no state of its own: the
// service layer owns the detector, the notifier, and the settings, and this
// type exists only to choose which of that is reachable from JavaScript.
type App struct {
	service *app.Service

	// windowContext is handed over by Wails at startup and is what the runtime
	// package needs to act on the window.
	windowContext context.Context
}

func NewApp(service *app.Service) *App {
	return &App{service: service}
}

func (a *App) startup(windowContext context.Context) {
	a.windowContext = windowContext
	a.service.Startup(windowContext)
}

func (a *App) shutdown(windowContext context.Context) {
	a.service.Shutdown(windowContext)
}

// Settings returns the saved configuration for the Setup and tuning screens.
func (a *App) Settings() app.SettingsView { return a.service.Settings() }

// Status is polled by the arm/disarm screen.
func (a *App) Status() app.StatusView { return a.service.Status() }

// PreviewFrame returns the latest camera picture for display in the webview.
func (a *App) PreviewFrame() string {
	frame := a.service.PreviewJPEG()
	if len(frame) == 0 {
		return ""
	}

	return base64.StdEncoding.EncodeToString(frame)
}

// Cameras lists capture devices confirmed to deliver frames.
func (a *App) Cameras() []detector.Camera { return a.service.Cameras() }

// RescanCameras looks for devices again, on the user's explicit request.
func (a *App) RescanCameras() []detector.Camera { return a.service.RescanCameras() }

// DiscoverNetworkCameras finds ONVIF devices on the local network.
func (a *App) DiscoverNetworkCameras() ([]onvif.Camera, error) {
	return onvif.DiscoverCameras()
}

// Every action returns an ActionResult rather than an error. A rejected Wails
// promise carries only the Go error's text, and that text is written for a log,
// not for the person who has to decide what to do next.
func (a *App) Arm() app.ActionResult    { return a.service.Arm() }
func (a *App) Disarm() app.ActionResult { return a.service.Disarm() }

func (a *App) SaveTelegram(form app.TelegramForm) app.ActionResult {
	return a.service.SaveTelegram(form)
}

func (a *App) TestTelegram(form app.TelegramForm) app.ActionResult {
	return a.service.TestTelegram(form)
}

func (a *App) SelectCamera(index int) app.ActionResult { return a.service.SelectCamera(index) }

func (a *App) TestCameraURL(address string) app.ActionResult {
	return a.service.TestCameraURL(address)
}

func (a *App) SelectCameraURL(address string) app.ActionResult {
	return a.service.SelectCameraURL(address)
}

func (a *App) SaveCameraLocation(name string) app.ActionResult {
	return a.service.SaveCameraLocation(name)
}

func (a *App) SaveTuning(form app.TuningForm) app.ActionResult { return a.service.SaveTuning(form) }

func (a *App) SaveLiveView(form app.LiveViewForm) app.ActionResult {
	return a.service.SaveLiveView(form)
}

func (a *App) SaveStartup(form app.StartupForm) app.ActionResult {
	return a.service.SaveStartup(form)
}

func (a *App) SaveNotifications(form app.NotificationsForm) app.ActionResult {
	return a.service.SaveNotifications(form)
}

func (a *App) StartManualRecording() app.ActionResult { return a.service.StartManualRecording() }
func (a *App) StopManualRecording() app.ActionResult  { return a.service.StopManualRecording() }
func (a *App) TakeSnapshot() app.ActionResult         { return a.service.TakeSnapshot() }

func (a *App) Clips() ([]app.RecordingView, error)     { return a.service.Clips() }
func (a *App) Snapshots() ([]app.RecordingView, error) { return a.service.Snapshots() }

func (a *App) DeleteRecording(path string) app.ActionResult { return a.service.DeleteRecording(path) }
func (a *App) SendRecording(path string) app.ActionResult   { return a.service.SendRecording(path) }

func (a *App) SetProtected(path string, protected bool) app.ActionResult {
	return a.service.SetProtected(path, protected)
}

// RecentEvents returns the detector's event history, newest first.
func (a *App) RecentEvents() []app.EventView { return a.service.Events() }

// LogLines returns recent log records. A windowed app has no terminal, so this
// is the only way a user sees them without hunting for the file.
func (a *App) LogLines() []string { return a.service.LogLines() }

// OpenExternally hands a recording to the desktop's default player.
//
// Clips are written with the mp4v codec, which WebKit does not decode, so the
// in-app player cannot be the only way to watch one. The path is checked
// against the media directories, since opening runs whatever program the OS
// associates with the file.
func (a *App) OpenExternally(path string) app.ActionResult {
	resolved, err := a.service.OpenTarget(path)
	if err != nil {
		return app.NotARecording()
	}

	runtime.BrowserOpenURL(a.windowContext, (&url.URL{Scheme: "file", Path: resolved}).String())

	return app.ActionResult{OK: true, Headline: "Opening in your video player"}
}

// OpenRecordingsFolder opens the media directory in the file manager. It takes
// no argument: the webview never names the folder, so it cannot be steered
// anywhere but the one the settings point at.
func (a *App) OpenRecordingsFolder() app.ActionResult {
	folder, err := a.service.RecordingsFolder()
	if err != nil {
		return app.ActionResult{
			Headline: "Your recordings folder could not be opened",
			NextStep: "Check that the folder in Settings exists and that SpareEye may write to it.",
		}
	}

	runtime.BrowserOpenURL(a.windowContext, (&url.URL{Scheme: "file", Path: folder}).String())

	return app.ActionResult{OK: true, Headline: "Opening your recordings folder"}
}

// ExportRecordings saves one tab's recordings as a zip archive the user places
// with a save dialog. It takes only the tab's name: the file list is built by
// the service and the destination chosen in the native dialog, so the webview
// never names a path. It runs synchronously; a few hundred megabytes take
// seconds, and the button shows a pending state meanwhile.
//
// Cancelling the dialog is not an error. It returns OK with the headline
// "Export cancelled" so the UI can show a neutral confirmation, not a red one.
func (a *App) ExportRecordings(collection string) app.ActionResult {
	paths, err := a.service.ExportPaths(collection)
	if err != nil {
		return app.ActionResult{
			Headline: "Your recordings could not be listed",
			NextStep: "Check that the recordings folder in Settings exists, then try again.",
		}
	}

	if len(paths) == 0 {
		return app.ActionResult{
			Headline: "Nothing to export",
			NextStep: "This tab has no recordings yet.",
		}
	}

	destination, err := runtime.SaveFileDialog(a.windowContext, runtime.SaveDialogOptions{
		DefaultFilename: fmt.Sprintf("spareeye-%s-%s.zip", collection, time.Now().Format("2006-01-02")),
		Title:           "Export recordings",
		Filters:         []runtime.FileFilter{{DisplayName: "Zip archive (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil {
		return app.ActionResult{
			Headline: "The save dialog could not be opened",
			NextStep: "Try again, or copy the files from the recordings folder by hand.",
		}
	}

	if destination == "" {
		return app.ActionResult{OK: true, Headline: "Export cancelled"}
	}

	// Some platforms' save dialogs return exactly what was typed, so a name
	// without the extension would produce a zip nothing opens on double-click.
	if !strings.EqualFold(filepath.Ext(destination), ".zip") {
		destination += ".zip"
	}

	if err := a.service.WriteExportArchive(paths, destination); err != nil {
		return app.ActionResult{
			Headline: "The recordings could not be exported",
			NextStep: "Check there is enough free space at that location and that it is writable, then try again.",
		}
	}

	return app.ActionResult{
		OK:       true,
		Headline: fmt.Sprintf("Exported %d recordings to %s", len(paths), filepath.Base(destination)),
	}
}
