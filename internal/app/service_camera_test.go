//go:build camera

// Integration tests that need a real camera. Run with:
//
//	go test -tags camera ./internal/app/ -v
//
// They are tagged out of the default build so CI, which has no webcam, still
// runs the rest of the suite.
package app

import (
	"bytes"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kaa-rada/internal/config"
	"kaa-rada/internal/detector"
)

// armedService builds a service pointed at the first usable camera, with alerts
// switched off so no test ever messages a real chat.
func armedService(t *testing.T) *Service {
	t.Helper()

	cameraIndex, found := detector.FirstUsableCamera()
	if !found {
		t.Skip("no usable camera; another process may be holding it")
	}

	mediaDirectory := t.TempDir()
	for _, sub := range []string{"snapshots", "videos"} {
		if err := os.MkdirAll(filepath.Join(mediaDirectory, sub), 0o700); err != nil {
			t.Fatalf("creating %s: %v", sub, err)
		}
	}

	log, err := OpenLog(t.TempDir())
	if err != nil {
		t.Fatalf("opening log: %v", err)
	}
	t.Cleanup(func() { log.Close() })

	settings := config.Default()
	settings.Storage.MediaDirectory = mediaDirectory
	settings.Detection.CameraIndex = cameraIndex
	settings.Telegram.Enabled = false

	return NewService(log, settings, nil)
}

func TestArmProducesADecodablePreview(t *testing.T) {
	service := armedService(t)

	if result := service.Arm(); !result.OK {
		t.Fatalf("arming: %+v", result)
	}
	t.Cleanup(func() { service.Disarm() })

	if status := service.Status(); !status.Armed {
		t.Fatalf("status after arming is %+v", status)
	}

	// The preview is re-encoded every 200ms, so the first frame may not be
	// there the instant the camera opens.
	frame := waitForPreview(t, service)
	status := waitForMeasuredStatus(t, service)

	decoded, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil {
		t.Fatalf("preview is not decodable JPEG: %v", err)
	}
	if decoded.Bounds().Dx() != status.FrameWidth {
		t.Fatalf("preview is %d wide, status reports %d", decoded.Bounds().Dx(), status.FrameWidth)
	}
}

func waitForPreview(t *testing.T, service *Service) []byte {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if frame := service.PreviewJPEG(); frame != nil {
			return frame
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatal("no preview frame within 5 seconds")
	return nil
}

// waitForMeasuredStatus waits for the frame geometry to appear. The detector
// only fills its status in once it has analysed two frames, which is two
// sample intervals after the camera opened, so the UI shows placeholders for
// the first second or two of a run.
func waitForMeasuredStatus(t *testing.T, service *Service) StatusView {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if status := service.Status(); status.FrameWidth > 0 && status.CaptureFPS > 0 {
			return status
		}
		time.Sleep(200 * time.Millisecond)
	}

	t.Fatal("status never reported a frame size")
	return StatusView{}
}

// TestSavingTuningRestartsTheDetector covers the reason a restart exists at
// all: the prebuffer and the frame size are fixed when the camera opens, so a
// changed lead time only takes effect on a fresh run.
func TestSavingTuningRestartsTheDetector(t *testing.T) {
	service := armedService(t)

	if result := service.Arm(); !result.OK {
		t.Fatalf("arming: %+v", result)
	}
	t.Cleanup(func() { service.Disarm() })

	tuning := service.Settings().Tuning
	tuning.PrebufferLeadSeconds = 1
	tuning.ClipDurationSeconds = 4

	if result := service.SaveTuning(tuning); !result.OK {
		t.Fatalf("saving tuning: %+v", result)
	}

	if status := service.Status(); !status.Armed {
		t.Fatalf("detector did not come back up after the restart: %+v", status)
	}
	if service.Settings().Tuning.ClipDurationSeconds != 4 {
		t.Fatal("the new clip duration was not persisted")
	}

	// A camera that reopened is delivering frames again.
	waitForPreview(t, service)
}

// TestDisarmFlushesAManualClip is the shutdown path the UI depends on: changing
// settings mid-recording must not truncate the file being written.
func TestDisarmFlushesAManualClip(t *testing.T) {
	service := armedService(t)

	if result := service.Arm(); !result.OK {
		t.Fatalf("arming: %+v", result)
	}

	if result := service.StartManualRecording(); !result.OK {
		t.Fatalf("starting a manual clip: %+v", result)
	}

	time.Sleep(3 * time.Second)

	if status := service.Status(); !status.RecordingManual {
		t.Fatalf("manual recording did not start: %+v", status)
	}

	if result := service.Disarm(); !result.OK {
		t.Fatalf("disarming: %+v", result)
	}

	clips, err := service.Clips()
	if err != nil {
		t.Fatalf("listing clips: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("found %d clips, want the one that was being written", len(clips))
	}
	if clips[0].SizeMegabytes <= 0 {
		t.Fatalf("clip %s is empty; the flush on cancel did not run", clips[0].Name)
	}
}
