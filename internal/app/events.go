package app

import (
	"path/filepath"
	"sync"
	"time"

	"kaa-rada/internal/detector"
)

// retainedEvents bounds the in-memory history. It is a display convenience, not
// an audit trail — the log file on disk is the durable record.
const retainedEvents = 200

// Event kinds as the UI knows them. They are strings rather than the detector's
// enum because the history also carries things the detector never sees, such as
// whether an alert actually reached Telegram.
const (
	eventCameraStarted = "camera-started"
	eventMotion        = "motion-detected"
	eventMotionEnded   = "motion-ended"
	eventPhotoSaved    = "photo-saved"
	eventClipStarted   = "recording-started"
	eventClipSaved     = "recording-saved"
	eventAlertSent     = "alert-sent"
	eventAlertFailed   = "alert-failed"
	eventTrouble       = "trouble"
)

// EventView is one thing that happened, written as a sentence.
//
// The detector's Event carries an enum, a file path and a Go error, none of
// which mean anything to somebody watching their front door. Message is the
// story; the remaining fields are for the UI to group and link entries.
type EventView struct {
	Kind      string    `json:"kind"`
	At        time.Time `json:"at"`
	Message   string    `json:"message"`
	MediaName string    `json:"mediaName"`
	MediaPath string    `json:"mediaPath"`
	Night     bool      `json:"night"`
}

type eventHistory struct {
	limit int

	mutex sync.Mutex
	held  []EventView
}

// record appends an already-worded entry. The messenger uses it to report
// deliveries, which the detector knows nothing about.
func (h *eventHistory) record(view EventView) {
	if view.At.IsZero() {
		view.At = time.Now()
	}

	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.held = append(h.held, view)
	if len(h.held) > h.limit {
		h.held = h.held[len(h.held)-h.limit:]
	}
}

func (h *eventHistory) add(event detector.Event) {
	kind, message := describeDetectorEvent(event)
	if kind == "" {
		return
	}

	view := EventView{
		Kind:      kind,
		At:        event.At,
		Message:   message,
		MediaPath: event.MediaPath,
		Night:     event.Night,
	}
	if event.MediaPath != "" {
		view.MediaName = filepath.Base(event.MediaPath)
	}

	h.record(view)
}

// describeDetectorEvent turns one engine event into a sentence, or returns an
// empty kind for the ones not worth telling the user about.
func describeDetectorEvent(event detector.Event) (string, string) {
	switch event.Kind {
	case detector.EventCameraOpened:
		return eventCameraStarted, "Camera started"

	case detector.EventMotionStarted:
		if event.Night {
			return eventMotion, "Motion detected (night)"
		}
		return eventMotion, "Motion detected"

	case detector.EventMotionStopped:
		return eventMotionEnded, "Everything went still again"

	case detector.EventSnapshotSaved:
		return eventPhotoSaved, "Saved a photo"

	case detector.EventClipStarted:
		if event.Manual {
			return eventClipStarted, "Started recording, because you asked"
		}
		return eventClipStarted, "Started recording"

	case detector.EventClipFinished:
		if event.Err != nil {
			return eventTrouble, "A recording could not be saved. Check there is free space on this computer."
		}
		if event.Manual {
			return eventClipSaved, "Saved your recording"
		}
		// Dropped frames mean a shorter, choppier clip rather than a lost one,
		// so it is worth mentioning but not worth alarming anybody over.
		if event.Detail != "" {
			return eventClipSaved, "Saved a video clip, though this computer struggled to keep up"
		}
		return eventClipSaved, "Saved a video clip"

	case detector.EventFailure:
		return eventTrouble, "Something went wrong while watching. If it keeps happening, restart the app."
	}

	return "", ""
}

// view returns the history newest first, which is the order the UI lists it in.
func (h *eventHistory) view() []EventView {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	reversed := make([]EventView, 0, len(h.held))
	for index := len(h.held) - 1; index >= 0; index-- {
		reversed = append(reversed, h.held[index])
	}

	return reversed
}
