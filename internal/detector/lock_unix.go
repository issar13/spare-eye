//go:build unix

package detector

import (
	"errors"
	"os"
	"syscall"
)

// lockExclusive takes a non-blocking advisory lock. The kernel releases it when
// the file descriptor closes, including on a crash, so a stale lock file left
// behind by a killed process cannot wedge the next launch.
func lockExclusive(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return ErrAlreadyRunning
	}

	return err
}
