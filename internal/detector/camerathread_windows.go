//go:build windows

package detector

import "runtime"

// pinCameraThread keeps every call into the capture backend on one OS thread
// until the returned function runs.
//
// OpenCV's Windows backend here is DirectShow, which is COM. A COM object
// belongs to the apartment of the thread that created it, and releasing it from
// another thread is undefined. Go moves goroutines between OS threads whenever
// it schedules them, so opening a device on one thread and closing it on
// another is entirely possible — and when it happens the process dies inside
// the driver, with no Go panic to recover and no error to return.
func pinCameraThread() func() {
	runtime.LockOSThread()
	return runtime.UnlockOSThread
}
