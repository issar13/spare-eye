package detector

import (
	"fmt"
	"os"
	"path/filepath"
)

// lockFileName is the file whose lock proves ownership of the camera. It lives
// in the configuration directory rather than beside the media, because a user
// may point the media directory at a network share where locking is unreliable.
const lockFileName = "camera.lock"

// InstanceLock is a held claim on the camera, released when the process exits.
//
// Only one process can open a capture device, and a second one currently fails
// deep inside the OpenCV backend with a message that names neither the camera
// nor the other process. Taking this lock first turns that into an error the UI
// can explain.
type InstanceLock struct {
	file *os.File
}

// AcquireInstanceLock claims the camera for this process. It returns
// ErrAlreadyRunning when another instance already holds the claim.
func AcquireInstanceLock(directory string) (*InstanceLock, error) {
	path := filepath.Join(directory, lockFileName)

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", path, err)
	}

	if err := lockExclusive(file); err != nil {
		file.Close()
		return nil, err
	}

	// The previous holder's PID is still in the file; replacing it means a
	// support question can be answered by reading one file.
	file.Truncate(0)
	file.Seek(0, 0)
	fmt.Fprintf(file, "%d\n", os.Getpid())

	return &InstanceLock{file: file}, nil
}

// Release gives up the claim. The operating system would drop the lock when the
// process exits anyway, so this only matters for an orderly shutdown.
func (l *InstanceLock) Release() error {
	return l.file.Close()
}

// ErrAlreadyRunning reports that another instance holds the camera lock.
var ErrAlreadyRunning = fmt.Errorf(
	"another copy of the motion detector is already running\n" +
		"  • only one process can hold the camera at a time\n" +
		"  • close the other window, or stop a headless spareeye-cli run, then try again",
)
