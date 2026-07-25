package detector

import (
	"errors"
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
