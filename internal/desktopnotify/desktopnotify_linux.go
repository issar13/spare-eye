//go:build linux

package desktopnotify

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	notificationsService = "org.freedesktop.Notifications"
	notificationsPath    = "/org/freedesktop/Notifications"
	notifyMethod         = "org.freedesktop.Notifications.Notify"
	applicationName      = "SpareEye"

	// sendTimeout bounds the call so a notification daemon that has hung cannot
	// keep a goroutine alive for the rest of the session.
	sendTimeout = 5 * time.Second
)

// Show asks the session's notification daemon to display the message. Alerts
// are minutes apart at most, so a connection per call is simpler than holding
// one open and having to notice when the session bus restarts.
func Show(message Message) error {
	connection, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connecting to the session bus: %w", err)
	}
	defer connection.Close()

	hints := map[string]dbus.Variant{}
	if message.ImagePath != "" {
		hints["image-path"] = dbus.MakeVariant(message.ImagePath)
	}

	sendContext, done := context.WithTimeout(context.Background(), sendTimeout)
	defer done()

	call := connection.Object(notificationsService, notificationsPath).CallWithContext(
		sendContext,
		notifyMethod,
		0,
		applicationName,
		uint32(0),
		"",
		message.Title,
		message.Body,
		[]string{},
		hints,
		int32(-1),
	)
	if call.Err != nil {
		return fmt.Errorf("sending the notification: %w", call.Err)
	}

	return nil
}
