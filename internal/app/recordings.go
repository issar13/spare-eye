package app

import (
	"fmt"
	"os"
	"path/filepath"

	"spareeye/internal/media"
	"spareeye/internal/notify"
)

// RecordingView is one file on disk, described for the browser screen. It adds
// the URL the webview can load and the size the UI displays, so the frontend
// does no arithmetic on bytes.
type RecordingView struct {
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	URL           string  `json:"url"`
	SizeMegabytes float64 `json:"sizeMegabytes"`
	CreatedAt     string  `json:"createdAt"`
	Manual        bool    `json:"manual"`
	Video         bool    `json:"video"`
}

// Clips returns recorded video, newest first.
func (s *Service) Clips() ([]RecordingView, error) {
	s.mutex.Lock()
	directory := s.settings.VideoDirectory()
	s.mutex.Unlock()

	recordings, err := media.ListVideos(directory)
	if err != nil {
		return nil, err
	}

	return recordingViews(recordings, videoRoute, true), nil
}

// Snapshots returns still images, newest first.
func (s *Service) Snapshots() ([]RecordingView, error) {
	s.mutex.Lock()
	directory := s.settings.SnapshotDirectory()
	s.mutex.Unlock()

	recordings, err := media.ListSnapshots(directory)
	if err != nil {
		return nil, err
	}

	return recordingViews(recordings, snapshotRoute, false), nil
}

func recordingViews(recordings []media.Recording, route string, video bool) []RecordingView {
	views := make([]RecordingView, 0, len(recordings))

	for _, recording := range recordings {
		views = append(views, RecordingView{
			Name:          recording.Name,
			Path:          recording.Path,
			URL:           route + recording.Name,
			SizeMegabytes: recording.SizeMegabytes(),
			CreatedAt:     recording.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Manual:        recording.Manual,
			Video:         video,
		})
	}

	return views
}

// DeleteRecording removes one file. Deleting is irreversible, so the path is
// checked against the media directories rather than trusted.
func (s *Service) DeleteRecording(path string) ActionResult {
	resolved, _, err := s.resolveMedia(path)
	if err != nil {
		return notARecording()
	}

	if err := os.Remove(resolved); err != nil {
		s.log.Error("deleting recording failed", "path", resolved, "error", err)
		return failed(
			"That recording could not be deleted",
			"It may already be gone, or another program may have it open. Refresh the list and try again.",
		)
	}

	s.log.Info("recording deleted", "path", resolved)

	return succeeded("Deleted " + filepath.Base(resolved))
}

// SendRecording uploads one file to Telegram on the user's explicit request.
func (s *Service) SendRecording(path string) ActionResult {
	resolved, kind, err := s.resolveMedia(path)
	if err != nil {
		return notARecording()
	}

	if !s.messenger.connected() {
		return failed(
			"Telegram is not connected",
			"Go to Setup, enter your bot token and chat ID, and press Test connection.",
		).withDoc(telegramSetupDoc)
	}

	s.log.Info("sending recording to telegram", "path", resolved)

	name := filepath.Base(resolved)
	s.deliver(name, name, &notify.Attachment{Path: resolved, Kind: kind})

	// The upload runs in the background, so this reports that it started. The
	// activity list gets the outcome when it lands.
	return succeeded("Sending " + name + " to Telegram…")
}

// notARecording covers a path that is not in the media folders. A user cannot
// reach this through the UI, so the wording only has to be non-alarming.
func notARecording() ActionResult {
	return failed(
		"That file is not one of your recordings",
		"Press Refresh to reload the list.",
	)
}

// resolveMedia confirms a path names a file directly inside one of the media
// directories. The webview is the only caller, but it renders paths that came
// from disk, and the delete and upload actions must not be steerable anywhere
// else on the filesystem.
func (s *Service) resolveMedia(path string) (string, notify.AttachmentKind, error) {
	s.mutex.Lock()
	snapshots := s.settings.SnapshotDirectory()
	videos := s.settings.VideoDirectory()
	s.mutex.Unlock()

	cleaned := filepath.Clean(path)

	switch filepath.Dir(cleaned) {
	case snapshots:
		return cleaned, notify.AttachmentImage, nil
	case videos:
		return cleaned, notify.AttachmentVideo, nil
	}

	return "", 0, fmt.Errorf("%s is not a recording in the media directory", path)
}
