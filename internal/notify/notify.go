// Package notify delivers motion alerts to a messaging provider and receives
// commands back from it. Providers are interchangeable so the detector never
// needs to know whether the user chose Telegram or WhatsApp.
package notify

import "context"

// Command is an instruction the user sent from their phone.
type Command string

const (
	CommandStartRecording Command = "record"
	CommandStopRecording  Command = "stop"
	CommandListRecordings Command = "list"
)

// Attachment is a file to deliver alongside a message. Path points at a file on
// local disk; Kind tells the provider which upload endpoint to use.
type Attachment struct {
	Path string
	Kind AttachmentKind
}

type AttachmentKind int

const (
	AttachmentImage AttachmentKind = iota
	AttachmentVideo
)

// Selection is a numbered choice the user made from a listing we sent them.
type Selection struct {
	Index int
}

// Notifier is one configured messaging provider.
type Notifier interface {
	// Send delivers a message, optionally with one attachment. A nil
	// attachment sends text only.
	Send(ctx context.Context, message string, attachment *Attachment) error

	// Commands returns a channel of commands from the authenticated user.
	// Messages from anyone else are discarded before reaching the channel.
	Commands(ctx context.Context) (<-chan Incoming, error)

	// Name identifies the provider in logs and in the UI.
	Name() string

	// Close releases connections and stops any polling.
	Close() error
}

// Incoming is one authenticated inbound message, parsed into either a known
// command or a numeric selection from a listing.
type Incoming struct {
	Command   Command
	Selection *Selection
}
