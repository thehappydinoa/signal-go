# signal-go Roadmap

> Status legend: **done**, **in progress**, **next**, **planned**

This roadmap tracks the staged build-out of a from-scratch Go Signal client on
top of Signal's official `libsignal` Rust library. Architectural decisions are
captured under [`docs/adr/`](./docs/adr/).

## Phase 1 — Foundation **(in progress)**

Get the cgo build, protobuf codegen, and the unauthenticated half of the
device-linking flow working end to end. Outcome: `signal-go link` connects to
`wss://chat.signal.org/v1/websocket/provisioning/` and prints a real
`sgnl://linkdevice?...` URL that the Signal mobile app will accept (it'll show
the "Link this device?" prompt; we won't yet complete the link).

- [x] Project layout + Go module
- [x] Vendor canonical `.proto` files from Signal-Android (`proto/`)
- [x] Vendor cbindgen-generated `signal_ffi.h` from libsignal v0.94.1
- [x] `scripts/build-libsignal.sh` — pinned, reproducible static-lib build
- [x] Taskfile (`task libsignal`, `task proto`, `task build`, `task test`, `task lint`)
- [x] `.golangci.yml`, `.editorconfig`, test conventions
- [ ] Protobuf codegen for `Provisioning`, `WebSocketResources`, `SignalService`
- [ ] `internal/libsignal`: cgo preamble, error mapping, basic key primitives
      (`PrivateKey`, `PublicKey`, `IdentityKeyPair`, `KeyPair.generate`)
- [ ] `internal/ws`: `WebSocketMessage` framed connection wrapper
- [ ] `internal/provisioning`: open provisioning ws, receive `ProvisioningUuid`,
      compose `sgnl://linkdevice?uuid=...&pub_key=...&capabilities=...` URL
- [ ] Demo `cmd/signal-go link` prints the URL (and optionally renders ANSI QR)
- [ ] Unit tests for proto roundtrip, URL encoding, ws frame parsing
- [ ] Integration test stub (skipped unless `SIGNAL_GO_E2E=1`)

## Phase 2 — Complete the link **(planned)**

- [ ] `ProvisioningCipher` decrypt via libsignal (cgo wrapper)
- [ ] Parse `ProvisionMessage` → ACI identity keypair, PNI identity keypair,
      profile key, master key/AEP, number, provisioning code, link-device token
- [ ] Prekey generation for both ACI and PNI:
  - signed prekey (rotating)
  - **mandatory PQXDH**: Kyber last-resort prekey + 100 one-time Kyber prekeys
  - 100 one-time prekeys
- [ ] `internal/web`: REST client, basic auth, JSON request/response
- [ ] `PUT /v1/devices/link` with `AccountAttributes`, prekeys, capabilities
- [ ] `internal/store`: storage interface + filesystem reference impl
      (account state, identity keys, prekey records, sessions, sender keys)
- [ ] End-to-end test against a real phone (manual, gated)

## Phase 3 — Receive **(planned)**

- [ ] Authenticated websocket with auto-reconnect + backoff
- [ ] Envelope dispatch loop
- [ ] Sealed-sender unwrap → session decrypt (Double Ratchet via libsignal)
- [ ] Content protobuf decode → typed events
      (`*MessageEvent`, `*ReceiptEvent`, `*TypingEvent`, `*SyncEvent`)
- [ ] Decryption-error handling (`SignalDecryptionErrorMessage` retry token)
- [ ] Prekey rotation on use; top-up endpoint

## Phase 4 — Send 1:1 **(planned)**

- [ ] Profile fetch (decrypt with profile key via libsignal `ProfileCipher`)
- [ ] Unidentified-access certificate refresh
- [ ] Establish session: prekey bundle fetch → `PreKeySignalMessage` first send
- [ ] Sealed-sender encrypt → `PUT /v1/messages/{uuid}`
- [ ] Multi-device fan-out, mismatched/stale-device handling
- [ ] Read/delivery receipts

## Phase 5 — Groups v2 **(planned)**

- [ ] zkgroup credential cache (server params + auth credentials)
- [ ] Group master key handling, GroupSecretParams
- [ ] Fetch + decrypt group state (`/v1/groups`)
- [ ] Sender-key distribution; group message encrypt/decrypt
- [ ] Group membership changes (join/leave/role)

## Phase 6 — Bot framework **(planned)**

A higher-level `pkg/bot` package on top of `pkg/signal` that makes Signal
bots as ergonomic as Telegram or Slack Bolt:

- [ ] `bot.Open(ctx, opts)` — load an existing linked-device account from a
      store directory (or guide the user through `signal-go link` if missing)
- [ ] Pattern dispatchers: `OnText`, `OnPrefix`, `OnRegex`, `OnCommand("/foo")`
- [ ] Scopes: `.DM()`, `.Group()`, `.From("+15551234567")`
- [ ] Reply helpers on `*Message`: `Reply`, `ReplyAttachment`, `React`,
      `Typing`, `MarkRead`
- [ ] Reaction and edit event handlers
- [ ] Middleware chain: logging, rate-limit, auth, per-conversation state
- [ ] Conversation state (sessions / wizards) via in-memory or persistent store
- [ ] Graceful shutdown, structured logging via `log/slog`

See [ADR 0008](./docs/adr/0008-bot-framework.md) for the API sketch.

## Phase 7 — Niceties **(planned, out of MVP)**

- [ ] Attachments (CDN3 upload/download, attachment cipher via libsignal)
- [ ] Storage Service sync (contacts, group list)
- [ ] CDSI contact discovery
- [ ] SQLite-backed store
- [ ] Backup/restore (linked-device "synchronized start")

## Non-goals

- We will not implement the Signal protocol cryptography in Go. All crypto goes
  through libsignal.
- We will not target headless registration (creating a brand-new Signal account
  by phone number). Only linking as a secondary device.
- We will not ship a REST/HTTP wrapper in this repo (that can live in a separate
  repo on top of `pkg/signal`).
