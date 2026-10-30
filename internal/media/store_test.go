package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeRecording creates a file of the given size and stamps its modification
// time, which is what Recording.CreatedAt reports. Tests set the time
// explicitly rather than relying on creation order, because the filesystem
// timestamp resolution is coarse enough that files written in a loop can share
// a modification time.
func writeRecording(t *testing.T, directory, name string, sizeBytes int, modified time.Time) string {
	t.Helper()

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, make([]byte, sizeBytes), 0o600); err != nil {
		t.Fatalf("writing %s returned %v", name, err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatalf("setting the time on %s returned %v", name, err)
	}

	return path
}

func recordingNames(recordings []Recording) []string {
	names := make([]string, 0, len(recordings))
	for _, recording := range recordings {
		names = append(names, recording.Name)
	}
	return names
}

func assertNamesInOrder(t *testing.T, recordings []Recording, expected ...string) {
	t.Helper()

	actual := recordingNames(recordings)
	if len(actual) != len(expected) {
		t.Fatalf("listing returned %v, want %v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("listing returned %v, want %v", actual, expected)
		}
	}
}

// TestListVideosReturnsNewestFirst is the regression test for the previous Go
// build, which globbed the directory, returned entries in filesystem order and
// carried a comment saying the sort was unimplemented. The consequence was that
// "recording 1" in the Telegram listing was not reliably the newest clip, so a
// user asking for the latest footage could be sent something days old.
func TestListVideosReturnsNewestFirst(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()

	// Written in an order deliberately unrelated to their timestamps.
	writeRecording(t, directory, "middle.mp4", 10, now.Add(-2*time.Hour))
	writeRecording(t, directory, "oldest.mp4", 10, now.Add(-48*time.Hour))
	writeRecording(t, directory, "newest.mp4", 10, now.Add(-1*time.Minute))

	recordings, err := ListVideos(directory)
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}

	assertNamesInOrder(t, recordings, "newest.mp4", "middle.mp4", "oldest.mp4")
}

func TestListSnapshotsReturnsNewestFirst(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()

	writeRecording(t, directory, "second.jpg", 10, now.Add(-30*time.Minute))
	writeRecording(t, directory, "first.jpg", 10, now.Add(-1*time.Minute))
	writeRecording(t, directory, "third.jpg", 10, now.Add(-90*time.Minute))

	recordings, err := ListSnapshots(directory)
	if err != nil {
		t.Fatalf("ListSnapshots returned %v", err)
	}

	assertNamesInOrder(t, recordings, "first.jpg", "second.jpg", "third.jpg")
}

func TestListVideosReturnsOnlyClips(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()

	writeRecording(t, directory, "clip.mp4", 10, now)
	// Uppercase extensions are accepted on purpose: strings.EqualFold means a
	// clip written by a tool that upper-cases the extension still shows up.
	writeRecording(t, directory, "shouting.MP4", 10, now.Add(-time.Minute))
	writeRecording(t, directory, "still.jpg", 10, now.Add(-2*time.Minute))
	writeRecording(t, directory, "notes.txt", 10, now.Add(-3*time.Minute))
	writeRecording(t, directory, "partial.mp4.tmp", 10, now.Add(-4*time.Minute))

	recordings, err := ListVideos(directory)
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}

	assertNamesInOrder(t, recordings, "clip.mp4", "shouting.MP4")
}

func TestListSnapshotsReturnsOnlyStillImages(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()

	writeRecording(t, directory, "still.jpg", 10, now)
	writeRecording(t, directory, "shouting.JPG", 10, now.Add(-time.Minute))
	writeRecording(t, directory, "clip.mp4", 10, now.Add(-2*time.Minute))
	// .jpeg is a different extension and is not matched; the app only ever
	// writes .jpg.
	writeRecording(t, directory, "other.jpeg", 10, now.Add(-3*time.Minute))

	recordings, err := ListSnapshots(directory)
	if err != nil {
		t.Fatalf("ListSnapshots returned %v", err)
	}

	assertNamesInOrder(t, recordings, "still.jpg", "shouting.JPG")
}

func TestListSkipsDirectories(t *testing.T) {
	// The media directory holds the snapshots and videos subdirectories, and a
	// directory named like a clip must not be offered as a playable recording.
	directory := t.TempDir()

	if err := os.Mkdir(filepath.Join(directory, "archive.mp4"), 0o700); err != nil {
		t.Fatalf("creating the directory returned %v", err)
	}
	writeRecording(t, directory, "clip.mp4", 10, time.Now())

	recordings, err := ListVideos(directory)
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}

	assertNamesInOrder(t, recordings, "clip.mp4")
}

func TestListOfAMissingDirectoryIsEmptyRatherThanAnError(t *testing.T) {
	// The UI lists recordings before the camera has ever run, so a directory
	// that does not exist yet is a normal state, not a failure to report.
	missing := filepath.Join(t.TempDir(), "never-created")

	videos, err := ListVideos(missing)
	if err != nil {
		t.Fatalf("ListVideos returned %v, want no error", err)
	}
	if videos != nil {
		t.Errorf("ListVideos returned %v, want nil", videos)
	}

	snapshots, err := ListSnapshots(missing)
	if err != nil {
		t.Fatalf("ListSnapshots returned %v, want no error", err)
	}
	if snapshots != nil {
		t.Errorf("ListSnapshots returned %v, want nil", snapshots)
	}
}

func TestListOfAnEmptyDirectoryIsEmpty(t *testing.T) {
	recordings, err := ListVideos(t.TempDir())
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}
	if len(recordings) != 0 {
		t.Errorf("ListVideos returned %v, want nothing", recordingNames(recordings))
	}
}

// TestManualFlagFollowsTheFilenamePrefix covers how the UI distinguishes a clip
// the user asked for from one motion triggered; the only signal is the prefix
// the recorder writes.
func TestManualFlagFollowsTheFilenamePrefix(t *testing.T) {
	cases := []struct {
		name           string
		expectedManual bool
	}{
		{"manual_20260725_120000.mp4", true},
		{"manual_.mp4", true},
		{"motion_20260725_120000.mp4", false},
		{"manual.mp4", false},
		{"a_manual_clip.mp4", false},
		// A distinct timestamp, not a case-flipped twin of the first entry:
		// NTFS is case-insensitive, so "Manual_20260725_120000.mp4" would
		// overwrite "manual_20260725_120000.mp4" and only one would be listed.
		{"Manual_20260725_130000.mp4", false},
	}

	directory := t.TempDir()
	now := time.Now()

	for index, testCase := range cases {
		writeRecording(t, directory, testCase.name, 10, now.Add(-time.Duration(index)*time.Minute))
	}

	recordings, err := ListVideos(directory)
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}

	manualByName := make(map[string]bool, len(recordings))
	for _, recording := range recordings {
		manualByName[recording.Name] = recording.Manual
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, listed := manualByName[testCase.name]
			if !listed {
				t.Fatalf("%s was not listed at all", testCase.name)
			}
			if actual != testCase.expectedManual {
				t.Errorf("Manual = %v, want %v", actual, testCase.expectedManual)
			}
		})
	}
}

func TestListReportsThePathSizeAndTimeOfEachRecording(t *testing.T) {
	directory := t.TempDir()
	modified := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	path := writeRecording(t, directory, "clip.mp4", 2048, modified)

	recordings, err := ListVideos(directory)
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}
	if len(recordings) != 1 {
		t.Fatalf("ListVideos returned %d recordings, want 1", len(recordings))
	}

	recording := recordings[0]
	if recording.Path != path {
		t.Errorf("Path = %q, want %q", recording.Path, path)
	}
	if recording.SizeBytes != 2048 {
		t.Errorf("SizeBytes = %d, want 2048", recording.SizeBytes)
	}
	if !recording.CreatedAt.Equal(modified) {
		t.Errorf("CreatedAt = %v, want %v", recording.CreatedAt, modified)
	}
}

func TestSizeMegabytesConvertsFromBytes(t *testing.T) {
	cases := []struct {
		name      string
		sizeBytes int64
		expected  float64
	}{
		{"one mebibyte", 1024 * 1024, 1},
		{"half a mebibyte", 512 * 1024, 0.5},
		{"two and a half mebibytes", 2560 * 1024, 2.5},
		{"an empty file", 0, 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recording := Recording{SizeBytes: testCase.sizeBytes}

			if actual := recording.SizeMegabytes(); actual != testCase.expected {
				t.Errorf("SizeMegabytes() = %v, want %v", actual, testCase.expected)
			}
		})
	}
}

func TestPruneRemovesOnlyRecordingsPastTheRetentionWindow(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()

	writeRecording(t, directory, "yesterday.mp4", 10, now.Add(-24*time.Hour))
	writeRecording(t, directory, "last-week.mp4", 10, now.Add(-7*24*time.Hour))
	writeRecording(t, directory, "last-month.jpg", 10, now.Add(-30*24*time.Hour))
	writeRecording(t, directory, "this-morning.jpg", 10, now.Add(-6*time.Hour))

	removed, err := Prune(directory, 3)
	if err != nil {
		t.Fatalf("Prune returned %v", err)
	}
	if removed != 2 {
		t.Errorf("Prune removed %d recordings, want 2", removed)
	}

	// Prune covers clips and stills alike, so one of each should be gone.
	for _, name := range []string{"last-week.mp4", "last-month.jpg"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Errorf("%s survived pruning", name)
		}
	}
	for _, name := range []string{"yesterday.mp4", "this-morning.jpg"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Errorf("%s was pruned but is inside the retention window: %v", name, err)
		}
	}
}

// TestPruneWithNonPositiveRetentionKeepsEverything covers the documented "keep
// forever" case. Getting this wrong would silently delete a user's entire
// archive the first time the app started, so both zero and a negative value are
// checked.
func TestPruneWithNonPositiveRetentionKeepsEverything(t *testing.T) {
	cases := []struct {
		name          string
		retentionDays int
	}{
		{"zero means keep forever", 0},
		{"a negative value also means keep forever", -1},
		{"a large negative value also means keep forever", -3650},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			directory := t.TempDir()
			ancient := time.Now().AddDate(-5, 0, 0)

			writeRecording(t, directory, "ancient.mp4", 10, ancient)
			writeRecording(t, directory, "ancient.jpg", 10, ancient)

			removed, err := Prune(directory, testCase.retentionDays)
			if err != nil {
				t.Fatalf("Prune returned %v", err)
			}
			if removed != 0 {
				t.Errorf("Prune removed %d recordings, want 0", removed)
			}

			for _, name := range []string{"ancient.mp4", "ancient.jpg"} {
				if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
					t.Errorf("%s was deleted despite a keep-forever retention: %v", name, err)
				}
			}
		})
	}
}

func TestPruneLeavesFileTypesItDoesNotManage(t *testing.T) {
	// Prune walks .mp4 and .jpg only. Anything else in the media directory is
	// not the app's to delete.
	directory := t.TempDir()
	ancient := time.Now().AddDate(-5, 0, 0)

	writeRecording(t, directory, "ancient.mp4", 10, ancient)
	writeRecording(t, directory, "notes.txt", 10, ancient)

	removed, err := Prune(directory, 1)
	if err != nil {
		t.Fatalf("Prune returned %v", err)
	}
	if removed != 1 {
		t.Errorf("Prune removed %d recordings, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(directory, "notes.txt")); err != nil {
		t.Errorf("notes.txt was deleted but is not a recording: %v", err)
	}
}

func TestPruneOfAMissingDirectoryIsANoOp(t *testing.T) {
	// Pruning runs on startup, before the media directories necessarily exist.
	removed, err := Prune(filepath.Join(t.TempDir(), "never-created"), 30)
	if err != nil {
		t.Fatalf("Prune returned %v, want no error", err)
	}
	if removed != 0 {
		t.Errorf("Prune removed %d recordings, want 0", removed)
	}
}

func TestPruneKeepsEverythingWhenNothingIsOldEnough(t *testing.T) {
	directory := t.TempDir()
	now := time.Now()

	writeRecording(t, directory, "recent.mp4", 10, now.Add(-time.Hour))
	writeRecording(t, directory, "recent.jpg", 10, now.Add(-2*time.Hour))

	removed, err := Prune(directory, 30)
	if err != nil {
		t.Fatalf("Prune returned %v", err)
	}
	if removed != 0 {
		t.Errorf("Prune removed %d recordings, want 0", removed)
	}

	remaining, err := ListVideos(directory)
	if err != nil {
		t.Fatalf("ListVideos returned %v", err)
	}
	assertNamesInOrder(t, remaining, "recent.mp4")
}

func TestPosterPathSitsInHiddenSubdirectoryAndKeepsClipName(t *testing.T) {
	clipPath := filepath.Join("videos", "motion_2026-01-01_00-00-00.mp4")

	expected := filepath.Join("videos", ".posters", "motion_2026-01-01_00-00-00.mp4.jpg")
	if actual := PosterPath(clipPath); actual != expected {
		t.Fatalf("PosterPath returned %s, want %s", actual, expected)
	}
}

func TestPostersAreNotListedAsRecordings(t *testing.T) {
	directory := t.TempDir()
	clipPath := writeRecording(t, directory, "motion_a.mp4", 10, time.Now())

	posterPath := PosterPath(clipPath)
	if err := os.MkdirAll(filepath.Dir(posterPath), 0o700); err != nil {
		t.Fatalf("creating poster directory returned %v", err)
	}
	writeRecording(t, filepath.Dir(posterPath), filepath.Base(posterPath), 10, time.Now())

	snapshots, err := ListSnapshots(directory)
	if err != nil {
		t.Fatalf("ListSnapshots returned %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("poster listed as a snapshot: %v", recordingNames(snapshots))
	}
}

func TestPruneRemovesThePosterOfAPrunedClip(t *testing.T) {
	directory := t.TempDir()
	old := time.Now().AddDate(0, 0, -10)

	clipPath := writeRecording(t, directory, "motion_old.mp4", 10, old)
	keptPath := writeRecording(t, directory, "motion_new.mp4", 10, time.Now())

	for _, path := range []string{clipPath, keptPath} {
		posterPath := PosterPath(path)
		if err := os.MkdirAll(filepath.Dir(posterPath), 0o700); err != nil {
			t.Fatalf("creating poster directory returned %v", err)
		}
		writeRecording(t, filepath.Dir(posterPath), filepath.Base(posterPath), 10, time.Now())
	}

	if _, err := Prune(directory, 7); err != nil {
		t.Fatalf("Prune returned %v", err)
	}

	if _, err := os.Stat(PosterPath(clipPath)); !os.IsNotExist(err) {
		t.Fatalf("poster of the pruned clip survived: %v", err)
	}
	if _, err := os.Stat(PosterPath(keptPath)); err != nil {
		t.Fatalf("poster of the kept clip was removed: %v", err)
	}
}

func TestPruneToleratesAClipWithoutAPoster(t *testing.T) {
	directory := t.TempDir()
	writeRecording(t, directory, "motion_old.mp4", 10, time.Now().AddDate(0, 0, -10))

	removed, err := Prune(directory, 7)
	if err != nil || removed != 1 {
		t.Fatalf("Prune returned %d, %v, want 1, nil", removed, err)
	}
}
