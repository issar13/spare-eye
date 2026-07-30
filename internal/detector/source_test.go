package detector

import (
	"testing"

	"spareeye/internal/config"
)

func TestAnAddressWinsOverTheDeviceIndex(t *testing.T) {
	source := SourceFor(config.DetectionSettings{CameraIndex: 2, CameraURL: "rtsp://camera.lan/live"})

	if !source.Network() {
		t.Fatal("settings with an address produced a local device source")
	}
	if target, ok := source.target().(string); !ok || target != "rtsp://camera.lan/live" {
		t.Fatalf("gocv would be handed %#v, want the address as a string", source.target())
	}
}

func TestAnEmptyAddressLeavesTheDeviceIndex(t *testing.T) {
	source := SourceFor(config.DetectionSettings{CameraIndex: 2})

	if source.Network() {
		t.Fatal("settings with no address produced a network source")
	}
	if target, ok := source.target().(int); !ok || target != 2 {
		t.Fatalf("gocv would be handed %#v, want the index as an int", source.target())
	}
}
