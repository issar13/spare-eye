//go:build windows

package app

import "golang.org/x/sys/windows"

const fatalCaption = "Kaa Rada could not start"

// ReportFatalError tells the user startup failed, in the only way a Windows
// desktop build can.
//
// The binary is linked for the GUI subsystem, so os.Stderr is not connected to
// anything: a printed message, and the runtime's own panic report, go nowhere.
// Without this the whole failure is a window that never appears.
func ReportFatalError(summary string, logPath string) {
	message := summary
	if logPath != "" {
		message += "\n\nThe details were written to:\n" + logPath
	}

	text, textError := windows.UTF16PtrFromString(message)
	caption, captionError := windows.UTF16PtrFromString(fatalCaption)
	if textError != nil || captionError != nil {
		return
	}

	windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}
