# Getting started

`signal-go` is **pre-alpha**. You can pair it as a Signal secondary
device and receive incoming messages (Phase 3), but sending isn't
wired yet (Phase 4). This guide walks the link and receive flows.

## Prerequisites

- **Go 1.25+** (we use `crypto/hkdf` and other stdlib bits from recent releases)
- **A C toolchain** (gcc/clang on Linux/macOS, MSVC on Windows)
- **Rust** — only required *once* to build the pinned `libsignal_ffi.a`
- **`protoc`** if you need to regenerate the protobufs (we ship the
  generated code, so most contributors skip this)
- **A real Signal account** on your phone (to scan the QR)

## Build

```sh
git clone https://github.com/thehappydinoa/signal-go
cd signal-go

# One-time: build the pinned libsignal_ffi.a (~5–10 min on first run; cached after).
task libsignal

# Build the demo CLI.
task build
```

Don't have `task`? `go install github.com/go-task/task/v3/cmd/task@latest`
or read [`Taskfile.yml`](../../Taskfile.yml) and run the equivalent `go`
/ `bash` commands by hand.

## Pair as a secondary device

```sh
./bin/signal-go link -store ./.signal-data
```

You'll get an interactive passphrase prompt. The passphrase is used to
encrypt your account state (AES-256-GCM, with the key derived via
Argon2id) — see [the encrypted-store diagram](../diagrams/encrypted-store.md).

The tool then prints a `sgnl://linkdevice?...` URL. Two ways to use it:

1. **Open the QR**: paste the URL into your favourite terminal-based QR
   generator and scan it from your phone's *Signal → Settings → Linked
   devices → + (Add device)* menu.
2. **Type it manually**: not currently possible — Signal's mobile app
   doesn't expose a "paste URL" option. Use option 1.

After you approve on the phone, signal-go decrypts the provisioning
envelope, generates ACI + PNI prekeys, registers via
`PUT /v1/devices/link`, uploads one-time prekey batches, and persists
the account under `./.signal-data/`.

## Receive messages (library API)

After linking, use `signal.Open` to load the account and start
receiving typed events:

```go
import "github.com/thehappydinoa/signal-go/pkg/signal"

client, err := signal.Open(ctx, signal.OpenOptions{
    AccountStore: acctStore,
    SignalStores: signalStores,
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

for ev := range client.Events() {
    switch e := ev.(type) {
    case *signal.MessageEvent:
        fmt.Printf("From %s: %s\n", e.Sender, e.Body)
    case *signal.ReceiptEvent:
        fmt.Printf("Receipt from %s\n", e.Sender)
    case *signal.TypingEvent:
        fmt.Printf("Typing from %s\n", e.Sender)
    case *signal.SyncMessageEvent:
        fmt.Printf("Sync: %s\n", e.SentBody)
    case *signal.DecryptionErrorEvent:
        fmt.Printf("Decrypt error: %v\n", e.Err)
    }
}
```

The `Client` connects to Signal's authenticated chat websocket, handles
auto-reconnection with exponential backoff, and dispatches incoming
envelopes as typed events.

[`signal.Open`](../../pkg/signal/client.go) wires a libsignal-backed
[`cipher.EnvelopeDecryptor`](../../internal/cipher/envelope.go) by default
(sealed sender, session cipher, prekey messages). Override
`OpenOptions.Decryptor` only for tests or custom backends.

## Non-interactive

For systemd units, container deployments, CI, etc.:

```sh
# Read the passphrase from a file (trailing newline trimmed).
./bin/signal-go link -store /var/lib/mybot -passphrase-file /run/secrets/store-passphrase
```

Or supply your own 32-byte key by writing a small Go program against
`pkg/signal` and `internal/store/fsstore.NewWithKey`. The CLI doesn't
expose this directly to keep flag surface small.

## Verify the link

```sh
ls -l ./.signal-data
# -rw------- 1 you you 4096 May 20 17:00 account.enc
# -rw------- 1 you you  170 May 20 17:00 kdf.json
```

Open the Signal app → *Linked devices* and you should see "signal-go"
listed (or whatever you passed to `-name`).

## Build a bot (Phase 6)

`pkg/bot` is a thin dispatcher on top of `pkg/signal` modeled on
Telegram bot / Slack Bolt. You register handlers, scope them with
`DM()`/`Group()`/`From()`/`Stage()`, and reply with `m.Reply(ctx, …)`:

```go
import "github.com/thehappydinoa/signal-go/pkg/bot"

b, err := bot.Open(ctx, bot.Options{
    AccountStore: acctStore,
    SignalStores: signalStores,
})
if err != nil { return err }
defer b.Close()

b.OnText("ping").DM().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
    return m.Reply(ctx, "pong")
})
return b.Run(ctx)
```

### Conversation state

Each conversation (sender ACI + optional group ID) has a small
key/value store accessible via `m.Convo()`. Pair it with the
`Stage()` matcher to build multi-step flows without writing your
own per-user FSM table:

```go
b.OnCommand("signup").DM().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
    m.Convo().SetStage("await_email")
    return m.Reply(ctx, "what's your email?")
})
b.OnRegex(emailRE).Stage("await_email").Do(func(ctx context.Context, m *bot.Message, args []string) error {
    m.Convo().Set("email", args[0])
    m.Convo().SetStage("await_age")
    return m.Reply(ctx, "and your age?")
})
b.OnRegex(numRE).Stage("await_age").Do(func(ctx context.Context, m *bot.Message, args []string) error {
    m.Convo().Set("age", args[0])
    m.Convo().ClearStage()
    return m.Reply(ctx, "thanks, " + m.Convo().Get("email") + "!")
})
b.OnCommand("cancel").DM().AnyStage().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
    m.Convo().Clear()
    return m.Reply(ctx, "ok, cancelled")
})
```

The default store is in-memory (`bot.MemoryConvoStore`). Pass
`bot.Options.ConvoStore` to plug in a persistent backend (the
`ConvoStore` interface is small enough that wrapping any
key/value store works).

### Groups v2 (fetch state)

Once you have a group's 32-byte master key (from an inbound group
message's `groupV2.masterKey`, or hex-decoded from
`MessageEvent.GroupID`), fetch the decrypted roster:

```go
masterKey, _ := hex.DecodeString(msg.GroupID)
grp, err := client.FetchGroup(ctx, masterKey)
if err != nil { return err }
if grp.IsAdmin(msg.Sender) {
    // restricted admin command
}
```

Auth credentials are fetched from `GET /v1/certificate/auth/group` and
cached per UTC day on the client. See [ADR 0018](../adr/0018-groups-v2-bootstrap.md).

## What's next

- **Receive** (Phase 3): connection, dispatch, and libsignal decrypt are
  working; inbound prekey decrypt triggers automatic `PUT /v2/keys` top-up
  when the local pool runs low (disable via `OpenOptions.DisablePreKeyMaintenance`).
- **Send** (Phase 4): mostly done. The [send flow](../diagrams/send-flow.md)
  describes the shape; profile fetch is the last open item.
- **Bot framework** (Phase 6): in progress — DM dispatch, scopes,
  middleware, and conversation state shipped; group reply lands with
  Phase 5. See [ADR 0008](../adr/0008-bot-framework.md).

## Troubleshooting

- *"the requested URL returned error: 404"* during `task libsignal` —
  the upstream tag in `scripts/build-libsignal.sh` is wrong or Signal
  moved the repo. Check the [pinned version](../../scripts/build-libsignal.sh).
- *"wrong passphrase (or the store is corrupted)"* — make sure you
  typed the same passphrase you used at link time, or delete
  `./.signal-data/` and re-link.
- *Compilation errors mentioning `signal_*`* — re-run `task libsignal`.
  The header (`internal/libsignal/include/signal_ffi.h`) and static
  library (`internal/libsignal/lib/libsignal_ffi.a`) must come from the
  same upstream tag.
