package signal

import (
	"time"
)

// Event is the interface satisfied by every typed event emitted through
// [Client.Events]. Switch on the concrete type to handle each kind.
type Event interface {
	// eventTimestamp returns the envelope or content timestamp so callers
	// can order or deduplicate. Unexported to keep the interface sealed.
	eventTimestamp() time.Time
}

// MessageEvent represents a received text message (DataMessage with a
// non-empty body).
type MessageEvent struct {
	// Sender is the ACI UUID of the message author.
	Sender string
	// SenderDevice is the device number that sent the message.
	SenderDevice uint32
	// Timestamp is the sender-side message timestamp.
	Timestamp time.Time
	// ServerTimestamp is the server-side receive time.
	ServerTimestamp time.Time
	// Body is the plaintext message text.
	Body string
	// GroupID is non-empty for group messages (hex-encoded group v2 master key).
	GroupID string
	// ExpiresIn is the disappearing-message timer, if set.
	ExpiresIn time.Duration
}

func (e *MessageEvent) eventTimestamp() time.Time { return e.Timestamp }

// ReceiptType distinguishes delivery, read, and viewed receipts.
type ReceiptType int

const (
	// ReceiptDelivery indicates the message was delivered to the recipient.
	ReceiptDelivery ReceiptType = iota
	// ReceiptRead indicates the recipient read the message.
	ReceiptRead
	// ReceiptViewed indicates the recipient viewed the message (e.g. a
	// view-once media).
	ReceiptViewed
)

// ReceiptEvent represents a delivery, read, or viewed receipt.
type ReceiptEvent struct {
	// Sender is the ACI UUID of the receipt author.
	Sender string
	// SenderDevice is the device that generated the receipt.
	SenderDevice uint32
	// Type is delivery, read, or viewed.
	Type ReceiptType
	// Timestamps are the sender-side timestamps of the messages being
	// acknowledged.
	Timestamps []time.Time
}

func (e *ReceiptEvent) eventTimestamp() time.Time {
	if len(e.Timestamps) > 0 {
		return e.Timestamps[0]
	}
	return time.Time{}
}

// TypingAction distinguishes started vs stopped typing indicators.
type TypingAction int

const (
	// TypingStarted means the sender began typing.
	TypingStarted TypingAction = iota
	// TypingStopped means the sender stopped typing.
	TypingStopped
)

// TypingEvent represents a typing indicator.
type TypingEvent struct {
	// Sender is the ACI UUID.
	Sender string
	// SenderDevice is the device number.
	SenderDevice uint32
	// Action is started or stopped.
	Action TypingAction
	// Timestamp is the typing indicator's timestamp.
	Timestamp time.Time
	// GroupID is non-empty for group typing events.
	GroupID string
}

func (e *TypingEvent) eventTimestamp() time.Time { return e.Timestamp }

// SyncMessageEvent represents a sync message from the user's own account
// (sent from another linked device).
type SyncMessageEvent struct {
	// SenderDevice is the device that originated the sync.
	SenderDevice uint32
	// Timestamp is the envelope timestamp.
	Timestamp time.Time
	// SentBody is set for sent-transcript syncs (the message text we sent
	// from another device).
	SentBody string
	// SentTo is the destination ACI for sent-transcript syncs.
	SentTo string
	// ReadTimestamps are the read-receipt timestamps for read syncs.
	ReadTimestamps []time.Time
}

func (e *SyncMessageEvent) eventTimestamp() time.Time { return e.Timestamp }

// DecryptionErrorEvent is emitted when an inbound envelope could not be
// decrypted. The receive loop continues; this event lets callers surface
// the error without losing the connection.
type DecryptionErrorEvent struct {
	// Sender is the ACI UUID of the envelope sender (may be empty for
	// sealed-sender messages where even the sender is unknown).
	Sender string
	// SenderDevice is the device number (0 if unknown).
	SenderDevice uint32
	// Timestamp is the envelope timestamp.
	Timestamp time.Time
	// Err is the underlying decryption error.
	Err error
}

func (e *DecryptionErrorEvent) eventTimestamp() time.Time { return e.Timestamp }

// Error implements error for [DecryptionErrorEvent] so callers can treat
// it as an error if desired.
func (e *DecryptionErrorEvent) Error() string {
	if e.Err != nil {
		return "decryption error: " + e.Err.Error()
	}
	return "decryption error: unknown"
}

// Unwrap supports [errors.Is] / [errors.As] chains.
func (e *DecryptionErrorEvent) Unwrap() error { return e.Err }

// QueueEmptyEvent is emitted once after connection (or reconnection) when
// the server confirms all queued envelopes have been delivered.
type QueueEmptyEvent struct {
	Timestamp time.Time
}

func (e *QueueEmptyEvent) eventTimestamp() time.Time { return e.Timestamp }

// ReactionEvent represents an inbound reaction to a previously-sent message.
//
// Reactions arrive as DataMessage payloads with a populated Reaction field
// (and no body text); they are dispatched as ReactionEvent rather than
// MessageEvent so handlers can treat them distinctly.
type ReactionEvent struct {
	// Sender is the ACI UUID of the reacting user.
	Sender string
	// SenderDevice is the device that produced the reaction.
	SenderDevice uint32
	// Timestamp is the reaction's own send time.
	Timestamp time.Time
	// ServerTimestamp is the server-side receive time.
	ServerTimestamp time.Time
	// Emoji is the reaction emoji (UTF-8 string). Empty when Remove is
	// true and the reactor did not specify which emoji to remove.
	Emoji string
	// Remove is true when the sender is removing a previous reaction
	// rather than adding one.
	Remove bool
	// TargetAuthorACI is the ACI UUID of the message being reacted to.
	TargetAuthorACI string
	// TargetTimestamp is the sender-side timestamp of the message being
	// reacted to (the conversation-level identifier).
	TargetTimestamp time.Time
	// GroupID is non-empty for reactions in group threads (hex-encoded
	// group v2 master key).
	GroupID string
}

func (e *ReactionEvent) eventTimestamp() time.Time { return e.Timestamp }

// EditMessageEvent represents an inbound edit of a previously-sent message.
//
// Edits arrive as the EditMessage Content variant (not DataMessage). The
// new body, target sent timestamp, and edit timestamp are surfaced
// directly; richer fields (mentions, attachments) are available via the
// raw protobuf accessors when needed.
type EditMessageEvent struct {
	// Sender is the ACI UUID of the editor.
	Sender string
	// SenderDevice is the device that produced the edit.
	SenderDevice uint32
	// Timestamp is the edit's send time (used as the conversation-level
	// identifier of the edit itself).
	Timestamp time.Time
	// ServerTimestamp is the server-side receive time.
	ServerTimestamp time.Time
	// TargetTimestamp is the sender-side timestamp of the original
	// message being replaced.
	TargetTimestamp time.Time
	// NewBody is the edited plaintext body. Empty if the edit removes
	// the body without supplying a new one.
	NewBody string
	// GroupID is non-empty for edits in group threads.
	GroupID string
}

func (e *EditMessageEvent) eventTimestamp() time.Time { return e.Timestamp }
