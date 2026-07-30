//go:build !windows

package app

import (
	"fmt"
	"os"
)

// ReportFatalError tells the user startup failed. Everywhere but Windows the
// binary keeps a usable standard error whether it was launched from a terminal
// or a desktop launcher, so that is where this goes; a dialog would need a UI
// toolkit that startup has not reached the point of initialising.
func ReportFatalError(summary string, logPath string) {
	fmt.Fprintf(os.Stderr, "SpareEye could not start: %s\n", summary)

	if logPath != "" {
		fmt.Fprintf(os.Stderr, "The details were written to %s\n", logPath)
	}
}
