// Package media lists and prunes the snapshots and clips on disk.
package media

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Recording is one file on disk, described for display.
type Recording struct {
	Path      string    `json:"path"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
	Manual    bool      `json:"manual"`

	// Protected recordings are starred by the user and never pruned.
	Protected bool `json:"protected"`
}

// SizeMegabytes is a display helper for the UI and message listings.
func (r Recording) SizeMegabytes() float64 {
	return float64(r.SizeBytes) / (1024 * 1024)
}

const posterDirectoryName = ".posters"

// PosterPath is where the poster image for a clip lives. Posters sit in a
// hidden subdirectory because listings read only the top level of the media
// directory, so they never appear as recordings of their own. The clip's full
// name, extension included, is kept so two clips can never share a poster.
func PosterPath(clipPath string) string {
	return filepath.Join(PosterDirectory(filepath.Dir(clipPath)), filepath.Base(clipPath)+".jpg")
}

// PosterDirectory is the folder holding the posters of the clips in videoDirectory.
func PosterDirectory(videoDirectory string) string {
	return filepath.Join(videoDirectory, posterDirectoryName)
}

// ListVideos returns clips newest first. The order is part of the contract, not
// a convenience: the Telegram listing is numbered and the user replies with a
// number, so "1" has to be the most recent clip. Filesystem order is not.
//
// protected is the starred set from LoadProtected. It is a parameter rather
// than read here because clips and stills live in subdirectories of the folder
// that holds protected.json, and a listing should not guess where that is.
func ListVideos(directory string, protected map[string]bool) ([]Recording, error) {
	return list(directory, ".mp4", protected)
}

// ListSnapshots returns still images newest first.
func ListSnapshots(directory string, protected map[string]bool) ([]Recording, error) {
	return list(directory, ".jpg", protected)
}

func list(directory, extension string, protected map[string]bool) ([]Recording, error) {
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
			Protected: protected[entry.Name()],
		})
	}

	sort.Slice(recordings, func(first, second int) bool {
		return recordings[first].CreatedAt.After(recordings[second].CreatedAt)
	})

	return recordings, nil
}

// Prune deletes unprotected recordings in directory older than retentionDays
// and returns how many were removed. A retention of zero or less keeps
// everything. Starred recordings are kept along with their posters.
//
// mediaDirectory is where protected.json lives. Prune reads it itself, under
// protectedMutex, so a star set while pruning is either seen or lands after
// the sweep, never lost between a caller's read and the deletion.
func Prune(directory string, retentionDays int, mediaDirectory string) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}

	protectedMutex.Lock()
	defer protectedMutex.Unlock()

	protected, err := loadProtected(mediaDirectory)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	removed := 0

	for _, extension := range []string{".mp4", ".jpg"} {
		recordings, err := list(directory, extension, protected)
		if err != nil {
			return removed, err
		}

		for _, recording := range recordings {
			if recording.Protected || recording.CreatedAt.After(cutoff) {
				continue
			}
			if err := os.Remove(recording.Path); err != nil {
				return removed, fmt.Errorf("removing %s: %w", recording.Path, err)
			}
			removed++

			if extension == ".mp4" {
				if err := os.Remove(PosterPath(recording.Path)); err != nil && !os.IsNotExist(err) {
					return removed, fmt.Errorf("removing poster for %s: %w", recording.Path, err)
				}
			}
		}
	}

	return removed, nil
}

const protectedFileName = "protected.json"

// protectedMutex serialises every read-modify-write of protected.json. The
// webview can star a clip while the startup prune is sweeping, and both touch
// the same file; one package-level lock is enough for a file this small.
var protectedMutex sync.Mutex

// LoadProtected returns the names of starred recordings. A missing file means
// nothing has been starred yet.
func LoadProtected(mediaDirectory string) (map[string]bool, error) {
	protectedMutex.Lock()
	defer protectedMutex.Unlock()

	return loadProtected(mediaDirectory)
}

// SetProtected stars or unstars one recording by file name. Names are kept
// instead of paths so the list survives the media directory being moved; a
// clip and a snapshot never share a name because their extensions differ.
//
// Stale names are removed by DeleteRecording calling this with false. Names
// left behind by files deleted outside the app are harmless: names carry a
// timestamp (manual snapshots include milliseconds), so a name is never reused
// for a different file.
func SetProtected(mediaDirectory, name string, protected bool) error {
	protectedMutex.Lock()
	defer protectedMutex.Unlock()

	current, err := loadProtected(mediaDirectory)
	if err != nil {
		return err
	}

	if protected {
		current[name] = true
	} else {
		delete(current, name)
	}

	names := make([]string, 0, len(current))
	for protectedName := range current {
		names = append(names, protectedName)
	}
	sort.Strings(names)

	encoded, err := json.MarshalIndent(names, "", "  ")
	if err != nil {
		return err
	}

	// Write to a temporary file and rename, so an interrupted save cannot
	// leave a truncated list that would silently unstar everything.
	path := filepath.Join(mediaDirectory, protectedFileName)
	temporary := path + ".tmp"

	if err := os.MkdirAll(mediaDirectory, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", mediaDirectory, err)
	}
	if err := os.WriteFile(temporary, encoded, 0o600); err != nil {
		return fmt.Errorf("writing protected list: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("replacing protected list: %w", err)
	}

	return nil
}

func loadProtected(mediaDirectory string) (map[string]bool, error) {
	encoded, err := os.ReadFile(filepath.Join(mediaDirectory, protectedFileName))
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading protected list: %w", err)
	}

	var names []string
	if err := json.Unmarshal(encoded, &names); err != nil {
		return nil, fmt.Errorf("parsing protected list: %w", err)
	}

	protected := make(map[string]bool, len(names))
	for _, name := range names {
		protected[name] = true
	}

	return protected, nil
}
