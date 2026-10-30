package app

import "spareeye/internal/desktopnotify"

// deliverDesktopAlert shows a notification from the operating system for a
// saved snapshot, when the user has not switched that off.
//
// It runs on its own goroutine for the same reason Telegram uploads do: the
// event reader must never wait on something outside the process. A failure is
// logged and nothing more, because the snapshot is already on disk and an
// alert that could not be shown is not worth interrupting the camera for.
func (s *Service) deliverDesktopAlert(snapshotPath string, night bool) {
	s.mutex.Lock()
	enabled := s.settings.Notifications.Desktop
	s.mutex.Unlock()

	if !enabled {
		return
	}

	title := "Motion detected"
	if night {
		title = "Motion detected (night)"
	}

	go func() {
		// Notification daemons are outside our control, and a panic on this
		// goroutine would end the process rather than just the alert.
		defer func() {
			if panicked := recover(); panicked != nil {
				s.log.Panicked("showing a desktop notification", panicked)
			}
		}()

		message := desktopnotify.Message{
			Title:     title,
			Body:      "SpareEye saved a photo on this computer.",
			ImagePath: snapshotPath,
		}
		if err := s.showDesktopNotification(message); err != nil {
			s.log.Warn("desktop notification failed", "error", err)
		}
	}()
}
