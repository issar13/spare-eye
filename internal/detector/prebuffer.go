package detector

import (
	"sync"

	"gocv.io/x/gocv"
)

// prebuffer holds the most recent frames so a clip can begin slightly before
// the moment motion was confirmed.
//
// The previous build used a 300-frame buffer and prepended all of it to every
// clip, which both prepended up to 37 seconds of unwanted history and held
// roughly 276 MB of frames at VGA resolution. This buffer is instead bounded by
// a wall-clock lead time, so its size follows the camera's real frame rate.
type prebuffer struct {
	mutex  sync.Mutex
	frames []gocv.Mat
	limit  int
}

func newPrebuffer(limit int) *prebuffer {
	if limit < 1 {
		limit = 1
	}
	return &prebuffer{
		frames: make([]gocv.Mat, 0, limit),
		limit:  limit,
	}
}

// add stores a clone of frame, discarding and releasing the oldest frame once
// the buffer is full. The caller keeps ownership of frame.
func (p *prebuffer) add(frame gocv.Mat) {
	clone := frame.Clone()

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.frames) == p.limit {
		oldest := p.frames[0]
		p.frames = p.frames[1:]
		oldest.Close()
	}

	p.frames = append(p.frames, clone)
}

// drain removes every buffered frame and transfers ownership to the caller,
// which must Close each returned Mat.
func (p *prebuffer) drain() []gocv.Mat {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	drained := p.frames
	p.frames = make([]gocv.Mat, 0, p.limit)

	return drained
}

// close releases every buffered frame.
func (p *prebuffer) close() {
	for _, frame := range p.drain() {
		frame.Close()
	}
}
