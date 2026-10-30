package detector

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"gocv.io/x/gocv"

	"spareeye/internal/media"
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

	// poster is the frame current when the clip started, saved as a JPEG beside
	// the clips because the webview cannot decode the mp4v video itself. The
	// recorder takes ownership and closes it.
	poster gocv.Mat
}

// recorder writes frames to a video file on its own goroutine so the capture
// loop is never blocked by disk or encoder latency.
//
// Every frame handed to the recorder is written. A VideoWriter that is opened,
// left idle for the clip duration and then closed produces a file that exists,
// has a plausible size, and contains nothing decodable.
type recorder struct {
	frames chan gocv.Mat
	stop   chan struct{}
	done   chan struct{}

	request clipRequest

	// droppedFrames counts frames discarded because the encoder fell behind.
	// The queue this counts against must stay bounded: an unbounded one never
	// drops anything, so memory grows without limit rather than the load being
	// shed, and this counter would sit at zero while the process swelled.
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

	r.writePoster()

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

// writePoster saves and releases the poster frame. A failure is deliberately
// ignored: a clip without a poster still plays, and the UI falls back to a
// plain tile, so there is nothing worth interrupting the recording for.
func (r *recorder) writePoster() {
	defer r.request.poster.Close()

	if r.request.poster.Empty() {
		return
	}

	posterPath := media.PosterPath(r.request.path)
	if err := os.MkdirAll(filepath.Dir(posterPath), 0o700); err != nil {
		return
	}
	gocv.IMWrite(posterPath, r.request.poster)
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
