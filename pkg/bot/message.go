package bot

import (
	"context"
	"time"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// Message is the per-event handle handed to a [HandlerFunc]. It carries
// the typed event for inspection and a back-pointer to the Bot so
// helpers like Reply can route through the same client.
type Message struct {
	event *signal.MessageEvent
	bot   *Bot
}

// Sender returns the ACI UUID of the message author.
func (m *Message) Sender() string { return m.event.Sender }

// SenderDevice returns the device number that sent the message.
func (m *Message) SenderDevice() uint32 { return m.event.SenderDevice }

// Body returns the plaintext message body.
func (m *Message) Body() string { return m.event.Body }

// Timestamp returns the sender-side message timestamp.
func (m *Message) Timestamp() time.Time { return m.event.Timestamp }

// IsGroup reports whether the message arrived in a group thread.
func (m *Message) IsGroup() bool { return m.event.GroupID != "" }

// GroupID returns the group v2 master key (hex) for group messages, or
// the empty string for 1:1 DMs.
func (m *Message) GroupID() string { return m.event.GroupID }

// Reply sends a 1:1 text message back to the original sender.
//
// Group replies are not yet supported (they need group v2 — Phase 5).
// Calling Reply on a group message returns an error; use the lower-
// level [signal.Client] until then.
func (m *Message) Reply(ctx context.Context, text string) error {
	if m.IsGroup() {
		return ErrReplyNotSupportedInGroup
	}
	_, err := m.bot.cli.Send(ctx, m.event.Sender, text)
	return err
}

// Event returns the wrapped typed event for callers that want the
// fields not exposed via the Message helpers.
func (m *Message) Event() *signal.MessageEvent { return m.event }
