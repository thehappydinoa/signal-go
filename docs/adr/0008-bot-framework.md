# ADR 0008 — Bot framework: `pkg/bot`

- Status: Accepted (API design only; implementation lands in Phase 6)
- Date: 2026-05-20

## Context

The primary downstream use of `signal-go` will be Signal bots. Hand-wiring
an event loop, decoding message types, filtering by group/DM, and routing
to handler functions is boilerplate every bot author repeats. Telegram-bot
and Slack Bolt have set the ergonomic bar.

We want `pkg/bot` to be a thin, composable layer on top of `pkg/signal`'s
event stream — not a separate framework that hides the underlying API. The
bot package depends on `pkg/signal`; `pkg/signal` does not know `pkg/bot`
exists.

## Decision

### Lifecycle

```go
b, err := bot.Open(ctx, bot.Options{
    StoreDir: "./.signal-data",
    Logger:   slog.Default(),
})
// ... register handlers ...
err = b.Run(ctx)   // blocks until ctx cancelled or fatal error
```

`Open` loads a previously-linked account from `StoreDir`. If none exists,
returns `ErrNotLinked`; callers can then run `signal.Link(...)` and try
again. (Helper `bot.LinkAndOpen` will do both in one call.)

### Handler registration

The default dispatch uses a fluent builder:

```go
b.On().Text("ping").Do(func(ctx context.Context, m *bot.Message) error {
    return m.Reply(ctx, "pong")
})

b.On().Command("weather").Do(func(ctx context.Context, m *bot.Message, args []string) error {
    if len(args) == 0 { return m.Reply(ctx, "usage: /weather <city>") }
    return m.Reply(ctx, "weather in "+args[0]+": ...")
})

b.On().Regex(regexp.MustCompile(`(?i)^remind (.+)$`)).
    Group().                    // only in group chats
    Do(func(ctx context.Context, m *bot.Message, match []string) error { ... })

b.On().Reaction().Do(func(ctx context.Context, r *bot.ReactionEvent) error { ... })
b.On().Edit().Do(func(ctx context.Context, e *bot.EditEvent) error { ... })
```

Scopes (`.Group()`, `.DM()`, `.From(addr)`, `.InGroup(id)`) compose with
matchers. Order of registration determines order of evaluation; the first
matching handler "wins" by default. `.Pass()` from a handler signals "I
didn't actually handle this, try the next one."

### Middleware

```go
b.Use(bot.Recover())               // turns panics into errors
b.Use(bot.RequestLogger(logger))   // structured per-message logging
b.Use(bot.RateLimit(10, time.Minute))
b.Use(myAuthMiddleware)
```

Middleware signature mirrors `http.Handler`'s wrap pattern but takes the
`bot.Message`:

```go
type Middleware func(next bot.HandlerFunc) bot.HandlerFunc
```

### Message API

```go
type Message struct {
    ID        string
    Sender    Address          // ACI/PNI + e164
    Group     *Group           // nil for DMs
    Timestamp time.Time
    Text      string
    Quote     *QuotedMessage
    // attachments, body ranges, etc.
}

func (m *Message) Reply(ctx context.Context, text string) error
func (m *Message) ReplyAttachment(ctx context.Context, file io.Reader, mime string) error
func (m *Message) React(ctx context.Context, emoji string) error
func (m *Message) Typing(ctx context.Context) (stop func(), err error)
func (m *Message) MarkRead(ctx context.Context) error
```

### Conversation state

Implemented in Phase 6. The shape is slightly different from the
original sketch — we expose a per-key handle rather than passing the
key to every call, which composes better with handler bodies that
already hold a `*Message`:

```go
type ConvoKey struct {
    Sender  string // ACI of the other party
    GroupID string // empty for DMs
}

type ConvoStore interface {
    Get(key ConvoKey, field string) (string, bool)
    Set(key ConvoKey, field, value string)
    Delete(key ConvoKey, field string)
    Clear(key ConvoKey)
    All(key ConvoKey) map[string]string
}

// Per-message scoped handle, plus stage helpers for FSM-style flows:
m.Convo().SetStage("awaiting_email")
v, ok := m.Convo().Get("email")

// Bot-wide handle for cross-conversation queries:
b.Convo().For(ConvoKey{Sender: "alice"}).Stage()
```

The `Match.Stage("awaiting_email")` / `Match.AnyStage()` filter wires
the stage into the dispatcher itself so handlers fire only when the
conversation is in a matching stage. FSM-style flows fall out of:

```go
b.OnCommand("signup").DM().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
    m.Convo().SetStage("await_email")
    return m.Reply(ctx, "what's your email?")
})
b.OnText("").Stage("await_email").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
    m.Convo().Set("email", m.Body())
    m.Convo().SetStage("await_age")
    return m.Reply(ctx, "and your age?")
})
```

Default impl (`bot.MemoryConvoStore`) is in-memory; bots that need
state to survive process restart can supply a custom `ConvoStore` via
`bot.Options.ConvoStore` (or `bot.WrapWithOptions` in tests).

Multi-step "wizard" sugar that auto-advances stages is a follow-up
once we've shaken out the bare primitives.

### Error handling

Handlers return errors. The default error policy logs the error and
continues. Users can install a custom error hook with `b.OnError(fn)`.

## Consequences

- **Pro**: Bot authors get a tight API similar to Telegram/Slack libs; no
  one writes an event-loop switch statement for the 100th time.
- **Pro**: Built on top of `pkg/signal` events, so power users can drop
  down to the lower layer for anything `bot` doesn't cover.
- **Con**: Adds API surface to maintain. Mitigation: keep the package tiny
  and let the dispatch logic live in clearly-tested files.
- **Con**: We must not finalize this API until `pkg/signal` events are
  stable (Phase 4). The ADR documents the intent; the package skeleton
  ships as a non-functional stub now, fleshed out in Phase 6.

## Not in scope

- Slash-command discovery / autocompletion (no Signal-side analog).
- Inline buttons / interactive UIs (Signal does not have these).
- Multi-account fleet management (one process = one linked device).
