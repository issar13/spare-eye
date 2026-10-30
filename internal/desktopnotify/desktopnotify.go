// Package desktopnotify shows a notification on the computer SpareEye runs on.
//
// It is the alert channel that needs no account and sends nothing anywhere, so
// it works for somebody who has not set up Telegram. Each platform lives in its
// own build-tagged file, the way internal/autostart does, and platforms with no
// implementation report that rather than failing to compile.
package desktopnotify

// Message is one notification. ImagePath is optional and names a file on this
// computer, which is shown beside the text where the platform supports it.
type Message struct {
	Title     string
	Body      string
	ImagePath string
}
