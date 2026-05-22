package bot

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
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

// Reply sends a text message back to the sender (1:1) or to the group
// thread (Groups v2 via [signal.Client.SendGroup]).
func (m *Message) Reply(ctx context.Context, text string) error {
	if m.IsGroup() {
		masterKey, err := decodeGroupMasterKey(m.event.GroupID)
		if err != nil {
			return err
		}
		_, err = m.bot.cli.SendGroup(ctx, masterKey, text)
		return err
	}
	_, err := m.bot.cli.Send(ctx, m.event.Sender, text)
	return err
}

// React reacts to this message with the given emoji on the sender's
// thread (1:1) or in the group thread (Groups v2).
func (m *Message) React(ctx context.Context, emoji string) error {
	if m.IsGroup() {
		masterKey, err := decodeGroupMasterKey(m.event.GroupID)
		if err != nil {
			return err
		}
		_, err = m.bot.cli.SendGroupReaction(ctx, masterKey, emoji, m.event.Sender, m.event.Timestamp, false)
		return err
	}
	_, err := m.bot.cli.SendReaction(ctx, m.event.Sender, emoji, m.event.Sender, m.event.Timestamp, false)
	return err
}

// Unreact removes a previously-sent reaction to this message. emoji
// may be empty if the recipient is expected to clear any prior
// reaction.
func (m *Message) Unreact(ctx context.Context, emoji string) error {
	if m.IsGroup() {
		masterKey, err := decodeGroupMasterKey(m.event.GroupID)
		if err != nil {
			return err
		}
		_, err = m.bot.cli.SendGroupReaction(ctx, masterKey, emoji, m.event.Sender, m.event.Timestamp, true)
		return err
	}
	_, err := m.bot.cli.SendReaction(ctx, m.event.Sender, emoji, m.event.Sender, m.event.Timestamp, true)
	return err
}

// Typing sends a started/stopped typing indicator to the message's
// sender (1:1) or group thread (Groups v2).
func (m *Message) Typing(ctx context.Context, action signal.TypingAction) error {
	if m.IsGroup() {
		masterKey, err := decodeGroupMasterKey(m.event.GroupID)
		if err != nil {
			return err
		}
		_, err = m.bot.cli.SendGroupTyping(ctx, masterKey, action)
		return err
	}
	_, err := m.bot.cli.SendTyping(ctx, m.event.Sender, action)
	return err
}

// MarkRead sends a READ receipt for this message back to its sender.
// In groups the receipt is delivered 1:1 to the message author, not the
// whole group.
func (m *Message) MarkRead(ctx context.Context) error {
	_, err := m.bot.cli.SendReceipt(ctx, m.event.Sender, signal.ReceiptRead, []time.Time{m.event.Timestamp})
	return err
}

// MarkViewed sends a VIEWED receipt for this message (typically for
// view-once media) back to its sender. In groups the receipt is
// delivered 1:1 to the message author.
func (m *Message) MarkViewed(ctx context.Context) error {
	_, err := m.bot.cli.SendReceipt(ctx, m.event.Sender, signal.ReceiptViewed, []time.Time{m.event.Timestamp})
	return err
}

// Event returns the wrapped typed event for callers that want the
// fields not exposed via the Message helpers.
func (m *Message) Event() *signal.MessageEvent { return m.event }

// ConvoKey returns the conversation key for this message: sender + group.
func (m *Message) ConvoKey() ConvoKey {
	return ConvoKey{Sender: m.event.Sender, GroupID: m.event.GroupID}
}

// Convo returns the [Convo] handle for this message's sender +
// group, providing per-conversation key/value state. Equivalent to
// b.Convo().For(m.ConvoKey()).
func (m *Message) Convo() *Convo {
	return m.bot.convo.For(m.ConvoKey())
}

func decodeGroupMasterKey(groupIDHex string) ([]byte, error) {
	masterKey, err := hex.DecodeString(groupIDHex)
	if err != nil {
		return nil, fmt.Errorf("bot: invalid group id: %w", err)
	}
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return nil, errors.New("bot: group id must be 32-byte master key")
	}
	return masterKey, nil
}

// Reaction is the per-event handle for reaction handlers registered
// via [Bot.OnReaction] / [Bot.OnAnyReaction].
type Reaction struct {
	event *signal.ReactionEvent
	bot   *Bot
}

// Sender returns the ACI UUID of the reacting user.
func (r *Reaction) Sender() string { return r.event.Sender }

// Emoji returns the reaction emoji.
func (r *Reaction) Emoji() string { return r.event.Emoji }

// IsRemoval reports whether the reaction is a removal of a prior one.
func (r *Reaction) IsRemoval() bool { return r.event.Remove }

// TargetAuthorACI returns the ACI of the message being reacted to.
func (r *Reaction) TargetAuthorACI() string { return r.event.TargetAuthorACI }

// TargetTimestamp returns the timestamp of the message being reacted to.
func (r *Reaction) TargetTimestamp() time.Time { return r.event.TargetTimestamp }

// IsGroup reports whether the reaction landed in a group thread.
func (r *Reaction) IsGroup() bool { return r.event.GroupID != "" }

// GroupID returns the group v2 master key (hex) when applicable.
func (r *Reaction) GroupID() string { return r.event.GroupID }

// Event returns the wrapped reaction event.
func (r *Reaction) Event() *signal.ReactionEvent { return r.event }

// ConvoKey returns the conversation key for this reaction: sender + group.
func (r *Reaction) ConvoKey() ConvoKey {
	return ConvoKey{Sender: r.event.Sender, GroupID: r.event.GroupID}
}

// Convo returns the [Convo] handle for this reaction's sender + group.
func (r *Reaction) Convo() *Convo { return r.bot.convo.For(r.ConvoKey()) }

// Edit is the per-event handle for edit handlers registered via
// [Bot.OnEdit].
type Edit struct {
	event *signal.EditMessageEvent
	bot   *Bot
}

// Sender returns the ACI UUID of the editing user.
func (e *Edit) Sender() string { return e.event.Sender }

// NewBody returns the edited body text.
func (e *Edit) NewBody() string { return e.event.NewBody }

// Timestamp returns the edit's send time (the conversation-level
// identifier of the edit).
func (e *Edit) Timestamp() time.Time { return e.event.Timestamp }

// TargetTimestamp returns the timestamp of the original message that
// is being replaced.
func (e *Edit) TargetTimestamp() time.Time { return e.event.TargetTimestamp }

// IsGroup reports whether the edit landed in a group thread.
func (e *Edit) IsGroup() bool { return e.event.GroupID != "" }

// GroupID returns the group v2 master key (hex) when applicable.
func (e *Edit) GroupID() string { return e.event.GroupID }

// Event returns the wrapped edit event.
func (e *Edit) Event() *signal.EditMessageEvent { return e.event }

// ConvoKey returns the conversation key for this edit: sender + group.
func (e *Edit) ConvoKey() ConvoKey {
	return ConvoKey{Sender: e.event.Sender, GroupID: e.event.GroupID}
}

// Convo returns the [Convo] handle for this edit's sender + group.
func (e *Edit) Convo() *Convo { return e.bot.convo.For(e.ConvoKey()) }
