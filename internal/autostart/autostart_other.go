//go:build !linux && !windows

package autostart

import "errors"

// errUnsupported keeps the package building everywhere, so nothing that calls
// it needs a build tag of its own. macOS would need a launch agent plist and
// the code signing that goes with it, which this project has not done yet.
var errUnsupported = errors.New("starting at login is not supported on this platform yet")

func Enable() error { return errUnsupported }

func Disable() error { return errUnsupported }

func Enabled() (bool, error) { return false, errUnsupported }
