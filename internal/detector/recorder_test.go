package detector

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TestRecorderWritesBrowserPlayableVideo feeds synthetic frames through a
// recorder and reads the clip back. On Linux the clip must be H.264, because
// that is the only codec the desktop app's webview can play; elsewhere the
// encoder may be missing, so only a decodable clip is required.
func TestRecorderWritesBrowserPlayableVideo(t *testing.T) {
	clipPath := filepath.Join(t.TempDir(), "clip.mp4")
	frameSize := image.Pt(320, 240)

	subject := startRecorder(clipRequest{
		path:      clipPath,
		frameRate: 10,
		frameSize: frameSize,
		manual:    true,
		poster:    gocv.NewMat(),
	})

	frame := gocv.NewMatWithSizeFromScalar(gocv.NewScalar(40, 120, 200, 0), frameSize.Y, frameSize.X, gocv.MatTypeCV8UC3)
	defer frame.Close()

	for range 10 {
		subject.submit(frame)
		time.Sleep(10 * time.Millisecond)
	}

	subject.stopRecording()
	if err := subject.wait(); err != nil {
		t.Fatalf("recorder reported %v", err)
	}

	playback, err := gocv.VideoCaptureFile(clipPath)
	if err != nil {
		t.Fatalf("clip could not be opened for playback: %v", err)
	}
	defer playback.Close()

	codec := strings.ToLower(playback.CodecString())
	t.Logf("clip codec: %q", codec)

	if runtime.GOOS == "linux" && codec != "h264" && codec != "avc1" {
		t.Errorf("clip codec = %q, want H.264 so the webview can play it", codec)
	}

	decoded := gocv.NewMat()
	defer decoded.Close()

	if !playback.Read(&decoded) || decoded.Empty() {
		t.Fatal("clip contains no decodable frames")
	}
}
