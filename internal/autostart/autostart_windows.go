//go:build windows

package autostart

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

// The Run key under HKEY_CURRENT_USER starts a program when this user logs in.
// The machine-wide key under HKEY_LOCAL_MACHINE would need administrator
// rights, which this app never asks for: it installs per-user into
// %LocalAppData%\Programs, so a machine-wide entry would fail to be written and
// would name a path other accounts cannot reach anyway.
const (
	runKeyPath            = `Software\Microsoft\Windows\CurrentVersion\Run`
	runKeyValueName       = "SpareEye"
	legacyRunKeyValueName = "KaaRada"
)

// Enable adds this binary to the current user's Run key.
func Enable() error {
	executable, err := executablePath()
	if err != nil {
		return err
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening the Run key: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(runKeyValueName, commandLine(executable)); err != nil {
		return fmt.Errorf("writing the %s Run value: %w", runKeyValueName, err)
	}
	if err := key.DeleteValue(legacyRunKeyValueName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the legacy %s Run value: %w", legacyRunKeyValueName, err)
	}

	return nil
}

// Disable removes the Run value. A value that is already gone, or a Run key
// that does not exist at all, is the state Disable exists to reach.
func Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening the Run key: %w", err)
	}
	defer key.Close()

	if err := key.DeleteValue(runKeyValueName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the %s Run value: %w", runKeyValueName, err)
	}
	if err := key.DeleteValue(legacyRunKeyValueName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing the legacy %s Run value: %w", legacyRunKeyValueName, err)
	}

	return nil
}

// Enabled reports whether the Run key will start this exact binary.
//
// A value naming some other path is reported as not enabled: it was written
// for a copy that has since been moved, replaced or deleted, and treating it
// as registered would show the user a switch that is on while nothing starts.
func Enabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("opening the Run key: %w", err)
	}
	defer key.Close()

	registered, _, err := key.GetStringValue(runKeyValueName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the %s Run value: %w", runKeyValueName, err)
	}

	executable, err := executablePath()
	if err != nil {
		return false, err
	}

	return registered == commandLine(executable), nil
}

// commandLine is what Windows runs at login. The path is quoted because the
// per-user install directory is %LocalAppData%\Programs\SpareEye, and an
// unquoted value containing a space is read as a command followed by arguments.
func commandLine(executable string) string {
	return fmt.Sprintf("%q %s", executable, StartedAtLoginFlag)
}
