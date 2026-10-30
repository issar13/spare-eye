package detector

import (
	"image"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gocv.io/x/gocv"
)

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
