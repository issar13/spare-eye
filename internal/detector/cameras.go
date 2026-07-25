package detector

import (
	"fmt"
	"runtime"

	"gocv.io/x/gocv"
)

// cameraProbeLimit bounds how many device indices are tried. Probing is cheap
// but not free, and a machine with more than eight cameras is not our concern.
const cameraProbeLimit = 8

// Camera is a capture device that was confirmed to deliver frames.
type Camera struct {
	Index  int    `json:"index"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Label  string `json:"label"`
}

// openCapture opens a device, naming a backend where the default is a poor one.
//
// On Linux OpenCV tries GStreamer first and prints a paragraph of warnings for
// every index that is not a capture device, which on a typical laptop is most of
// them. Asking for V4L2 directly avoids building a pipeline that cannot run.
func openCapture(index int) (*gocv.VideoCapture, error) {
	if runtime.GOOS == "linux" {
		return gocv.OpenVideoCaptureWithAPI(index, gocv.VideoCaptureV4L2)
	}
	return gocv.OpenVideoCapture(index)
}

// AvailableCameras returns every index that opens *and* produces a frame. Use
// it to populate a camera picker.
//
// Opening alone is not enough to prove a device is usable: virtual camera
// drivers such as Iriun or OBS present a device node even when nothing is
// streaming through it, so a naive probe reports a camera that then fails at
// first read. Each candidate is required to hand over a real frame.
func AvailableCameras() []Camera {
	found := make([]Camera, 0, 1)

	for index := 0; index < cameraProbeLimit; index++ {
		camera, err := probeCamera(index)
		if err != nil {
			continue
		}
		found = append(found, camera)
	}

	return found
}

// FirstUsableCamera returns the lowest working index, stopping as soon as one is
// found. Preferred over AvailableCameras when only a default is needed, since it
// avoids probing — and noisily failing on — every remaining device.
func FirstUsableCamera() (int, bool) {
	for index := 0; index < cameraProbeLimit; index++ {
		if camera, err := probeCamera(index); err == nil {
			return camera.Index, true
		}
	}

	return 0, false
}

func probeCamera(index int) (Camera, error) {
	capture, err := openCapture(index)
	if err != nil {
		return Camera{}, err
	}
	defer capture.Close()

	if !capture.IsOpened() {
		return Camera{}, fmt.Errorf("camera %d did not open", index)
	}

	frame := gocv.NewMat()
	defer frame.Close()

	if !capture.Read(&frame) || frame.Empty() {
		return Camera{}, fmt.Errorf("camera %d opened but produced no frame", index)
	}

	return Camera{
		Index:  index,
		Width:  frame.Cols(),
		Height: frame.Rows(),
		Label:  fmt.Sprintf("Camera %d (%dx%d)", index, frame.Cols(), frame.Rows()),
	}, nil
}
