//go:build !linux && !windows

package desktopnotify

import "errors"

// errUnsupported keeps the package building everywhere. macOS would need a
// signed app bundle for the notification centre to accept anything from it.
var errUnsupported = errors.New("desktop notifications are not supported on this platform yet")

func Show(Message) error { return errUnsupported }
