package detector

import (
	"fmt"
	"image"
	"sync/atomic"
	"time"

	"gocv.io/x/gocv"
)

// clipRequest describes one recording to write to disk.
type clipRequest struct {
	path      string
	frameRate float64
	frameSize image.Point

	// duration bounds an automatic clip. A manual clip leaves it zero and runs
	// until stop is called.
	duration time.Duration

	manual bool

	// lead is footage captured before the trigger, prepended to the clip. The
	// recorder takes ownership and closes each frame.
	lead []gocv.Mat
}

// recorder writes frames to a video file on its own goroutine so the capture
// loop is never blocked by disk or encoder latency.
//
// The previous Go build created a VideoWriter, slept for the clip duration, and
// closed it without ever writing a frame, producing empty files.
type recorder struct {
	frames chan gocv.Mat
	stop   chan struct{}
	done   chan struct{}

	request clipRequest

	// droppedFrames counts frames discarded because the encoder fell behind.
	// The previous build used an unbounded queue, so its documented
	// frame-dropping path could never run and memory grew without limit
	// instead.
	droppedFrames atomic.Int64

	writeError error
}

// startRecorder begins writing a clip. Call submit to feed it live frames and
// stop to finish a manual clip.
func startRecorder(request clipRequest) *recorder {
	// Two seconds of slack absorbs encoder stalls without letting a backlog
	// grow unboundedly.
	capacity := int(request.frameRate * 2)
	if capacity < 8 {
		capacity = 8
	}

	activeRecorder := &recorder{
		frames:  make(chan gocv.Mat, capacity),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		request: request,
	}

	go activeRecorder.run()

	return activeRecorder
}

// submit offers a frame to the recorder, cloning it so the capture loop may
// reuse its own Mat. A frame offered while the encoder is saturated is dropped
// rather than stalling capture.
func (r *recorder) submit(frame gocv.Mat) {
	clone := frame.Clone()

	select {
	case r.frames <- clone:
	default:
		clone.Close()
		r.droppedFrames.Add(1)
	}
}

// stopRecording ends a manual clip. It is safe to call more than once.
func (r *recorder) stopRecording() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

// wait blocks until the clip is fully written and returns any write error.
func (r *recorder) wait() error {
	<-r.done
	return r.writeError
}

func (r *recorder) run() {
	defer close(r.done)

	writer, err := gocv.VideoWriterFile(
		r.request.path,
		"mp4v",
		r.request.frameRate,
		r.request.frameSize.X,
		r.request.frameSize.Y,
		true,
	)
	if err != nil {
		r.writeError = fmt.Errorf("opening %s: %w", r.request.path, err)
		r.releasePending()
		return
	}
	defer writer.Close()

	for _, frame := range r.request.lead {
		if frame.Empty() {
			frame.Close()
			continue
		}
		if err := writer.Write(frame); err != nil {
			r.writeError = err
		}
		frame.Close()
	}

	if r.request.manual {
		r.writeUntilStopped(writer)
	} else {
		r.writeForDuration(writer, r.request.duration)
	}

	r.releasePending()
}

func (r *recorder) writeUntilStopped(writer *gocv.VideoWriter) {
	for {
		select {
		case frame := <-r.frames:
			r.writeFrame(writer, frame)
		case <-r.stop:
			// Flush whatever is already queued so the clip does not lose its
			// final moments.
			r.flush(writer)
			return
		}
	}
}

func (r *recorder) writeForDuration(writer *gocv.VideoWriter, duration time.Duration) {
	deadline := time.NewTimer(duration)
	defer deadline.Stop()

	for {
		select {
		case frame := <-r.frames:
			r.writeFrame(writer, frame)
		case <-r.stop:
			r.flush(writer)
			return
		case <-deadline.C:
			r.flush(writer)
			return
		}
	}
}

func (r *recorder) flush(writer *gocv.VideoWriter) {
	for {
		select {
		case frame := <-r.frames:
			r.writeFrame(writer, frame)
		default:
			return
		}
	}
}

func (r *recorder) writeFrame(writer *gocv.VideoWriter, frame gocv.Mat) {
	defer frame.Close()

	if frame.Empty() {
		return
	}
	if err := writer.Write(frame); err != nil && r.writeError == nil {
		r.writeError = err
	}
}

// releasePending closes any frame still queued, so a failed or finished clip
// never leaks the OpenCV buffers behind them.
func (r *recorder) releasePending() {
	for {
		select {
		case frame := <-r.frames:
			frame.Close()
		default:
			return
		}
	}
}
