package detector

import (
	"image"

	"gocv.io/x/gocv"
)

// backgroundBlendWeight is how much each analysed frame contributes to the
// background model. Diffing only against the previous frame turns a lighting
// change into a burst of false motion, whereas a running average lets the
// scene absorb a gradual change. The trade-off is the other direction: too high
// and a person standing still fades into the background and stops counting as
// motion; too low and the model lags behind real lighting shifts. 0.05 settles
// in roughly twenty samples, which at the default one-second interval is long
// enough to keep a lingering visitor "moving" and short enough to follow
// clouds and lamps. It is deliberately not a setting: the right value depends
// on the sample interval, which users already tune.
const backgroundBlendWeight = 0.05

// motionAnalyzer compares each blurred grayscale frame against a running
// average of the scene. It owns every Mat it holds; Close releases them.
type motionAnalyzer struct {
	background      gocv.Mat
	backgroundBytes gocv.Mat
	difference      gocv.Mat
	thresholded     gocv.Mat
	dilationKernel  gocv.Mat
}

func newMotionAnalyzer() *motionAnalyzer {
	return &motionAnalyzer{
		background:      gocv.NewMat(),
		backgroundBytes: gocv.NewMat(),
		difference:      gocv.NewMat(),
		thresholded:     gocv.NewMat(),
		dilationKernel:  gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 3)),
	}
}

func (analyzer *motionAnalyzer) Close() {
	analyzer.background.Close()
	analyzer.backgroundBytes.Close()
	analyzer.difference.Close()
	analyzer.thresholded.Close()
	analyzer.dilationKernel.Close()
}

// motionPresent reports whether the frame differs from the background by more
// than threshold over a region larger than minimumArea, then folds the frame
// into the background. The first frame only seeds the model.
func (analyzer *motionAnalyzer) motionPresent(grayscale gocv.Mat, threshold float64, minimumArea float64) bool {
	if analyzer.background.Empty() {
		grayscale.ConvertTo(&analyzer.background, gocv.MatTypeCV32F)
		return false
	}

	analyzer.background.ConvertTo(&analyzer.backgroundBytes, gocv.MatTypeCV8U)
	gocv.AbsDiff(analyzer.backgroundBytes, grayscale, &analyzer.difference)
	gocv.Threshold(analyzer.difference, &analyzer.thresholded, float32(threshold), 255, gocv.ThresholdBinary)
	gocv.Dilate(analyzer.thresholded, &analyzer.thresholded, analyzer.dilationKernel)

	// Blended after the comparison so a frame never dilutes the very
	// background it is being judged against.
	gocv.AccumulatedWeighted(grayscale, &analyzer.background, backgroundBlendWeight)

	return exceedsArea(analyzer.thresholded, minimumArea)
}
