# Creating a Signal bot

This guide walks through building a new bot on top of **signal-go**. It
assumes you have already completed the environment setup in
[Getting started](./getting-started.md) (Go 1.25+, cgo, `libsignal_ffi.a`).

## What you are building

A **signal-go bot** is a Go program that:

1. Runs as a **linked Signal secondary device** (same model as Signal Desktop).
2. Keeps encrypted account + protocol state on disk (SQLite store).
3. Listens on Signal's authenticated chat websocket for inbound events.
4. Sends replies with `pkg/signal` or the higher-level `pkg/bot` dispatcher.

There is no separate “bot API” from Signal — your program is a normal linked
device. Users message your bot's phone number (or a group it belongs to) like
any other contact.

## Choose your layer

| Layer | When to use |
|-------|-------------|
| **`pkg/bot`** | Most bots: command routing, DMs vs groups, conversation state, middleware. Start here. |
| **`pkg/signal` only** | Full control over the event loop, custom filtering, or non-message events you handle yourself. See [`examples/echo-bot`](../../examples/echo-bot/). |

The rest of this guide focuses on **`pkg/bot`**, which is what the shipped
examples use.

## One-time setup: link a device

Every bot needs a persisted store. Link once per bot identity:

```sh
task libsignal   # if you have not built libsignal_ffi.a yet
task build       # → bin/signal-go

./bin/signal-go link -store ./.my-bot
```

Scan the QR code from Signal → Settings → Linked devices. The store directory
now holds your ACI, device ID, encrypted keys, and sessions.

**Tip:** use a dedicated store directory per bot deployment. Do not share a
store between two running processes.

## Minimal bot (under 40 lines)

Create `cmd/mybot/main.go`:

```go
package main

import (
 "context"
 "os"

 "github.com/thehappydinoa/signal-go/examples/internal/botexample"
 "github.com/thehappydinoa/signal-go/pkg/bot"
)

func main() {
 os.Exit(botexample.Run(os.Args[1:], ".my-bot", setup))
}

func setup(b *bot.Bot) error {
 b.OnText("ping").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
  return m.Reply(ctx, "pong")
 })
 return nil
}
```

Run it:

```sh
go run ./cmd/mybot -store ./.my-bot
```

Message the linked account from another Signal user (not from the same linked
device — self-messages are normally ignored). Send `ping`; the bot replies
`pong`.

### What `botexample.Run` gives you

[`examples/internal/botexample/run.go`](../../examples/internal/botexample/run.go)
is shared scaffolding used by all `pkg/bot` examples. It:

- Parses `-store`, `-passphrase-file`, `-plaintext`, `-client`, `-user-agent`
- Opens the SQLite store and calls `bot.Open`
- Installs a default rate-limit retry middleware
- Blocks on `b.Run(ctx)` until Ctrl+C

For production you can copy this pattern or inline the same steps in your
`main` package.

## Handler registration

Handlers are registered **before** `b.Run`. The first matching handler wins
unless it returns `bot.ErrPass` (try the next handler).

### Commands and prefixes

```go
b.OnCommand("help").Do(func(ctx context.Context, m *bot.Message, args []string) error {
 return m.Reply(ctx, "try /ping")
})

b.OnCommand("echo").Do(func(ctx context.Context, m *bot.Message, args []string) error {
 return m.Reply(ctx, strings.Join(args, " "))
})

// Catch unknown slash commands last:
b.OnPrefix("/").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
 return m.Reply(ctx, "unknown command")
})
```

`OnCommand("echo")` matches `/echo`, `/echo hello`, etc. Args are split on
whitespace (the command name is not included).

### Exact text and regex

```go
b.OnText("ping").Do(...)                              // body == "ping"
b.OnRegex(regexp.MustCompile(`^remind (.+)$`)).Do(...) // capture groups in args
b.OnAnyText().Stage("await_name").Do(...)             // any body while in stage
```

### Scopes: DM, group, sender, group ID

```go
b.OnCommand("secret").DM().Do(...)           // direct messages only
b.OnCommand("poll").Group().Do(...)          // group threads only
b.OnCommand("admin").From(adminACI).Do(...)  // one sender only
b.OnCommand("alerts").Group().InGroups(groupIDHex).Do(...)
```

`GroupID()` on a message is the **hex-encoded 32-byte group master key**
(the same value passed to `Client.FetchGroup` / `SendGroup`).

### Reactions, edits, group updates

```go
b.OnReaction("👍").Group().Do(func(ctx context.Context, r *bot.Reaction) error {
 return nil
})

b.OnEdit().Do(func(ctx context.Context, e *bot.Edit) error { ... })

b.OnGroupUpdate(func(ctx context.Context, u *bot.GroupUpdate) error {
 grp, err := u.Sync(ctx) // optional: refresh local view
 _ = grp
 return nil
})
```

Enable background group sync with `bot.Options{AutoSyncGroupUpdates: true}`
(already set by `botexample.Run`).

## Replying and side effects

On `*bot.Message`:

| Method | Purpose |
|--------|---------|
| `m.Reply(ctx, text)` | Send text (DM or group, auto-routed) |
| `m.ReplyAttachment(ctx, reader, contentType)` | Send a file |
| `m.React(ctx, emoji)` | React to this message |
| `m.Typing(ctx)` | Typing indicator (`stop()` when done) |
| `m.MarkRead(ctx)` | Read receipt to author |

Access raw fields with `m.Sender()`, `m.Body()`, `m.GroupID()`, `m.IsGroup()`,
`m.Timestamp()`.

## Conversation state and wizards

Per-conversation key/value state lives on `m.Convo()` (in-memory by default):

```go
b.OnCommand("signup").DM().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
 m.Convo().SetStage("await_email")
 return m.Reply(ctx, "What's your email?")
})

b.OnAnyText().Stage("await_email").DM().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
 m.Convo().Set("email", m.Body())
 m.Convo().ClearStage()
 return m.Reply(ctx, "Thanks!")
})
```

For multi-step flows, use **`bot.Wizard`** — see
[`examples/wizard-bot`](../../examples/wizard-bot/) and the wizard section in
[Getting started](./getting-started.md#build-a-bot-phase-6).

Persist state across restarts by implementing `bot.ConvoStore` and passing it
in `bot.Options{ConvoStore: myStore}`.

## Middleware

Global middleware wraps every handler (outermost registered runs first):

```go
b.Use(func(next bot.HandlerFunc) bot.HandlerFunc {
 return func(ctx context.Context, m *bot.Message, args []string) error {
  // before
  err := next(ctx, m, args)
  // after
  return err
 }
})
```

Per-handler middleware:

```go
b.OnCommand("admin").Use(requireAdmin).Do(...)
```

See [`examples/middleware-bot`](../../examples/middleware-bot/) for recover,
logging, rate limiting, and admin gates.

Built-in helpers include `bot.RateLimitRetryMiddleware` (already used in
`botexample.Run`).

## Working with groups

### Join or create a group

Group admin APIs (`CreateGroup`, `FetchGroup`, `AddMember`, …) live on
`*signal.Client`, not on the narrow `bot.Client` interface. Open the client
once, wrap it for handlers:

```go
sgClient, err := signal.Open(ctx, signal.OpenOptions{
 AccountStore:           db,
 SignalStores:           db.SignalStores(),
 GroupDistributionStore: db.GroupDistributionStore(),
 GroupEndorsementStore:  db.GroupEndorsementStore(),
 AutoSyncGroupUpdates:   true,
})
b := bot.WrapWithOptions(sgClient, bot.WrapOptions{Logger: slog.Default()})
defer sgClient.Close()
```

**Join** via invite link:

```go
grp, err := sgClient.JoinGroupViaInviteLink(ctx, "https://signal.group/#...")
```

**Create** a new group (bot becomes administrator):

```go
result, err := sgClient.CreateGroup(ctx, signal.CreateGroupOptions{
 Title: "My alert group",
 Members: []signal.CreateGroupMember{
  {ACI: memberACI, ProfileKey: profileKey},
 },
})
masterKey := result.MasterKey
inviteURL, _, err := sgClient.EnableGroupInviteLink(ctx, masterKey, signal.GroupInviteLinkAccessAny)
```

Members need a known **32-byte profile key** (from an inbound message,
`FetchProfile`, or `SetRecipientProfileKey`). Without it they are added as
pending invites.

See [Getting started — Groups v2](./getting-started.md#groups-v2-membership) and
[ADR 0038](../adr/0038-groups-v2-create.md).

### Group commands in the bot layer

```go
b.OnCommand("status").Group().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
 masterKey, _ := hex.DecodeString(m.GroupID())
 grp, err := sgClient.FetchGroup(ctx, masterKey)
 if err != nil { return err }
 return m.Reply(ctx, fmt.Sprintf("%q — %d members", grp.Title, len(grp.Members)))
})
```

`m.Reply` in a group automatically uses `SendGroup`.

## Project layout (recommended)

```bash
my-bot/
├── cmd/mybot/main.go      # main + handler registration
├── internal/
│   ├── commands/          # command handlers (optional split)
│   └── config/            # YAML/env config (optional)
├── go.mod                 # require github.com/thehappydinoa/signal-go
└── .my-bot/               # gitignored — linked device store
```

Keep handler functions small; put business logic in `internal/` packages.
Use `botexample.Run` or your own `main` that opens the store once and calls
`bot.Open`.

## Configuration and secrets

| Secret / path | Notes |
|---------------|-------|
| Store directory | Contains keys and sessions — treat like a password database (`chmod 700`, encrypted passphrase). |
| Passphrase | Use `-passphrase-file` or env in production; avoid `-plaintext` outside tests. |
| Admin ACIs | Gate privileged commands with env vars or config (see middleware-bot). |

Never log profile keys, passphrases, or store passphrases. Structured logging
of sender ACI and group ID is fine.

## Testing

1. **Unit tests** — test pure functions in `internal/` without Signal network.
2. **Handler tests** — use `bot.Wrap(client)` with a stub `bot.Client` (see
   `pkg/bot` tests).
3. **Live smoke test** — run against a linked store; message the bot from a
   second account.
4. **E2E** — repository e2e tests under `pkg/signal` (`-tags=e2e`); see
   [Testing e2e](./testing-e2e.md).

During development:

```sh
task test
task lint
go run ./cmd/mybot -store ./.my-bot -auto-typing
```

## Deployment checklist

- [ ] Linked store created on the target host (or restored from backup).
- [ ] Passphrase available to the process (file, secret manager — not argv).
- [ ] Process supervisor (systemd, Docker, etc.) with restart policy.
- [ ] Single writer to the store (one process per linked device).
- [ ] `CGO_ENABLED=1` and `libsignal_ffi.a` present in the image/VM.
- [ ] Outbound network to `*.signal.org` (TLS pinned — see ADR 0034).

## Example gallery

| Example | Teaches |
|---------|---------|
| [`command-bot`](../../examples/command-bot/) | Slash commands |
| [`wizard-bot`](../../examples/wizard-bot/) | Multi-step DM flows |
| [`poll-bot`](../../examples/poll-bot/) | Group state + reactions |
| [`middleware-bot`](../../examples/middleware-bot/) | Middleware pipeline |
| [`attachment-bot`](../../examples/attachment-bot/) | Files, typing, receipts |
| [`echo-bot`](../../examples/echo-bot/) | Raw `pkg/signal` event loop |
| [`group-crawler`](../../examples/group-crawler/) | Group logs + invite joins |

Full index: [`examples/README.md`](../../examples/README.md).

## Troubleshooting

| Symptom | Likely cause |
|---------|----------------|
| `ErrNotLinked` | Run `signal-go link -store <path>` first. |
| Bot never replies | You are messaging from the same linked ACI; use another account. |
| DM send fails | Fetch profile / store profile key for sealed sender (see getting-started). |
| Group reply fails | Unknown UAK for a member; ensure SKDM ran (prior group traffic or endorsements). |
| `429` / rate limit | Back off; enable `RateLimitRetryMiddleware` or reduce send rate. |

More detail: [Getting started — Troubleshooting](./getting-started.md).

## Next steps

- Read [ADR 0008](../adr/0008-bot-framework.md) for design rationale.
- Browse [`pkg/bot`](../../pkg/bot/) godoc for the full API surface.
- For a production-style integration, see
  [atstreetlevel-bot-go](https://github.com/thehappydinoa/atstreetlevel-bot-go)
  (incident routing bot built on signal-go).
