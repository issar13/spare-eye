// Package autostart registers this executable with the operating system so it
// runs when the user logs in, and reports whether it currently is registered.
//
// A security camera that is only watching while somebody remembers to open it
// is not watching. Everything here is per-user: nothing needs administrator
// rights, and nothing is left behind for other accounts on the machine.
package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

// StartedAtLoginFlag is added to the registered command so the app can tell it
// was launched by the operating system rather than by the person sitting in
// front of it, and open its window out of the way.
const StartedAtLoginFlag = "--started-at-login"

// executablePath is the real location of the running binary.
//
// The registration outlives the shell that created it, so a relative path or a
// path chosen when this project was built would be pointing at nothing by the
// time the user next logs in. Symlinks are resolved because a Linux user may
// launch the app through one in ~/.local/bin while the binary itself lives in
// an unpacked tarball anywhere.
func executablePath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating this program: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", executable, err)
	}

	return resolved, nil
}
