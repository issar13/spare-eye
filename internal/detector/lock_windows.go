//go:build windows

package detector

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockExclusive takes a non-blocking lock on the whole file. Windows releases
// it when the handle closes, including on a crash, so a stale lock file left
// behind by a killed process cannot wedge the next launch.
func lockExclusive(file *os.File) error {
	const wholeFile = ^uint32(0)

	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)

	err := windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, wholeFile, wholeFile, overlapped)

	// A range another handle already holds is refused with ERROR_LOCK_VIOLATION.
	// ERROR_IO_PENDING means the request was queued rather than granted, which
	// with LOCKFILE_FAIL_IMMEDIATELY can only be a conflicting holder as well,
	// and both have to map to ErrAlreadyRunning: the UI refuses to arm on that
	// error alone, and reports anything else as a lock that could not be taken.
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrAlreadyRunning
	}

	return err
}
