//go:build linux

package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateConfigDirectory points os.UserConfigDir at a per-test temporary
// directory. Every test here writes an autostart entry, and without this they
// would register the test binary in the developer's real session, which would
// then try to run a deleted file at every login.
func isolateConfigDirectory(t *testing.T) string {
	t.Helper()

	temporaryHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", temporaryHome)

	return temporaryHome
}

func TestEnableWritesADesktopEntryThatEnabledRecognises(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)

	if err := Enable(); err != nil {
		t.Fatalf("Enable() returned %v", err)
	}

	path := filepath.Join(temporaryHome, autostartDirectoryName, desktopEntryName)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Enable() did not write %s: %v", path, err)
	}

	entry := string(contents)
	for _, required := range []string{"[Desktop Entry]", "Type=Application", "Name=Kaa Rada", "X-GNOME-Autostart-enabled=true"} {
		if !strings.Contains(entry, required) {
			t.Errorf("the entry is missing %q:\n%s", required, entry)
		}
	}

	executable, err := executablePath()
	if err != nil {
		t.Fatalf("executablePath() returned %v", err)
	}

	// The quotes matter: an install path is allowed to contain a space, and an
	// unquoted one would be read as a command plus arguments.
	wantedCommand := "Exec=\"" + executable + "\" " + StartedAtLoginFlag
	if !strings.Contains(entry, wantedCommand) {
		t.Errorf("the entry runs something other than %q:\n%s", wantedCommand, entry)
	}

	enabled, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled() returned %v", err)
	}
	if !enabled {
		t.Error("Enabled() = false directly after Enable()")
	}
}

func TestDisableRemovesTheEntryAndIsSafeToRepeat(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)

	if err := Enable(); err != nil {
		t.Fatalf("Enable() returned %v", err)
	}
	if err := Disable(); err != nil {
		t.Fatalf("Disable() returned %v", err)
	}

	path := filepath.Join(temporaryHome, autostartDirectoryName, desktopEntryName)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Disable() left %s behind", path)
	}

	// Turning the switch off twice reaches the state the user asked for both
	// times, so the second call is not a failure.
	if err := Disable(); err != nil {
		t.Errorf("a second Disable() returned %v", err)
	}

	enabled, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled() returned %v", err)
	}
	if enabled {
		t.Error("Enabled() = true after Disable()")
	}
}

func TestEnabledIsFalseWithNothingRegistered(t *testing.T) {
	isolateConfigDirectory(t)

	enabled, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled() returned %v", err)
	}
	if enabled {
		t.Error("Enabled() = true with no autostart directory at all")
	}
}

func TestAnEntryNamingAnotherBinaryIsNotEnabled(t *testing.T) {
	temporaryHome := isolateConfigDirectory(t)

	// What an entry written before the app was moved, replaced by an installer,
	// or deleted outright looks like. Reporting it as enabled would show the
	// user a switch that is on while nothing starts at login.
	directory := filepath.Join(temporaryHome, autostartDirectoryName)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("creating %s: %v", directory, err)
	}

	stale := desktopEntry(filepath.Join(temporaryHome, "somewhere", "else", "kaa-rada"))
	if err := os.WriteFile(filepath.Join(directory, desktopEntryName), []byte(stale), 0o644); err != nil {
		t.Fatalf("writing the stale entry: %v", err)
	}

	enabled, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled() returned %v", err)
	}
	if enabled {
		t.Error("Enabled() = true for an entry naming a different binary")
	}
}
