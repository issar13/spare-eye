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

// openCapture opens a source, naming a backend where the default is a poor one.
//
// On Linux OpenCV tries GStreamer first for a local device and prints a
// paragraph of warnings for every index that is not a capture device, which on
// a typical laptop is most of them. Asking for V4L2 directly avoids building a
// pipeline that cannot run.
//
// A network stream is the opposite case: V4L2 speaks to kernel video devices
// and has no idea what to do with a URL, so the default preference — which
// resolves to FFMPEG — is the only one that can open one.
func openCapture(source Source) (*gocv.VideoCapture, error) {
	if runtime.GOOS == "linux" && !source.Network() {
		return gocv.OpenVideoCaptureWithAPI(source.target(), gocv.VideoCaptureV4L2)
	}

	return gocv.OpenVideoCapture(source.target())
}

// AvailableCameras returns every index that opens *and* produces a frame,
// probing in this process. A console program can call it directly, because a
// driver that takes the process down there takes down something the user is
// watching; a windowed app should use AvailableCamerasIsolated instead.
//
// Opening alone is not enough to prove a device is usable: virtual camera
// drivers such as Iriun or OBS present a device node even when nothing is
// streaming through it, so a naive probe reports a camera that then fails at
// first read. Each candidate is required to hand over a real frame.
// onProbe, when supplied, is called with each index just before it is opened.
// A failure inside the capture backend can take the whole process down without
// surfacing as a Go error, so the caller's log is the only record of which
// device was being opened at the time.
func AvailableCameras(onProbe func(index int)) []Camera {
	defer pinCameraThread()()

	found := make([]Camera, 0, 1)

	for index := 0; index < cameraProbeLimit; index++ {
		if onProbe != nil {
			onProbe(index)
		}

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
	defer pinCameraThread()()

	for index := 0; index < cameraProbeLimit; index++ {
		if camera, err := probeCamera(index); err == nil {
			return camera.Index, true
		}
	}

	return 0, false
}

func probeCamera(index int) (Camera, error) {
	width, height, err := probeSource(DeviceSource(index))
	if err != nil {
		return Camera{}, err
	}

	return Camera{
		Index:  index,
		Width:  width,
		Height: height,
		Label:  fmt.Sprintf("Camera %d (%dx%d)", index, width, height),
	}, nil
}

// probeStream is the same proof applied to a network address. The result is
// reported as a Camera because the picture it describes is the same one, and
// because the child process that carries it back speaks a protocol built around
// that shape; its index is not a device number and nothing reads it.
func probeStream(address string) (Camera, error) {
	width, height, err := probeSource(NetworkSource(address))
	if err != nil {
		return Camera{}, err
	}

	return Camera{
		Width:  width,
		Height: height,
		Label:  fmt.Sprintf("Network stream (%dx%d)", width, height),
	}, nil
}

// probeSource opens a source and requires a real frame from it before calling
// it usable.
func probeSource(source Source) (int, int, error) {
	capture, err := openCapture(source)
	if err != nil {
		return 0, 0, err
	}
	defer capture.Close()

	if !capture.IsOpened() {
		return 0, 0, fmt.Errorf("%s did not open", source)
	}

	frame := gocv.NewMat()
	defer frame.Close()

	if !capture.Read(&frame) || frame.Empty() {
		return 0, 0, fmt.Errorf("%s opened but produced no frame", source)
	}

	return frame.Cols(), frame.Rows(), nil
}
