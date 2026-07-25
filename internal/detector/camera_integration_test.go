//go:build camera

// Integration tests that need a real camera. Run with:
//
//	go test -tags camera ./internal/detector/ -v
//
// They are tagged out of the default build so CI, which has no webcam, still
// runs the rest of the suite.
package detector

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gocv.io/x/gocv"

	"kaa-rada/internal/config"
)

// TestManualClipContainsFrames decodes the clip it produces. A recorder that
// writes no frames still leaves a file on disk with a plausible size, so
// asserting the file exists proves nothing; the frames have to be read back.
func TestManualClipContainsFrames(t *testing.T) {
	settings := settingsInTempDirectory(t)
	subject := New(settings)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runFailed := make(chan error, 1)
	go func() { runFailed <- subject.Run(ctx) }()

	clipPath := ""
	clipFinished := make(chan struct{})

	go func() {
		defer close(clipFinished)

		for event := range subject.Events() {
			switch event.Kind {
			case EventCameraOpened:
				t.Logf("camera opened: %s", event.Detail)
			case EventClipStarted:
				t.Logf("clip started: %s", event.MediaPath)
			case EventClipFinished:
				clipPath = event.MediaPath
				if event.Err != nil {
					t.Errorf("clip finished with error: %v", event.Err)
				}
				if event.Detail != "" {
					t.Logf("clip note: %s", event.Detail)
				}
				return
			case EventFailure:
				t.Errorf("detector reported failure: %v", event.Err)
			}
		}
	}()

	// Let the camera settle and the frame-rate measurement complete.
	time.Sleep(4 * time.Second)

	subject.StartManualRecording()
	time.Sleep(5 * time.Second)
	subject.StopManualRecording()

	select {
	case <-clipFinished:
	case <-time.After(15 * time.Second):
		t.Fatal("clip never finished")
	}

	cancel()

	if err := <-runFailed; err != nil {
		t.Fatalf("Run returned %v", err)
	}

	if clipPath == "" {
		t.Fatal("no clip path was reported")
	}

	assertPlayableVideo(t, clipPath)
}

// TestStatusReportsCameraGeometry confirms the detector reads its dimensions
// from the camera rather than assuming a resolution.
func TestStatusReportsCameraGeometry(t *testing.T) {
	settings := settingsInTempDirectory(t)
	subject := New(settings)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() { _ = subject.Run(ctx) }()
	go func() {
		for range subject.Events() {
		}
	}()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		status := subject.Status()
		if status.FrameWidth > 0 && status.CaptureFPS > 0 {
			t.Logf("camera geometry: %dx%d at %.1f FPS",
				status.FrameWidth, status.FrameHeight, status.CaptureFPS)
			cancel()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	cancel()
	t.Fatal("status never reported camera geometry")
}

// TestPreviewProducesDecodableJPEG covers the frame path the Wails UI depends on.
func TestPreviewProducesDecodableJPEG(t *testing.T) {
	settings := settingsInTempDirectory(t)
	subject := New(settings)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	go func() { _ = subject.Run(ctx) }()
	go func() {
		for range subject.Events() {
		}
	}()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if jpeg := subject.PreviewJPEG(); len(jpeg) > 0 {
			decoded, err := gocv.IMDecode(jpeg, gocv.IMReadColor)
			if err != nil {
				t.Fatalf("preview JPEG did not decode: %v", err)
			}
			defer decoded.Close()

			if decoded.Empty() {
				t.Fatal("preview JPEG decoded to an empty image")
			}

			t.Logf("preview: %d bytes, %dx%d", len(jpeg), decoded.Cols(), decoded.Rows())
			cancel()
			return
		}
		time.Sleep(250 * time.Millisecond)
	}

	cancel()
	t.Fatal("no preview frame was ever produced")
}

func assertPlayableVideo(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("clip was not written: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("clip file is empty")
	}

	playback, err := gocv.VideoCaptureFile(path)
	if err != nil {
		t.Fatalf("clip could not be opened for playback: %v", err)
	}
	defer playback.Close()

	frame := gocv.NewMat()
	defer frame.Close()

	frameCount := 0
	for playback.Read(&frame) && !frame.Empty() {
		frameCount++
	}

	if frameCount == 0 {
		t.Fatalf("clip %s contains no decodable frames (%d bytes on disk)", path, info.Size())
	}

	t.Logf("clip verified: %d frames, %d bytes", frameCount, info.Size())
}

func settingsInTempDirectory(t *testing.T) config.Config {
	t.Helper()

	// The default index is not assumed to work. Virtual webcam drivers occupy
	// low indices without streaming, so the first genuinely usable device is
	// discovered rather than guessed.
	cameras := AvailableCameras()
	if len(cameras) == 0 {
		t.Skip("no usable camera on this machine")
	}
	t.Logf("using camera index %d (%s)", cameras[0].Index, cameras[0].Label)

	settings := config.Default()
	settings.Detection.CameraIndex = cameras[0].Index
	settings.Storage.MediaDirectory = t.TempDir()

	for _, sub := range []string{"snapshots", "videos"} {
		if err := os.MkdirAll(filepath.Join(settings.Storage.MediaDirectory, sub), 0o700); err != nil {
			t.Fatalf("creating media directory: %v", err)
		}
	}

	return settings
}
