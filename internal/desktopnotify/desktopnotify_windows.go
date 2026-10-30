//go:build windows

package desktopnotify

import (
	"fmt"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
)

// Show pushes a toast. The library falls back to PowerShell when the app has no
// registered identity, which is the case for an unpackaged per-user install.
func Show(message Message) error {
	notification := toast.Notification{
		AppID: "SpareEye",
		Title: message.Title,
		Body:  message.Body,
		Icon:  message.ImagePath,
	}

	if err := notification.Push(); err != nil {
		return fmt.Errorf("pushing the toast: %w", err)
	}

	return nil
}
