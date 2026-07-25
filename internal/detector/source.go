package detector

import (
	"fmt"

	"kaa-rada/internal/config"
)

// Source is what a capture is opened on: either a local device index or the
// address of a network stream.
//
// It exists so the int-or-string decision gocv's API forces is made once. Every
// other place in this package asks the source what it is instead of looking at
// two settings fields and working it out again.
type Source struct {
	index   int
	address string
}

func DeviceSource(index int) Source { return Source{index: index} }

func NetworkSource(address string) Source { return Source{address: address} }

// SourceFor reads whichever of the two the settings describe. An address wins:
// somebody who has typed one is not also expressing an opinion about which USB
// port to open.
func SourceFor(settings config.DetectionSettings) Source {
	if settings.CameraURL != "" {
		return NetworkSource(settings.CameraURL)
	}

	return DeviceSource(settings.CameraIndex)
}

// Network reports whether this is a stream. It decides both which capture
// backend is asked for and whether frames drying up mid-run is worth retrying.
func (s Source) Network() bool { return s.address != "" }

// target is the value gocv is handed. OpenVideoCapture takes an int for a
// device and a string for a URI, distinguished at runtime.
func (s Source) target() interface{} {
	if s.Network() {
		return s.address
	}

	return s.index
}

func (s Source) String() string {
	if s.Network() {
		return s.address
	}

	return fmt.Sprintf("camera %d", s.index)
}
