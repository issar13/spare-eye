package app

import (
	"fmt"
	"slices"

	"spareeye/internal/media"
)

const bytesPerGigabyte = 1 << 30

// recycleOldestClips deletes the oldest clips one at a time until the drive has
// the configured free space again or none are left to delete. Starred clips and
// clips still being written are never candidates.
func (s *Service) recycleOldestClips() {
	s.mutex.Lock()
	settings := s.settings
	openClips := slices.Clone(s.openClips)
	s.mutex.Unlock()

	if settings.Storage.MinimumFreeGigabytes <= 0 {
		return
	}

	floorBytes := uint64(settings.Storage.MinimumFreeGigabytes) * bytesPerGigabyte

	// Each pass deletes a file or returns, so this cannot spin forever.
	for {
		freeBytes := s.readHost(settings.Storage.MediaDirectory).FreeBytes

		// A reading of zero means the free space could not be read, not that
		// the drive is full. Acting on it would delete every clip on a
		// platform that cannot answer.
		if freeBytes == 0 || freeBytes >= floorBytes {
			return
		}

		removedName, err := media.RemoveOldestUnprotectedVideo(settings.VideoDirectory(), settings.Storage.MediaDirectory, openClips)
		if err != nil {
			s.log.Error("recycling a clip failed", "error", err)
			return
		}
		if removedName == "" {
			s.log.Warn("free space is low but every remaining clip is starred or still being written", "freeBytes", freeBytes)
			return
		}

		s.log.Info("removed the oldest clip to free space", "clip", removedName, "freeBytes", freeBytes)
		s.events.record(EventView{
			Kind:    eventClipRecycled,
			Message: fmt.Sprintf("Removed %s to free space", removedName),
		})
	}
}
