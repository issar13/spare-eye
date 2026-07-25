package detector

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestInstanceLockRefusesASecondHolder(t *testing.T) {
	directory := t.TempDir()

	first, err := AcquireInstanceLock(directory)
	if err != nil {
		t.Fatalf("acquiring the first lock: %v", err)
	}

	if _, err := AcquireInstanceLock(directory); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire returned %v, want ErrAlreadyRunning", err)
	}

	if err := first.Release(); err != nil {
		t.Fatalf("releasing: %v", err)
	}

	// Releasing must make the claim available again, or restarting the app
	// after a clean shutdown would be refused.
	second, err := AcquireInstanceLock(directory)
	if err != nil {
		t.Fatalf("acquiring after release: %v", err)
	}
	second.Release()
}

// TestALockFileThatCannotBeOpenedIsNotReportedAsASecondInstance guards the
// distinction the UI depends on: it refuses to arm on ErrAlreadyRunning alone,
// and a claim that could not be made at all says nothing about who has the
// camera.
func TestALockFileThatCannotBeOpenedIsNotReportedAsASecondInstance(t *testing.T) {
	unwritable := filepath.Join(t.TempDir(), "no-such-directory")

	_, err := AcquireInstanceLock(unwritable)
	if err == nil {
		t.Fatal("acquiring in a directory that does not exist succeeded")
	}
	if errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("an unopenable lock file was reported as another instance: %v", err)
	}
}
