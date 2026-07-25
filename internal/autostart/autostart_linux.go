//go:build linux

package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	autostartDirectoryName = "autostart"
	desktopEntryName       = "kaa-rada.desktop"
)

// Enable writes an XDG autostart entry, which every mainstream Linux desktop
// reads when a session begins.
func Enable() error {
	executable, err := executablePath()
	if err != nil {
		return err
	}

	path, err := desktopEntryPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(desktopEntry(executable)), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// Disable removes the autostart entry. An entry that is already gone is the
// state Disable exists to reach, so it is not an error.
func Disable() error {
	path, err := desktopEntryPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", path, err)
	}

	return nil
}

// Enabled reports whether the session will start this exact binary.
//
// An entry naming some other path is reported as not enabled: it was written
// for a copy that has since been moved, replaced or deleted, and treating it
// as registered would show the user a switch that is on while nothing starts.
func Enabled() (bool, error) {
	path, err := desktopEntryPath()
	if err != nil {
		return false, err
	}

	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	executable, err := executablePath()
	if err != nil {
		return false, err
	}

	return strings.Contains(string(contents), commandLine(executable)+"\n"), nil
}

func desktopEntryPath() (string, error) {
	// os.UserConfigDir is what internal/config uses to find config.json, so a
	// user who has moved XDG_CONFIG_HOME gets both in the same place.
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating user config directory: %w", err)
	}

	return filepath.Join(base, autostartDirectoryName, desktopEntryName), nil
}

// commandLine is the Exec line the desktop environment runs. The path is
// quoted because an unpacked tarball can sit in a directory whose name
// contains a space, which would otherwise be read as the start of an argument.
func commandLine(executable string) string {
	return fmt.Sprintf("Exec=%q %s", executable, StartedAtLoginFlag)
}

func desktopEntry(executable string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Kaa Rada",
		commandLine(executable),
		// GNOME and its derivatives honour this key and ignore an entry that
		// lacks it once the user has switched the app off in their own
		// startup-applications screen.
		"X-GNOME-Autostart-enabled=true",
		"",
	}, "\n")
}
