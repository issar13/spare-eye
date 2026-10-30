// Package media lists and prunes the snapshots and clips on disk.
package media

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Recording is one file on disk, described for display.
type Recording struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
	Manual    bool      `json:"manual"`
}

// SizeMegabytes is a display helper for the UI and message listings.
func (r Recording) SizeMegabytes() float64 {
	return float64(r.SizeBytes) / (1024 * 1024)
}

// ListVideos returns clips newest first. The order is part of the contract, not
// a convenience: the Telegram listing is numbered and the user replies with a
// number, so "1" has to be the most recent clip. Filesystem order is not.
func ListVideos(directory string) ([]Recording, error) {
	return list(directory, ".mp4")
}

// ListSnapshots returns still images newest first.
func ListSnapshots(directory string) ([]Recording, error) {
	return list(directory, ".jpg")
}

func list(directory, extension string) ([]Recording, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", directory, err)
	}

	recordings := make([]Recording, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), extension) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			// The file disappeared between listing and stat; skip it rather
			// than failing the whole listing.
			continue
		}

		recordings = append(recordings, Recording{
			Path:      filepath.Join(directory, entry.Name()),
			Name:      entry.Name(),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime(),
			Manual:    strings.HasPrefix(entry.Name(), "manual_"),
		})
	}

	sort.Slice(recordings, func(first, second int) bool {
		return recordings[first].CreatedAt.After(recordings[second].CreatedAt)
	})

	return recordings, nil
}

const bytesPerGigabyte = 1024 * 1024 * 1024

// PruneToSizeCap deletes the oldest recordings across all directories, clips
// and snapshots together, until their combined size fits maxGigabytes, and
// returns how many were removed. A cap of zero or less keeps everything.
//
// It is meant to run before capture starts, as Prune is. Nothing is being
// written then, so no file can be deleted mid-recording; calling it while a
// clip is open would need that clip's path excluded.
func PruneToSizeCap(directories []string, maxGigabytes float64) (int, error) {
	if maxGigabytes <= 0 {
		return 0, nil
	}

	recordings := make([]Recording, 0)
	for _, directory := range directories {
		for _, extension := range []string{".mp4", ".jpg"} {
			listed, err := list(directory, extension)
			if err != nil {
				return 0, err
			}
			recordings = append(recordings, listed...)
		}
	}

	sort.Slice(recordings, func(first, second int) bool {
		return recordings[first].CreatedAt.Before(recordings[second].CreatedAt)
	})

	totalBytes := int64(0)
	for _, recording := range recordings {
		totalBytes += recording.SizeBytes
	}

	capBytes := int64(maxGigabytes * bytesPerGigabyte)
	removed := 0

	for _, recording := range recordings {
		if totalBytes <= capBytes {
			break
		}
		if err := os.Remove(recording.Path); err != nil {
			return removed, fmt.Errorf("removing %s: %w", recording.Path, err)
		}
		totalBytes -= recording.SizeBytes
		removed++
	}

	return removed, nil
}

// Prune deletes recordings older than retentionDays and returns how many were
// removed. A retention of zero or less keeps everything.
func Prune(directory string, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	removed := 0

	for _, extension := range []string{".mp4", ".jpg"} {
		recordings, err := list(directory, extension)
		if err != nil {
			return removed, err
		}

		for _, recording := range recordings {
			if recording.CreatedAt.After(cutoff) {
				continue
			}
			if err := os.Remove(recording.Path); err != nil {
				return removed, fmt.Errorf("removing %s: %w", recording.Path, err)
			}
			removed++
		}
	}

	return removed, nil
}
