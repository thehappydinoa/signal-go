# ADR 0016 — Control messages, reactions, and edits

**Status:** Accepted
**Date:** 2026-05-22

## Context

Phase 4 (`Send 1:1`) shipped text-message send. Phase 6 (`Bot framework`)
shipped text-event dispatch. Real bots also need to:

1. Send **delivery / read / viewed receipts** in response to inbound
   messages, so the sender's UI updates correctly.
2. Send **typing indicators** to feed back "the bot is composing a
   response" while a slow handler runs.
3. Send and receive **reactions** (`👍`, `❤️`, …) for a thumbs-up bot
   pattern that's common across Slack/Telegram bots.
4. Receive **edit events** so handlers don't double-fire on the same
   logical message after the user fixes a typo.

All four of these ride the same encrypted-envelope pipeline that
`Client.Send` already implements: discover devices, establish sessions,
encrypt content per-device, PUT to `/v1/messages/{aci}`. Only the
`Content` protobuf body and a few request flags differ.

The wire-format details:

- **Receipts** are a top-level `Content.ReceiptMessage` carrying the
  acknowledged-message timestamps and a kind enum
  (`DELIVERY` / `READ` / `VIEWED`). Sent with `urgent=false`,
  `silent=true`.
- **Typing indicators** are a top-level `Content.TypingMessage` with
  an `action` (`STARTED` / `STOPPED`), the indicator's own timestamp,
  and an optional group id. Sent with `urgent=false`, `online=true`,
  `silent=true` — the server drops them if the recipient is offline.
- **Reactions** are *not* a top-level Content variant. They ride
  inside a `DataMessage.Reaction` (alongside fields the server
  treats as just another message — the recipient client recognizes
  it as a reaction by structural inspection). Sent with `urgent=true`.
- **Edits** are a top-level `Content.EditMessage` with a target sent
  timestamp + a wrapped `DataMessage` carrying the new body.

## Decision

### Public send API

Four `*signal.Client` methods, mirroring the `Send(ctx, recipient,
text)` shape:

```go
func (c *Client) Send(ctx, recipientACI, text string) (Receipt, error)
func (c *Client) SendReceipt(ctx, recipientACI string, kind ReceiptType, timestamps []time.Time) (Receipt, error)
func (c *Client) SendTyping(ctx, recipientACI string, action TypingAction) (Receipt, error)
func (c *Client) SendReaction(ctx, recipientACI, emoji, targetAuthorACI string, targetTS time.Time, remove bool) (Receipt, error)
```

All four share a private `sendContent(ctx, recipient, contentBytes,
ts, deliveryOpts)` pipeline that handles device discovery, session
establishment, sealed-sender selection (when the recipient's UAK is
known), and at-most-one retry on 409/410. Only the marshalled
`Content` and the per-kind request flags (`urgent` / `online` /
`silent`) vary — those are bundled into a `deliveryOpts` struct.

`Edit` send is **not** in this slice. Composing an edit means matching
a target message timestamp and replacing its body via the bot
framework's higher-level conversation state, which lands later. The
`*signal.Client` API for edit-send will follow the same shape when
that ADR lands.

### Public receive API

Two new typed events on `Client.Events()`:

```go
type ReactionEvent struct {
    Sender, TargetAuthorACI, GroupID string
    SenderDevice                     uint32
    Emoji                            string
    Remove                           bool
    Timestamp, ServerTimestamp       time.Time
    TargetTimestamp                  time.Time
}

type EditMessageEvent struct {
    Sender, GroupID                  string
    SenderDevice                     uint32
    NewBody                          string
    Timestamp, ServerTimestamp       time.Time
    TargetTimestamp                  time.Time
}
```

Inbound receipts already surface as `*ReceiptEvent`; inbound typing
already surfaces as `*TypingEvent`. The receive-side dispatcher in
`pkg/signal` is updated to:

- Treat a `DataMessage` whose `Reaction` field is set as a
  `*ReactionEvent` (instead of a `*MessageEvent` with empty body —
  this would otherwise fire `OnText("")` matchers).
- Treat a top-level `EditMessage` as a `*EditMessageEvent`.

### Public bot API

Three new dispatcher entry points on `*bot.Bot`:

```go
b.OnReaction("👍").DM().Do(func(ctx, r *bot.Reaction) error { ... })
b.OnAnyReaction().Do(...)
b.OnEdit().Do(func(ctx, e *bot.Edit) error { ... })
```

All share the same `DM` / `Group` / `From(aci)` scope helpers used by
text dispatchers. Removals (`Reaction.Remove == true`) are skipped
by default; chain `.IncludeRemovals()` to receive them.

Five new helpers on `*bot.Message` for the response side:

```go
m.React(ctx, "👍")        // SendReaction
m.Unreact(ctx, "👍")      // SendReaction(remove=true)
m.Typing(ctx, action)     // SendTyping
m.MarkRead(ctx)           // SendReceipt(READ, [m.Timestamp])
m.MarkViewed(ctx)         // SendReceipt(VIEWED, [m.Timestamp])
```

All five return `ErrReplyNotSupportedInGroup` for group messages
until Phase 5 lands the group-send path, matching the existing
`m.Reply` semantic.

### Why dispatch reactions separately

Surfacing a reaction-only DataMessage as `*MessageEvent{Body: ""}`
would silently fire `OnText("")` matchers and confuse handlers that
inspect `m.Body()`. A dedicated `*ReactionEvent` preserves the
type-safe dispatcher contract.

### Why `pkg/signal.Client` not a sub-package

Receipts/typing/reactions are part of the same wire-format protocol
as `Send`, share its session machinery, and benefit from the same
sealed-sender + retry behaviour. Splitting them into a sub-package
would duplicate `discoverAndEnsureSessions`, the cert cache, and the
device-cache — for no observable API gain.

## Consequences

- The `Client` interface that `pkg/bot` consumes grows by three
  methods. Every test stub (`fakeClient` in `pkg/bot/bot_test.go`)
  must implement them. New stubs are trivial wrappers; existing
  callers that embed `*signal.Client` are unaffected.
- The receive dispatcher's `*MessageEvent` no longer fires for
  reaction-only `DataMessage`s. Callers who *want* the reaction as a
  message-event (very unusual) must read `Underlying().Events()`
  and re-route.
- `padContent` is applied to all encrypted Content uniformly. This
  matches what Signal-Android and signalmeow do; if it ever differs
  for one of the new kinds we'll re-revisit.
- Group reactions, group typing, and group edits all require the
  Phase 5 group-send path. The current `*Message` helpers fail
  closed on group threads; the bot dispatchers themselves accept
  inbound reactions/edits in groups (the wire-level decode works
  today).
- 1:1 edit-message *send* is implemented as `signal.Client.SendEdit`
  (`Content.editMessage` with `targetSentTimestamp` + nested
  `DataMessage`). Group edit-send remains out of scope until a
  dedicated group wire path exists.
