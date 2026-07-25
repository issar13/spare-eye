package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"kaa-rada/internal/media"
	"kaa-rada/internal/notify"
)

// messenger holds the connected notifier, if there is one. It is a separate
// concern from the detector: credentials can change while the camera runs, and
// the camera can run with no notifier at all.
type messenger struct {
	mutex     sync.Mutex
	telegram  *notify.Telegram
	cancel    context.CancelFunc
	context   context.Context
	reachable bool

	// offered is the listing most recently sent to the phone, so a numeric
	// reply can be resolved back to a file.
	offered []media.Recording
}

func (m *messenger) connected() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.reachable
}

// restartMessenger reconnects the notifier to whatever the current settings
// say. Verifying happens off the caller's goroutine because it makes a network
// round trip, and the UI thread must not wait on Telegram.
func (s *Service) restartMessenger() {
	s.messenger.stop()

	s.mutex.Lock()
	settings := s.settings
	s.mutex.Unlock()

	if !settings.ConfiguredForAlerts() {
		s.log.Info("alerts disabled or unconfigured; running detection only")
		return
	}

	messengerContext, cancel := context.WithCancel(context.Background())
	telegram := notify.NewTelegram(settings.Telegram.BotToken, settings.Telegram.ChatID)

	s.messenger.mutex.Lock()
	s.messenger.telegram = telegram
	s.messenger.cancel = cancel
	s.messenger.context = messengerContext
	s.messenger.mutex.Unlock()

	go func() {
		verifyContext, done := context.WithTimeout(messengerContext, verifyTimeout)
		defer done()

		if err := telegram.Verify(verifyContext); err != nil {
			s.log.Error("telegram unreachable; alerts will not be sent", "error", err)
			return
		}

		s.messenger.mutex.Lock()
		stillCurrent := s.messenger.telegram == telegram
		if stillCurrent {
			s.messenger.reachable = true
		}
		s.messenger.mutex.Unlock()

		if !stillCurrent {
			return
		}

		s.log.Info("telegram connected")
		s.followCommands(messengerContext, telegram)
	}()
}

func (m *messenger) stop() {
	m.mutex.Lock()
	cancel := m.cancel
	telegram := m.telegram

	m.cancel = nil
	m.telegram = nil
	m.context = nil
	m.reachable = false
	m.mutex.Unlock()

	if cancel != nil {
		cancel()
	}
	if telegram != nil {
		telegram.Close()
	}
}

// send delivers a message on its own goroutine. An upload must never delay the
// event reader, which would let the detector's event buffer overflow.
//
// The outcome is written into the event history so "Sent to Telegram" appears
// in the activity list beside the recording it belongs to. Without it a failed
// upload is silent, and the user believes an alert went out when it did not.
func (s *Service) deliver(description string, caption string, attachment *notify.Attachment) {
	s.messenger.mutex.Lock()
	telegram := s.messenger.telegram
	messengerContext := s.messenger.context
	s.messenger.mutex.Unlock()

	if telegram == nil {
		return
	}

	go func() {
		if err := telegram.Send(messengerContext, caption, attachment); err != nil {
			s.log.Error("sending to telegram failed", "description", description, "error", err)
			s.events.record(EventView{
				Kind:    eventAlertFailed,
				Message: "Could not send " + description + " to Telegram. It is still saved on this computer.",
			})
			return
		}

		s.events.record(EventView{Kind: eventAlertSent, Message: "Sent " + description + " to Telegram"})
	}()
}

func (s *Service) deliverSnapshot(path string, night bool) {
	caption := "☀️ Day motion"
	if night {
		caption = "🌙 Night motion"
	}

	s.deliver("the photo", caption, &notify.Attachment{Path: path, Kind: notify.AttachmentImage})
}

func (s *Service) deliverClip(path string, manual bool) {
	caption := "🎥 Sustained motion recorded"
	if manual {
		caption = "📹 Manual recording finished"
	}

	s.deliver("the recording", caption, &notify.Attachment{Path: path, Kind: notify.AttachmentVideo})
}

// followCommands drives the detector from messages the user sends their bot,
// mirroring what the headless CLI offers so the two builds behave alike.
func (s *Service) followCommands(messengerContext context.Context, telegram *notify.Telegram) {
	commands, err := telegram.Commands(messengerContext)
	if err != nil {
		s.log.Error("command polling unavailable", "error", err)
		return
	}

	for incoming := range commands {
		switch {
		case incoming.Command == notify.CommandStartRecording:
			s.log.Info("telegram command", "command", "record")
			if result := s.StartManualRecording(); !result.OK {
				telegram.Send(messengerContext, "⚠️ "+result.Headline+" "+result.NextStep, nil)
				continue
			}
			telegram.Send(messengerContext, "📹 Recording started. Send /stop to end.", nil)

		case incoming.Command == notify.CommandStopRecording:
			s.log.Info("telegram command", "command", "stop")
			if result := s.StopManualRecording(); !result.OK {
				telegram.Send(messengerContext, "⚠️ "+result.Headline+" "+result.NextStep, nil)
			}

		case incoming.Command == notify.CommandListRecordings:
			s.log.Info("telegram command", "command", "list")
			s.offerListing(messengerContext, telegram)

		case incoming.Selection != nil:
			s.sendSelection(messengerContext, telegram, incoming.Selection.Index)
		}
	}
}

func (s *Service) offerListing(messengerContext context.Context, telegram *notify.Telegram) {
	s.mutex.Lock()
	directory := s.settings.VideoDirectory()
	s.mutex.Unlock()

	recordings, err := media.ListVideos(directory)
	if err != nil {
		telegram.Send(messengerContext, fmt.Sprintf("⚠️ Could not list recordings: %v", err), nil)
		return
	}

	s.messenger.mutex.Lock()
	s.messenger.offered = recordings
	s.messenger.mutex.Unlock()

	telegram.Send(messengerContext, formatListing(recordings), nil)
}

func (s *Service) sendSelection(messengerContext context.Context, telegram *notify.Telegram, index int) {
	s.messenger.mutex.Lock()
	offered := s.messenger.offered
	s.messenger.mutex.Unlock()

	if index < 1 || index > len(offered) {
		telegram.Send(messengerContext, fmt.Sprintf("⚠️ Choose a number between 1 and %d", len(offered)), nil)
		return
	}

	chosen := offered[index-1]
	s.log.Info("telegram command", "command", "send", "name", chosen.Name)

	attachment := &notify.Attachment{Path: chosen.Path, Kind: notify.AttachmentVideo}
	if err := telegram.Send(messengerContext, chosen.Name, attachment); err != nil {
		telegram.Send(messengerContext, fmt.Sprintf("❌ Could not send %s: %v", chosen.Name, err), nil)
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
