package detector

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocv.io/x/gocv"

	"spareeye/internal/media"
)

func TestRecorderWritesPosterBesideTheClip(t *testing.T) {
	clipPath := filepath.Join(t.TempDir(), "motion_2026-01-01_00-00-00.mp4")

	poster := gocv.NewMatWithSize(48, 64, gocv.MatTypeCV8UC3)

	active := startRecorder(clipRequest{
		path:      clipPath,
		frameRate: 10,
		frameSize: image.Pt(64, 48),
		duration:  50 * time.Millisecond,
		poster:    poster,
	})
	active.wait()

	info, err := os.Stat(media.PosterPath(clipPath))
	if err != nil || info.Size() == 0 {
		t.Fatalf("poster missing or empty: %v", err)
	}
}
