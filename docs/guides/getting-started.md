# Getting started

`signal-go` is **pre-alpha**. You can pair it as a Signal secondary
device, receive typed events (Phase 3), and send 1:1 messages with
automatic sealed-sender when a profile key is known (Phase 4).

## Prerequisites

- **Go 1.25+** (we use `crypto/hkdf` and other stdlib bits from recent releases)
- **A C toolchain** (gcc/clang on Linux/macOS; on Windows use **MSYS2
  MinGW-w64**, not MSVC — see below)
- **Rust** — only required *once* to build the pinned `libsignal_ffi.a`
- **`protoc`** if you need to regenerate the protobufs (we ship the
  generated code, so most contributors skip this)
- **A real Signal account** on your phone (to scan the QR)

### Environment (`.env`)

CI only sets a few globals (`GO_VERSION`, `CGO_ENABLED`, pinned
`LIBSIGNAL_VERSION`). The **Windows release** leg in
[`.github/workflows/release.yml`](../../.github/workflows/release.yml)
is the reference for native Windows: MSYS2 `mingw-w64-x86_64-gcc`, writable
`TEMP`, explicit `PROTOC` / include path, and `CC=gcc` / `CXX=g++`.

Locally:

```sh
cp .env.example .env
# Edit paths (especially TMP/TEMP and PROTOC_INCLUDE on Windows).
```

[`Taskfile.yml`](../../Taskfile.yml) loads `.env` automatically. For a bare
shell session:

```sh
set -a && source .env && set +a
# or:
source scripts/dev-env.sh
```

**`CGO_ENABLED` must be `1`.** If `go env CGO_ENABLED` prints `0`, either
`go env -w CGO_ENABLED=1` or set it in `.env` (some Windows Go installs
ship with cgo disabled).

### Windows (Git Bash + MSYS2)

1. Install [MSYS2](https://www.msys2.org/) and, in the **MINGW64** shell:
   `pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-toolchain mingw-w64-x86_64-nasm git`
2. Install `protoc` with the standard `google/protobuf/*.proto` includes
   (WinGet `Google.Protobuf`, or MSYS `mingw-w64-x86_64-protobuf`).
3. Copy `.env.example` → `.env` and set `TMP`/`TEMP` to your user Temp
   folder (not `C:\WINDOWS`).
4. From **Git Bash** at the repo root: `source scripts/dev-env.sh` then
   `task libsignal` and `task build`.

Native Windows builds are still **experimental** (see
[ADR 0033](../adr/0033-release-pipeline.md)); Linux/macOS match CI today.
WSL2 with the Linux flow is the easiest path if MSYS linking blocks you.

## Build

```sh
git clone https://github.com/thehappydinoa/signal-go
cd signal-go

cp .env.example .env   # optional but recommended on Windows

# One-time: build the pinned libsignal_ffi.a (~5–10 min on first run; cached after).
task libsignal

# Build the library and demo CLI → bin/signal-go
task build
```

`task build` also runs `go build ./...` for all packages. The CLI binary
always lands at **`bin/signal-go`** (on Windows: `bin/signal-go.exe`).
`go build ./cmd/signal-go` without `-o` writes under `cmd/signal-go/`
instead — prefer `task build` or `-o bin/signal-go` for a stable path.

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

The tool prints an **ANSI QR code** in the terminal (when stdout is a TTY)
plus the `sgnl://linkdevice?...` URL as fallback. Scan from your phone:
*Signal → Settings → Linked devices → + (Add device)*.

- **`signal-go link -no-qr`** — URL only (scripts, narrow terminals, or
  `NO_COLOR` set).
- Signal's mobile app does not support pasting the URL manually; you need
  the QR (on-screen or from another device).

After you approve on the phone, signal-go decrypts the provisioning
envelope, generates ACI + PNI prekeys, registers via
`PUT /v1/devices/link`, uploads one-time prekey batches, and persists
the account under `./.signal-data/`.

### Link-and-sync (message history transfer)

To request the primary device upload message history during linking,
enable link-and-sync when calling [`signal.Link`](../../pkg/signal/link.go)
from library code:

```go
db, _ := sqlstore.OpenWithPassphrase("./.signal-data", passphrase)
linked, err := signal.Link(ctx, signal.LinkOptions{
    OnURL:             printURL,
    Store:             db,
    LinkAndSync:       true,
    SignalStores:      db.SignalStores(),
    BackupImportStore: db, // imports contact identity keys + group master keys
})
if linked.Sync != nil && linked.Sync.Imported {
    // Contacts/groups are persisted; profile keys load on signal.Open.
    _ = linked.Sync.ImportStats
}
```

Import covers contact identity keys, profile keys, and group master keys.
Full chat history (`ChatItem` frames) is not imported yet — see
[ADR 0031](../adr/0031-transfer-archive-frame-import.md).

The primary may respond with `CONTINUE_WITHOUT_UPLOAD` (link proceeds
without history) or `RELINK_REQUESTED` (start linking again).

### Client User-Agent presets

By default the CLI sends `signal-go` in `User-Agent` and `X-Signal-Agent`.
For linked-device flows you can mimic upstream clients using templates taken
from the official source trees:

| Preset | Upstream template | Source |
|--------|-------------------|--------|
| `android` | `Signal-Android/{version} Android/{sdk}` | [StandardUserAgentInterceptor.java](https://github.com/signalapp/Signal-Android/blob/main/app/src/main/java/org/thoughtcrime/securesms/net/StandardUserAgentInterceptor.java#L12) |
| `ios` | `Signal-iOS/{version} iOS/{systemVersion}` | [HttpHeaders.swift](https://github.com/signalapp/Signal-iOS/blob/main/SignalServiceKit/Network/HttpHeaders.swift#L151-L153) |
| `desktop-linux` | `Signal-Desktop/{version} Linux {release}` | [getUserAgent.ts](https://github.com/signalapp/Signal-Desktop/blob/main/ts/util/getUserAgent.ts#L7-L28) |
| `desktop-macos` | `Signal-Desktop/{version} macOS {release}` | same |
| `desktop-windows` | `Signal-Desktop/{version} Windows {release}` | same |

Default version numbers in each preset are snapshots (not live-fetched). Override
with `UserAgentOptions` in library code or `-user-agent` on the CLI.

Note: Signal Desktop sends `X-Signal-Agent: OWD` separately from
`User-Agent` ([WebAPI.ts](https://github.com/signalapp/Signal-Desktop/blob/v7.47.0/ts/textsecure/WebAPI.ts#L336-L337));
signal-go currently uses the same string for both headers.

```sh
# Linux desktop linked device (recommended on servers/VMs)
./bin/signal-go link -store ./.signal-data -client desktop-linux

# Other presets: android, ios, desktop-macos, desktop-windows
./bin/signal-go link -client android -store ./.signal-data
```

Library callers set [`signal.LinkOptions.ClientProfile`](../../pkg/signal/link.go)
(or [`signal.OpenOptions.ClientProfile`](../../pkg/signal/client.go)) to
`signal.UserAgentDesktopLinux`, `signal.UserAgentAndroid`, etc. Use
[`signal.UserAgentUpstreamSource`](../../pkg/signal/useragent.go) to retrieve
the citation for a profile. Pass a non-empty `UserAgent` string to override the
preset entirely.

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

## Send and profile fetch (library API)

After linking, `signal.Client.Send` delivers 1:1 text. Sealed-sender
activates automatically once the recipient's profile key is known — it
arrives on inbound messages (`DataMessage.profileKey`) or can be supplied
explicitly:

```go
// Optional: fetch display name when you already have the profile key.
prof, err := client.FetchProfile(ctx, senderACI, profileKey)
if err == nil {
    fmt.Println(prof.DisplayName())
}

_, err = client.Send(ctx, recipientACI, "hello")
```

See [ADR 0017](../adr/0017-profile-fetch.md) for the UAK derivation and
ProfileCipher wire format.

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

### Groups v2 (send)

Reply in a group with `Client.SendGroup` or `bot.Message.Reply`. Each
member must have a known unidentified access key (from an inbound
`profileKey`, `FetchProfile`, or `SetRecipientProfileKey`):

```go
_, err := client.SendGroup(ctx, masterKey, "hello group")
```

SKDM is distributed to members over 1:1 sessions; the payload is delivered
via `PUT /v1/messages/multi_recipient`. Send prefers a **group send
endorsement token** from the latest [FetchGroup]; combined UAK is the
legacy fallback. See [ADR 0019](../adr/0019-group-sender-key.md) and
[ADR 0020](../adr/0020-group-endorsements-membership.md).

### Attachments

Send a file in 1:1 or group threads; inbound messages expose metadata on
`MessageEvent.Attachments`:

```go
_, err := client.SendAttachment(ctx, recipientACI, bytes.NewReader(data), signal.SendAttachmentOptions{
    ContentType: "image/png",
    FileName:    "chart.png",
})
_, err = client.SendGroupAttachment(ctx, masterKey, bytes.NewReader(data), signal.SendAttachmentOptions{
    ContentType: "application/pdf",
})

// In a bot handler:
for _, att := range m.Attachments() {
    plain, err := client.DownloadAttachment(ctx, att)
    // ...
}
return m.ReplyAttachment(ctx, bytes.NewReader([]byte("ok")), "text/plain")
```

See [ADR 0026](../adr/0026-attachment-cipher.md).

### Groups v2 (membership)

Administrators can promote/demote, add/remove members, or any member can leave:

```go
err := client.LeaveGroup(ctx, masterKey)
grp, err := client.PromoteMember(ctx, masterKey, memberACI)
grp, err := client.DemoteMember(ctx, masterKey, memberACI)
grp, err := client.RemoveMember(ctx, masterKey, memberACI)
grp, err := client.AddMember(ctx, masterKey, memberACI, profileKey, signal.GroupRoleDefault)
```

[AddMember] fetches an expiring profile key credential from the chat service
(requires the member's 32-byte profile key). Optional
`OpenOptions.GroupDistributionStore` (e.g. `fsstore.NewGroupDistributionStore`)
and `OpenOptions.GroupEndorsementStore` (e.g. `fsstore.NewGroupEndorsementStore`)
persist sender-key distribution UUIDs and group send endorsement caches across
restarts.

Join a group via invite link (requires the linked account's profile key):

```go
preview, err := client.PreviewGroupJoin(ctx, "https://signal.group/#...")
grp, err := client.JoinGroupViaInviteLink(ctx, "https://signal.group/#...")
```

When the link requires admin approval, [JoinGroupViaInviteLink] adds the local
user to the pending list instead of full membership.

Catch up from a known revision without a full refetch:

```go
grp, err := client.SyncGroup(ctx, masterKey, knownRevision)
page, err := client.FetchGroupLogs(ctx, masterKey, signal.GroupLogsFetchOptions{
    FromRevision: knownRevision,
    IncludeLastState: true,
})
```

Inbound peer changes arrive as `GroupUpdateEvent` (empty body, populated
`groupChange`). Handle them in a bot or on the raw client event stream:

```go
b.OnGroupUpdate(func(ctx context.Context, u *bot.GroupUpdate) error {
    grp, err := u.Sync(ctx) // applies via SyncGroup from cached revision
    if err != nil { return err }
    log.Printf("group %s now at revision %d", u.GroupID(), grp.Revision)
    return nil
})
```

Enable background sync on every inbound update with
`bot.Options{AutoSyncGroupUpdates: true}` (or the same field on
`signal.OpenOptions`). See [ADR 0025](../adr/0025-inbound-group-updates.md).

### Groups v2 (control messages)

React and typing in groups use the same sender-key multi-recipient path as
text. Read/viewed receipts go 1:1 to the message author (not the whole group):

```go
_, err := client.SendGroupReaction(ctx, masterKey, "👍", authorACI, msgTime, false)
_, err := client.SendGroupTyping(ctx, masterKey, signal.TypingStarted)
_, err := client.SendReceipt(ctx, authorACI, signal.ReceiptRead, []time.Time{msgTime})
```

Bot helpers (`m.React`, `m.Typing`, `m.MarkRead`, …) branch automatically
when `m.IsGroup()`. Multi-step flows use [bot.Wizard]:

```go
signup := b.Wizard("signup")
signup.Step("await_email", func(ctx context.Context, m *bot.Message, _ []string) error {
    m.Convo().Set("email", m.Body())
    signup.Advance(m, "await_age")
    return m.Reply(ctx, "age?")
})
signup.Register()
```

See [ADR 0021](../adr/0021-group-control-messages.md) and
[ADR 0022](../adr/0022-phase5-finish.md).

### Storage Service sync (contacts + group list)

Pull the encrypted contact list and Groups v2 chat list from Signal's
storage service. Requires a non-empty `AccountEntropyPool` on the linked
account (populated at link time; updated via `SyncMessage.Keys`):

```go
result, err := client.SyncStorage(ctx)
if err != nil {
    log.Fatal(err)
}
for _, c := range result.Contacts {
    fmt.Println(c.ACI, c.GivenName)
}
for _, g := range result.Groups {
    grp, _ := client.FetchGroup(ctx, g.MasterKey)
    fmt.Println(g.ID, grp.Title)
}
```

Contact profile keys are cached automatically for sealed-sender send.
Enable background sync when a linked device requests it:

```go
client, err := signal.Open(ctx, signal.OpenOptions{
    AccountStore:    acctStore,
    SignalStores:    signalStores,
    AutoSyncStorage: true,
})
```

See [ADR 0027](../adr/0027-storage-service-sync.md).

### CDSI contact discovery

Resolve E.164 phone numbers to Signal ACIs via CDSI (requires network access
to Signal's contact discovery service):

```go
result, err := client.DiscoverContacts(ctx, []string{"+15551234567", "+441234567890"})
for _, c := range result.Contacts {
    fmt.Println(c.E164, c.ACI, c.PNI)
}
```

Directory credentials are fetched automatically from `GET /v2/directory/auth`.
See [ADR 0028](../adr/0028-cdsi-contact-discovery.md).

### SQLite-backed store

For production bots that must survive restarts with sessions and prekeys intact,
use `internal/store/sqlstore` instead of `memstore` + `fsstore`:

```go
import "github.com/thehappydinoa/signal-go/internal/store/sqlstore"

db, err := sqlstore.OpenWithPassphrase("./.signal-data", passphrase)
// db implements account.Store
// db.SignalStores() implements store.SignalStores
client, err := signal.Open(ctx, signal.OpenOptions{
    AccountStore: db,
    SignalStores: db.SignalStores(),
})
```

See [ADR 0029](../adr/0029-sqlite-backed-store.md).

## What's next

- **Receive** (Phase 3): connection, dispatch, and libsignal decrypt are
  working; inbound prekey decrypt triggers automatic `PUT /v2/keys` top-up
  when the local pool runs low (disable via `OpenOptions.DisablePreKeyMaintenance`).
- **Send** (Phase 4): done — see [send flow](../diagrams/send-flow.md) and
  [ADR 0017](../adr/0017-profile-fetch.md).
- **Groups v2** (Phase 5): done. See [ADR 0018](../adr/0018-groups-v2-bootstrap.md)
  through [ADR 0025](../adr/0025-inbound-group-updates.md).
- **Bot framework** (Phase 6): wizard sugar, group helpers, and
  `OnGroupUpdate` shipped. See [ADR 0008](../adr/0008-bot-framework.md),
  [ADR 0022](../adr/0022-phase5-finish.md), and
  [ADR 0025](../adr/0025-inbound-group-updates.md).

## Releases (maintainers)

To ship a new `v*` tag and draft GitHub Release binaries, see
[Releasing](../guides/releasing.md) (Actions → **Create release tag**).

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
- *`undefined reference to fiat_p256_adx_mul` (Windows)* — your
  `libsignal_ffi.a` predates the MinGW stub step in
  `scripts/build-libsignal.sh`. Run `task libsignal` again (or
  `FORCE=1 task libsignal`); the script appends portable ADX fallbacks
  for COFF linkers.
- *Browser or `curl` shows an untrusted cert for `https://chat.signal.org`* —
  expected if Signal's private root is not in your OS trust store. The
  official apps pin that CA; `signal-go` does too ([ADR 0034](../adr/0034-signal-tls-root-pinning.md)).
  Browsers: install the same root Signal ships (`signal-messenger.cer` in
  [Signal-iOS](https://github.com/signalapp/Signal-iOS/blob/main/SignalServiceKit/Resources/Certificates/signal-messenger.cer))
  into **Trusted Root Certification Authorities** if you want Chrome/Edge to
  open the site. For CLI use, rebuild after the pinning change — do not use
  `InsecureSkipVerify` against production.
- *`signal-go link`: `x509: certificate signed by unknown authority`* — on
  builds before ADR 0034, or if you override TLS with a custom `RootCAs` pool
  that omits Signal's root. Update to current `main` and retry; UniFi SSL
  inspection can also break TLS if it replaces the leaf (check the issuer is
  **Signal Messenger, LLC**, not your firewall vendor).
