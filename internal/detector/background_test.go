package detector

import (
	"image"
	"image/color"
	"testing"

	"gocv.io/x/gocv"
)

const (
	testFrameWidth  = 160
	testFrameHeight = 120
	testThreshold   = 30.0
	testMinimumArea = 500.0
)

func uniformFrame(brightness float64) gocv.Mat {
	return gocv.NewMatWithSizeFromScalar(gocv.NewScalar(brightness, 0, 0, 0), testFrameHeight, testFrameWidth, gocv.MatTypeCV8U)
}

func TestMotionAnalyzerIgnoresAGradualBrightnessRamp(t *testing.T) {
	analyzer := newMotionAnalyzer()
	defer analyzer.Close()

	for brightness := 60.0; brightness <= 140; brightness++ {
		frame := uniformFrame(brightness)
		triggered := analyzer.motionPresent(frame, testThreshold, testMinimumArea)
		frame.Close()

		if triggered {
			t.Fatalf("brightness %.0f counted as motion during a gradual ramp", brightness)
		}
	}
}

func TestMotionAnalyzerFlagsAMovingBrightRectangle(t *testing.T) {
	analyzer := newMotionAnalyzer()
	defer analyzer.Close()

	for range 5 {
		frame := uniformFrame(60)
		analyzer.motionPresent(frame, testThreshold, testMinimumArea)
		frame.Close()
	}

	frame := uniformFrame(60)
	defer frame.Close()
	gocv.Rectangle(&frame, image.Rect(40, 30, 100, 90), color.RGBA{R: 255}, -1)

	if !analyzer.motionPresent(frame, testThreshold, testMinimumArea) {
		t.Fatal("bright rectangle on a static scene was not flagged as motion")
	}
}

func TestMotionAnalyzerSeedsFromTheFirstFrameWithoutTriggering(t *testing.T) {
	analyzer := newMotionAnalyzer()
	defer analyzer.Close()

	frame := uniformFrame(200)
	defer frame.Close()

	if analyzer.motionPresent(frame, testThreshold, testMinimumArea) {
		t.Fatal("first frame has nothing to compare against and must not trigger")
	}
}
