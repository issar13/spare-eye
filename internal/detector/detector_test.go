package detector

import (
	"testing"
	"time"

	"gocv.io/x/gocv"

	"kaa-rada/internal/config"
)

func TestIsNightHandlesWindowCrossingMidnight(t *testing.T) {
	settings := config.Default()
	// Defaults are 18:45 to 06:00, the window the previous build could not
	// express with whole-hour constants.
	subject := New(settings)

	cases := []struct {
		name        string
		clock       time.Time
		expectNight bool
	}{
		{"just before night starts", at(18, 44), false},
		{"exactly at night start", at(18, 45), true},
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

// TestPrebufferHonoursItsLimit guards the regression that mattered most: the
// previous build kept 300 frames and prepended all of them to every clip.
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

func at(hour, minute int) time.Time {
	return time.Date(2026, time.July, 25, hour, minute, 0, 0, time.UTC)
}

func closeAll(frames []gocv.Mat) {
	for _, frame := range frames {
		frame.Close()
	}
}
