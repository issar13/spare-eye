package detector

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocv.io/x/gocv"

	"spareeye/internal/config"
)

func TestIsNightHandlesWindowCrossingMidnight(t *testing.T) {
	settings := config.Default()
	// Defaults are 19:00 to 06:00, a window that wraps past midnight.
	subject := New(settings)

	cases := []struct {
		name        string
		clock       time.Time
		expectNight bool
	}{
		{"just before night starts", at(18, 59), false},
		{"exactly at night start", at(19, 0), true},
		{"late evening", at(23, 30), true},
		{"after midnight", at(2, 0), true},
		{"one minute before night ends", at(5, 59), true},
		{"exactly at night end", at(6, 0), false},
		{"midday", at(12, 0), false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := subject.isNight(testCase.clock); got != testCase.expectNight {
				t.Errorf("isNight(%s) = %v, want %v",
					testCase.clock.Format("15:04"), got, testCase.expectNight)
			}
		})
	}
}

// TestIsNightHonoursABoundaryOffTheHour is why the window is stored as minutes
// rather than an hour: a schedule that turns over at 21:30 has to be honoured
// to the minute, not rounded to 21:00 or 22:00.
func TestIsNightHonoursABoundaryOffTheHour(t *testing.T) {
	settings := config.Default()
	settings.Detection.NightStartMinutes = 21*60 + 30
	settings.Detection.NightEndMinutes = 6 * 60

	subject := New(settings)

	if subject.isNight(at(21, 29)) {
		t.Error("21:29 should not be night for a window starting at 21:30")
	}
	if !subject.isNight(at(21, 30)) {
		t.Error("21:30 should be night for a window starting at 21:30")
	}
}

func TestIsNightWithDaytimeWindow(t *testing.T) {
	settings := config.Default()
	// A non-wrapping window, e.g. someone treating 09:00-17:00 as "night".
	settings.Detection.NightStartMinutes = 9 * 60
	settings.Detection.NightEndMinutes = 17 * 60

	subject := New(settings)

	if subject.isNight(at(8, 59)) {
		t.Error("08:59 should not be night for a 09:00-17:00 window")
	}
	if !subject.isNight(at(12, 0)) {
		t.Error("12:00 should be night for a 09:00-17:00 window")
	}
	if subject.isNight(at(17, 0)) {
		t.Error("17:00 should not be night for a 09:00-17:00 window")
	}
}

func TestProfileForSelectsConfiguredSensitivity(t *testing.T) {
	settings := config.Default()
	settings.Detection.Day.MinimumContourArea = 1111
	settings.Detection.Night.MinimumContourArea = 2222

	subject := New(settings)

	if got := subject.profileFor(false).MinimumContourArea; got != 1111 {
		t.Errorf("day profile area = %v, want 1111", got)
	}
	if got := subject.profileFor(true).MinimumContourArea; got != 2222 {
		t.Errorf("night profile area = %v, want 2222", got)
	}
}

// TestPrebufferHonoursItsLimit is the guard on the buffer's bound. If it stops
// discarding, clips silently grow a long tail of unwanted history and the
// buffer's memory cost becomes unbounded.
func TestPrebufferHonoursItsLimit(t *testing.T) {
	const limit = 4

	buffer := newPrebuffer(limit)
	defer buffer.close()

	for index := 0; index < limit*3; index++ {
		frame := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
		buffer.add(frame)
		frame.Close()
	}

	drained := buffer.drain()
	defer closeAll(drained)

	if len(drained) != limit {
		t.Fatalf("buffer held %d frames, want %d", len(drained), limit)
	}

	for _, frame := range drained {
		if frame.Empty() {
			t.Error("buffer retained an empty frame")
		}
	}
}

func TestPrebufferDrainLeavesBufferEmpty(t *testing.T) {
	buffer := newPrebuffer(3)
	defer buffer.close()

	frame := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	defer frame.Close()

	buffer.add(frame)
	closeAll(buffer.drain())

	if remaining := buffer.drain(); len(remaining) != 0 {
		closeAll(remaining)
		t.Errorf("drain left %d frames behind", len(remaining))
	}
}

func TestNewPrebufferRejectsNonPositiveLimit(t *testing.T) {
	// A very short lead time or a slow camera can compute zero frames; the
	// buffer must still be usable rather than panicking on add.
	buffer := newPrebuffer(0)
	defer buffer.close()

	frame := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	defer frame.Close()

	buffer.add(frame)

	drained := buffer.drain()
	defer closeAll(drained)

	if len(drained) != 1 {
		t.Errorf("buffer held %d frames, want 1", len(drained))
	}
}

// TestPreviewIntervalClampsAHandEditedFrameRate matters because config.json is
// hand-editable and the interval is a division: a frames-per-second of zero
// would divide by zero inside the capture loop and take the whole watch down.
func TestPreviewIntervalClampsAHandEditedFrameRate(t *testing.T) {
	cases := []struct {
		name            string
		framesPerSecond int
		expected        time.Duration
	}{
		{"the default of 12 frames per second", 12, time.Second / 12},
		{"zero is raised to the minimum", 0, time.Second / minPreviewFPS},
		{"a negative is raised to the minimum", -30, time.Second / minPreviewFPS},
		{"exactly the minimum is kept", minPreviewFPS, time.Second / minPreviewFPS},
		{"exactly the maximum is kept", maxPreviewFPS, time.Second / maxPreviewFPS},
		{"above the maximum is lowered", 240, time.Second / maxPreviewFPS},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := config.Default()
			settings.Detection.PreviewFPS = testCase.framesPerSecond

			subject := New(settings)

			if got := subject.previewInterval(); got != testCase.expected {
				t.Errorf("previewInterval() with %d FPS = %v, want %v",
					testCase.framesPerSecond, got, testCase.expected)
			}
		})
	}
}

// TestPreviewQualityClampsAHandEditedQuality guards the value handed to libjpeg,
// which accepts 0 to 100 and rejects anything outside it: an out-of-range
// quality would fail every encode and leave the live view permanently blank.
func TestPreviewQualityClampsAHandEditedQuality(t *testing.T) {
	cases := []struct {
		name       string
		configured int
		expected   int
	}{
		{"the default of 80", 80, 80},
		{"zero is raised to the minimum", 0, minPreviewQuality},
		{"a negative is raised to the minimum", -10, minPreviewQuality},
		{"exactly the minimum is kept", minPreviewQuality, minPreviewQuality},
		{"exactly the maximum is kept", maxPreviewQuality, maxPreviewQuality},
		{"a value libjpeg would reject is lowered", 200, maxPreviewQuality},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := config.Default()
			settings.Detection.PreviewQuality = testCase.configured

			subject := New(settings)

			if got := subject.previewQuality(); got != testCase.expected {
				t.Errorf("previewQuality() with %d = %d, want %d",
					testCase.configured, got, testCase.expected)
			}
		})
	}
}

func TestClampIntKeepsTheBoundsInclusive(t *testing.T) {
	cases := []struct {
		name     string
		value    int
		lowest   int
		highest  int
		expected int
	}{
		{"inside the range is untouched", 5, 1, 10, 5},
		{"the lower bound is a legal value", 1, 1, 10, 1},
		{"the upper bound is a legal value", 10, 1, 10, 10},
		{"below the range becomes the lower bound", 0, 1, 10, 1},
		{"above the range becomes the upper bound", 11, 1, 10, 10},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := clampInt(testCase.value, testCase.lowest, testCase.highest)
			if got != testCase.expected {
				t.Errorf("clampInt(%d, %d, %d) = %d, want %d",
					testCase.value, testCase.lowest, testCase.highest, got, testCase.expected)
			}
		})
	}
}

func at(hour, minute int) time.Time {
	return time.Date(2026, time.July, 25, hour, minute, 0, 0, time.UTC)
}

func closeAll(frames []gocv.Mat) {
	for _, frame := range frames {
		frame.Close()
	}
}

func TestManualSnapshotIsSavedWithManualPrefixAndAnnounced(t *testing.T) {
	settings := config.Default()
	settings.Storage.MediaDirectory = t.TempDir()
	if err := os.MkdirAll(settings.SnapshotDirectory(), 0o755); err != nil {
		t.Fatal(err)
	}
	subject := New(settings)

	frame := gocv.NewMatWithSize(48, 64, gocv.MatTypeCV8UC3)
	defer frame.Close()

	subject.TakeSnapshot()
	subject.handleManualRequests(frame, image.Pt(64, 48), 10, nil)

	saved, err := filepath.Glob(filepath.Join(settings.SnapshotDirectory(), "manual_*.jpg"))
	if err != nil || len(saved) != 1 {
		t.Fatalf("expected one manual_*.jpg, got %v (err %v)", saved, err)
	}

	select {
	case event := <-subject.Events():
		if event.Kind != EventSnapshotSaved || !event.Manual || event.MediaPath != saved[0] {
			t.Errorf("unexpected event %+v, want a manual EventSnapshotSaved for %s", event, saved[0])
		}
	default:
		t.Fatal("no event was published")
	}
}
