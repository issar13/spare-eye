package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"spareeye/internal/host"
)

func TestRecycleOldestClipsStopsOnceEnoughSpaceIsFree(t *testing.T) {
	service := newTestService(t)
	settings := service.Settings()
	videoDirectory := filepath.Join(settings.MediaDirectory, "videos")
	now := time.Now()

	for index, name := range []string{"a-oldest.mp4", "b-older.mp4", "c-newer.mp4", "d-newest.mp4"} {
		path := filepath.Join(videoDirectory, name)
		if err := os.WriteFile(path, []byte("clip"), 0o600); err != nil {
			t.Fatalf("writing %s returned %v", name, err)
		}
		modified := now.Add(time.Duration(index-10) * time.Hour)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatalf("setting the time on %s returned %v", name, err)
		}
	}

	service.mutex.Lock()
	service.settings.Storage.MinimumFreeGigabytes = 5
	service.mutex.Unlock()

	// Space stays short until two clips have gone.
	service.readHost = func(string) host.Reading {
		remaining, err := os.ReadDir(videoDirectory)
		if err != nil {
			t.Fatalf("reading the video directory returned %v", err)
		}
		if len(remaining) > 2 {
			return host.Reading{FreeBytes: 1 << 30}
		}
		return host.Reading{FreeBytes: 6 << 30}
	}

	service.recycleOldestClips()

	remaining, err := os.ReadDir(videoDirectory)
	if err != nil {
		t.Fatalf("reading the video directory returned %v", err)
	}
	if len(remaining) != 2 || remaining[0].Name() != "c-newer.mp4" || remaining[1].Name() != "d-newest.mp4" {
		t.Fatalf("remaining clips = %v, want the two newest", remaining)
	}

	history := service.Events()
	if len(history) != 2 || history[0].Kind != eventClipRecycled {
		t.Fatalf("history = %+v, want two recycling entries", history)
	}
}

func TestRecycleOldestClipsGivesUpWhenNothingCanBeRemoved(t *testing.T) {
	service := newTestService(t)
	service.readHost = func(string) host.Reading { return host.Reading{FreeBytes: 1 << 20} }

	// An empty video directory would loop forever if the lack of a removable
	// clip were not treated as the end; the test finishing is the assertion.
	service.recycleOldestClips()

	if len(service.Events()) != 0 {
		t.Fatalf("history = %+v, want nothing recorded", service.Events())
	}
}

func TestRecycleOldestClipsTreatsAnUnreadableDriveAsNotLow(t *testing.T) {
	service := newTestService(t)
	settings := service.Settings()
	clip := filepath.Join(settings.MediaDirectory, "videos", "only.mp4")
	if err := os.WriteFile(clip, []byte("clip"), 0o600); err != nil {
		t.Fatalf("writing the clip returned %v", err)
	}

	service.readHost = func(string) host.Reading { return host.Reading{FreeBytes: 0} }
	service.recycleOldestClips()

	if _, err := os.Stat(clip); err != nil {
		t.Fatalf("the clip was removed on an unreadable drive: %v", err)
	}
}
