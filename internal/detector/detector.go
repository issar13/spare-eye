// Package detector owns the camera. It watches for motion, saves snapshots and
// clips, and publishes events that a UI or notifier can subscribe to.
package detector

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"gocv.io/x/gocv"

	"motion-detector/internal/config"
)

// EventKind identifies what happened, so subscribers can filter without
// parsing message text.
type EventKind int

const (
	EventMotionStarted EventKind = iota
	EventMotionStopped
	EventSnapshotSaved
	EventClipStarted
	EventClipFinished
	EventCameraOpened
	EventFailure
)

// Event is one thing the detector observed.
type Event struct {
	Kind      EventKind
	At        time.Time
	Night     bool
	MediaPath string
	Manual    bool
	Detail    string
	Err       error
}

// Status is a snapshot of detector state for the UI to poll.
type Status struct {
	Running         bool      `json:"running"`
	Recording       bool      `json:"recording"`
	RecordingManual bool      `json:"recordingManual"`
	Night           bool      `json:"night"`
	MotionSince     time.Time `json:"motionSince"`
	CaptureFPS      float64   `json:"captureFPS"`
	FrameWidth      int       `json:"frameWidth"`
	FrameHeight     int       `json:"frameHeight"`
}

// previewInterval limits how often a frame is JPEG-encoded for the UI. Encoding
// every frame would burn CPU on work nobody sees.
const previewInterval = 200 * time.Millisecond

// Detector runs the capture and analysis loop.
type Detector struct {
	settings config.Config

	events chan Event

	manualStart chan struct{}
	manualStop  chan struct{}

	// preview holds the most recent JPEG-encoded frame for the UI.
	preview atomic.Pointer[[]byte]

	mutex           sync.Mutex
	activeRecorder  *recorder
	recordingManual bool

	// recordings tracks the goroutines awaiting each clip. Run must not close
	// the event channel until they have all published their final event,
	// otherwise a clip finishing during shutdown sends on a closed channel and
	// panics. Waiting on the recorder alone is not enough: the recorder and the
	// goroutine awaiting it wake on the same signal, so the publish can lose
	// the race against the close.
	recordings sync.WaitGroup

	status atomic.Pointer[Status]
}

// New builds a detector. Call Run to open the camera and begin.
func New(settings config.Config) *Detector {
	created := &Detector{
		settings: settings,
		// Buffered so a slow subscriber cannot stall capture; see publish.
		events:      make(chan Event, 64),
		manualStart: make(chan struct{}, 1),
		manualStop:  make(chan struct{}, 1),
	}
	created.status.Store(&Status{})

	return created
}

// Events returns the event stream. It closes when Run returns.
func (d *Detector) Events() <-chan Event { return d.events }

// Status returns current state, safe to call from any goroutine.
func (d *Detector) Status() Status { return *d.status.Load() }

// PreviewJPEG returns the most recent frame encoded as JPEG, or nil before the
// first frame arrives.
func (d *Detector) PreviewJPEG() []byte {
	stored := d.preview.Load()
	if stored == nil {
		return nil
	}
	return *stored
}

// StartManualRecording asks for a manual clip. Extra requests while one is
// already running are discarded rather than queued.
func (d *Detector) StartManualRecording() {
	select {
	case d.manualStart <- struct{}{}:
	default:
	}
}

// StopManualRecording ends the current manual clip.
func (d *Detector) StopManualRecording() {
	select {
	case d.manualStop <- struct{}{}:
	default:
	}
}

// publish delivers an event, dropping it if no subscriber is keeping up. Capture
// must never block on a UI that stopped reading.
func (d *Detector) publish(event Event) {
	event.At = time.Now()

	select {
	case d.events <- event:
	default:
	}
}

// Run opens the camera and processes frames until ctx is cancelled.
func (d *Detector) Run(ctx context.Context) error {
	// Deferred calls run in reverse order, so the wait happens before the
	// close. Both are needed: see the recordings field.
	defer close(d.events)
	defer d.recordings.Wait()

	camera, err := openCapture(d.settings.Detection.CameraIndex)
	if err != nil {
		return fmt.Errorf("opening camera %d: %w", d.settings.Detection.CameraIndex, err)
	}
	defer camera.Close()

	frame := gocv.NewMat()
	defer frame.Close()

	if !camera.Read(&frame) || frame.Empty() {
		return fmt.Errorf("camera %d opened but returned no frames", d.settings.Detection.CameraIndex)
	}

	frameSize := image.Pt(frame.Cols(), frame.Rows())

	captureFPS, err := measureCaptureRate(camera, &frame)
	if err != nil {
		return err
	}

	d.publish(Event{
		Kind:   EventCameraOpened,
		Detail: fmt.Sprintf("%dx%d at %.1f FPS", frameSize.X, frameSize.Y, captureFPS),
	})

	// The buffer holds only as much history as the configured lead time, so its
	// memory cost follows the real frame rate instead of a fixed frame count.
	leadFrames := int(d.settings.Detection.PrebufferLead.AsDuration().Seconds() * captureFPS)
	history := newPrebuffer(leadFrames)
	defer history.close()

	return d.processFrames(ctx, camera, &frame, frameSize, captureFPS, history)
}

// measureCaptureRate times a short burst of reads. Webcams commonly report 0 or
// a nominal value for VideoCaptureFPS, and writing clips at a wrong frame rate
// is what made the previous build's recordings play back at the wrong speed.
func measureCaptureRate(camera *gocv.VideoCapture, frame *gocv.Mat) (float64, error) {
	const sampleCount = 30

	started := time.Now()
	captured := 0

	for captured < sampleCount {
		if !camera.Read(frame) || frame.Empty() {
			continue
		}
		captured++
	}

	elapsed := time.Since(started)
	if elapsed <= 0 {
		return 0, fmt.Errorf("could not measure camera frame rate")
	}

	measured := float64(captured) / elapsed.Seconds()

	// Clamp to a sane band: a pathological reading would size the prebuffer and
	// the clip frame rate absurdly.
	if measured < 1 {
		measured = 1
	}
	if measured > 60 {
		measured = 60
	}

	return measured, nil
}

func (d *Detector) processFrames(
	ctx context.Context,
	camera *gocv.VideoCapture,
	frame *gocv.Mat,
	frameSize image.Point,
	captureFPS float64,
	history *prebuffer,
) error {
	grayscale := gocv.NewMat()
	defer grayscale.Close()

	previousGrayscale := gocv.NewMat()
	defer previousGrayscale.Close()

	difference := gocv.NewMat()
	defer difference.Close()

	thresholded := gocv.NewMat()
	defer thresholded.Close()

	dilationKernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 3))
	defer dilationKernel.Close()

	var (
		lastAnalysis  time.Time
		lastPreview   time.Time
		lastAlert     time.Time
		motionSince   time.Time
		motionOngoing bool
	)

	for {
		if ctx.Err() != nil {
			d.finishActiveRecording()
			return nil
		}

		// A webcam read blocks until the next frame, which paces this loop at
		// the camera's native rate without a sleep.
		if !camera.Read(frame) || frame.Empty() {
			select {
			case <-ctx.Done():
				d.finishActiveRecording()
				return nil
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}

		night := d.isNight(time.Now())
		profile := d.profileFor(night)

		d.feedActiveRecording(*frame)
		history.add(*frame)
		d.handleManualRequests(*frame, frameSize, captureFPS, history)

		if time.Since(lastPreview) >= previewInterval {
			d.updatePreview(*frame)
			lastPreview = time.Now()
		}

		// Analysis is throttled independently of capture, so sampling cadence
		// no longer dictates the frame rate clips are written at.
		if time.Since(lastAnalysis) < profile.SampleInterval.AsDuration() {
			continue
		}
		lastAnalysis = time.Now()

		gocv.CvtColor(*frame, &grayscale, gocv.ColorBGRToGray)
		gocv.GaussianBlur(grayscale, &grayscale, image.Pt(21, 21), 0, 0, gocv.BorderDefault)

		if previousGrayscale.Empty() {
			grayscale.CopyTo(&previousGrayscale)
			continue
		}

		gocv.AbsDiff(previousGrayscale, grayscale, &difference)
		gocv.Threshold(difference, &thresholded, float32(profile.ThresholdValue), 255, gocv.ThresholdBinary)
		gocv.Dilate(thresholded, &thresholded, dilationKernel)

		motionPresent := exceedsArea(thresholded, profile.MinimumContourArea)
		grayscale.CopyTo(&previousGrayscale)

		d.storeStatus(night, motionSince, captureFPS, frameSize)

		if !motionPresent {
			if motionOngoing {
				d.publish(Event{Kind: EventMotionStopped, Night: night})
			}
			motionOngoing = false
			motionSince = time.Time{}
			continue
		}

		if !motionOngoing {
			motionOngoing = true
			motionSince = time.Now()
			d.publish(Event{Kind: EventMotionStarted, Night: night})
		}

		if time.Since(lastAlert) >= profile.AlertCooldown.AsDuration() {
			if path, err := d.saveSnapshot(*frame, night); err != nil {
				d.publish(Event{Kind: EventFailure, Err: err})
			} else {
				lastAlert = time.Now()
				d.publish(Event{Kind: EventSnapshotSaved, Night: night, MediaPath: path})
			}
		}

		if time.Since(motionSince) >= d.settings.Detection.MotionPersist.AsDuration() {
			d.beginAutomaticClip(frameSize, captureFPS, history)
			// Cleared so a single sustained event cannot immediately queue a
			// second clip the moment this one finishes.
			motionSince = time.Now()
		}
	}
}

// exceedsArea reports whether any moving region is larger than minimumArea.
func exceedsArea(mask gocv.Mat, minimumArea float64) bool {
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	for index := 0; index < contours.Size(); index++ {
		if gocv.ContourArea(contours.At(index)) > minimumArea {
			return true
		}
	}

	return false
}

func (d *Detector) profileFor(night bool) config.SensitivityProfile {
	if night {
		return d.settings.Detection.Night
	}
	return d.settings.Detection.Day
}

// isNight compares minutes-since-midnight, handling the window that wraps past
// midnight. The previous build could only express whole hours.
func (d *Detector) isNight(now time.Time) bool {
	minutes := now.Hour()*60 + now.Minute()
	start := d.settings.Detection.NightStartMinutes
	end := d.settings.Detection.NightEndMinutes

	if start == end {
		return false
	}
	if start < end {
		return minutes >= start && minutes < end
	}

	return minutes >= start || minutes < end
}

func (d *Detector) saveSnapshot(frame gocv.Mat, night bool) (string, error) {
	name := fmt.Sprintf("motion_%s.jpg", time.Now().Format("2006-01-02_15-04-05"))
	path := filepath.Join(d.settings.SnapshotDirectory(), name)

	if !gocv.IMWrite(path, frame) {
		return "", fmt.Errorf("writing snapshot %s", path)
	}

	return path, nil
}

func (d *Detector) updatePreview(frame gocv.Mat) {
	encoded, err := gocv.IMEncode(gocv.JPEGFileExt, frame)
	if err != nil {
		return
	}
	defer encoded.Close()

	// Copy out of the native buffer before it is released.
	jpeg := make([]byte, len(encoded.GetBytes()))
	copy(jpeg, encoded.GetBytes())

	d.preview.Store(&jpeg)
}

func (d *Detector) storeStatus(night bool, motionSince time.Time, captureFPS float64, frameSize image.Point) {
	d.mutex.Lock()
	recording := d.activeRecorder != nil
	manual := d.recordingManual
	d.mutex.Unlock()

	d.status.Store(&Status{
		Running:         true,
		Recording:       recording,
		RecordingManual: manual,
		Night:           night,
		MotionSince:     motionSince,
		CaptureFPS:      captureFPS,
		FrameWidth:      frameSize.X,
		FrameHeight:     frameSize.Y,
	})
}

func (d *Detector) feedActiveRecording(frame gocv.Mat) {
	d.mutex.Lock()
	active := d.activeRecorder
	d.mutex.Unlock()

	if active != nil {
		active.submit(frame)
	}
}

// handleManualRequests services start and stop requests arriving from the UI or
// a notifier command.
func (d *Detector) handleManualRequests(frame gocv.Mat, frameSize image.Point, captureFPS float64, history *prebuffer) {
	select {
	case <-d.manualStart:
		d.beginClip(frameSize, captureFPS, history, true)
	default:
	}

	select {
	case <-d.manualStop:
		d.mutex.Lock()
		active := d.activeRecorder
		manual := d.recordingManual
		d.mutex.Unlock()

		// Only a manual clip responds to stop. The previous build let a stop
		// arriving during an automatic clip latch a flag that silently killed
		// the *next* manual recording.
		if active != nil && manual {
			active.stopRecording()
		}
	default:
	}
}

func (d *Detector) beginAutomaticClip(frameSize image.Point, captureFPS float64, history *prebuffer) {
	d.beginClip(frameSize, captureFPS, history, false)
}

func (d *Detector) beginClip(frameSize image.Point, captureFPS float64, history *prebuffer, manual bool) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.activeRecorder != nil {
		return
	}

	prefix := "motion"
	if manual {
		prefix = "manual"
	}

	name := fmt.Sprintf("%s_%s.mp4", prefix, time.Now().Format("2006-01-02_15-04-05"))
	path := filepath.Join(d.settings.VideoDirectory(), name)

	request := clipRequest{
		path:      path,
		frameRate: captureFPS,
		frameSize: frameSize,
		manual:    manual,
		lead:      history.drain(),
	}
	if !manual {
		request.duration = d.settings.Detection.ClipDuration.AsDuration()
	}

	active := startRecorder(request)
	d.activeRecorder = active
	d.recordingManual = manual

	d.publish(Event{Kind: EventClipStarted, MediaPath: path, Manual: manual})

	// Clearing the active recorder happens off the capture path so a slow
	// encoder finishing never stalls frame reads.
	d.recordings.Add(1)
	go d.awaitRecording(active, path, manual)
}

func (d *Detector) awaitRecording(active *recorder, path string, manual bool) {
	defer d.recordings.Done()

	err := active.wait()

	d.mutex.Lock()
	if d.activeRecorder == active {
		d.activeRecorder = nil
		d.recordingManual = false
	}
	d.mutex.Unlock()

	detail := ""
	if dropped := active.droppedFrames.Load(); dropped > 0 {
		detail = fmt.Sprintf("%d frames dropped; encoder could not keep up", dropped)
	}

	d.publish(Event{
		Kind:      EventClipFinished,
		MediaPath: path,
		Manual:    manual,
		Detail:    detail,
		Err:       err,
	})
}

// finishActiveRecording stops any clip in progress and waits for the file to be
// closed, so shutdown never leaves a truncated video behind.
func (d *Detector) finishActiveRecording() {
	d.mutex.Lock()
	active := d.activeRecorder
	d.mutex.Unlock()

	if active == nil {
		return
	}

	active.stopRecording()
	active.wait()
}
