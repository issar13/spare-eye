//go:build !windows

package detector

// pinCameraThread does nothing away from Windows. V4L2 owns no per-thread
// state, so the capture handle is valid on whichever thread the runtime picks.
func pinCameraThread() func() {
	return func() {}
}
