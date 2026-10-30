package app

import (
	"context"
	"fmt"
	"time"

	"spareeye/internal/media"
)

// cleanupInterval is how often a running app checks the drive. Footage can
// fill a disk in hours, so waiting for the next launch is too slow, while the
// check itself is cheap enough that every ten minutes costs nothing.
const cleanupInterval = 10 * time.Minute

const bytesPerGigabyte = 1 << 30

// startCleanupLoop runs pruneAndRecycle on a timer until stopCleanupLoop.
func (s *Service) startCleanupLoop() {
	cleanupContext, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})

	s.mutex.Lock()
	s.stopCleanup = cancel
	s.cleanupFinished = finished
	s.mutex.Unlock()

	go func() {
		defer close(finished)

		// A panic here would end the process under an open window; a missed
		// cleanup is far cheaper.
		defer func() {
			if panicked := recover(); panicked != nil {
				s.log.Panicked("cleaning up old recordings", panicked)
			}
		}()

		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cleanupContext.Done():
				return
			case <-ticker.C:
				s.pruneAndRecycle()
			}
		}
	}()
}

func (s *Service) stopCleanupLoop() {
	s.mutex.Lock()
	cancel, finished := s.stopCleanup, s.cleanupFinished
	s.stopCleanup, s.cleanupFinished = nil, nil
	s.mutex.Unlock()

	if cancel == nil {
		return
	}

	cancel()
	<-finished
}

// pruneAndRecycle applies the retention period, then makes room if the drive
// is still short of the configured floor.
func (s *Service) pruneAndRecycle() {
	s.pruneExpiredMedia()
	s.recycleOldestClips()
}

// recycleOldestClips deletes the oldest unstarred clips one at a time until the
// drive has the configured free space again or none are left to delete.
func (s *Service) recycleOldestClips() {
	s.mutex.Lock()
	settings := s.settings
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

		removedName, err := media.RemoveOldestUnprotectedVideo(settings.VideoDirectory(), settings.Storage.MediaDirectory)
		if err != nil {
			s.log.Error("recycling a clip failed", "error", err)
			return
		}
		if removedName == "" {
			s.log.Warn("free space is low but every remaining clip is starred", "freeBytes", freeBytes)
			return
		}

		s.log.Info("removed the oldest clip to free space", "clip", removedName, "freeBytes", freeBytes)
		s.events.record(EventView{
			Kind:    eventClipRecycled,
			Message: fmt.Sprintf("Removed %s to free space", removedName),
		})
	}
}
