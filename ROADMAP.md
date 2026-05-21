# signal-go Roadmap

> Status legend: **done**, **in progress**, **next**, **planned**

This roadmap tracks the staged build-out of a from-scratch Go Signal client on
top of Signal's official `libsignal` Rust library. Architectural decisions are
captured under [`docs/adr/`](./docs/adr/).

## Phase 1 — Foundation **(done)**

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
- [x] Protobuf codegen for `Provisioning`, `WebSocketResources`, `SignalService`
- [x] `internal/libsignal`: cgo preamble, error mapping, basic key primitives
      (`PrivateKey`, `PublicKey`, `IdentityKeyPair`, `KeyPair.generate`)
- [x] `internal/ws`: `WebSocketMessage` framed connection wrapper
- [x] `internal/provisioning`: open provisioning ws, receive `ProvisioningUuid`,
      compose `sgnl://linkdevice?uuid=...&pub_key=...&capabilities=...` URL
- [x] Demo `cmd/signal-go link` prints the URL (ANSI QR rendering still
      open as a nice-to-have)
- [x] Unit tests for proto roundtrip, URL encoding, ws frame parsing
- [x] Integration test stub (skipped unless `SIGNAL_GO_E2E=1`)

## Phase 2 — Complete the link **(done except where noted)**

- [x] `ProvisioningCipher` decrypt (Go AES-CBC + HMAC on top of libsignal
      ECDH/HKDF, since libsignal exposes the primitives but not the cipher)
- [x] Parse `ProvisionMessage` → ACI/PNI identity keys, profile key,
      AccountEntropyPool, number, provisioning code
- [x] Prekey generation for both ACI and PNI:
  - [x] Curve25519 signed prekey (rotating)
  - [x] Kyber/ML-KEM last-resort prekey (rotating, signed)
  - [x] Curve25519 one-time prekeys (generator)
  - [x] Kyber one-time prekeys (generator)
- [x] `internal/web`: REST client (basic auth, JSON, error type)
- [x] `PUT /v1/devices/link` with `AccountAttributes` + signed + Kyber
      last-resort prekeys (both namespaces)
- [x] `internal/account`: Account model + validation
- [x] `internal/store`: storage interface + `memstore` (tests) + `fsstore`
      (atomic JSON write to disk)
- [x] Public API: `signal.Link` orchestrates the whole flow and persists
- [x] Upload one-time prekeys (Curve25519 + Kyber, batch size configurable
      via `LinkOptions.OneTimePreKeyCount`, default 100) via
      `PUT /v2/keys?identity={aci,pni}` after the link succeeds
- [ ] **Phase 2-followup**: encrypted device name (libsignal
      `signal_device_name_*` FFI)
- [ ] End-to-end test against a real phone (manual, gated by `SIGNAL_GO_E2E=1`)

## Phase 3 — Receive **(in progress)**

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

## Phase 8 — Security audit **(planned; required before v0.1.0)**

A focused review before we cut a `v0.1.0` tag and put `signal-go` in front
of real Signal accounts. Scope is **our Go code and our cgo boundary** —
libsignal itself is out of scope (we trust upstream Signal). See
[ADR 0011](./docs/adr/0011-security-audit.md) for the methodology,
threat model, and what "pass" means.

Internal review (we do this before any external work):

- [ ] `internal/libsignal` cgo audit:
  - every `*Buffer` lifetime, `keepAlive`, finalizer, and `cgo.Handle` is
    accounted for
  - no Go pointers cross into Rust except via the documented borrowed/owned
    rules in `doc.go`
  - errors free their underlying `SignalFfiError` exactly once
  - confirm we link the *release* libsignal_ffi.a, not any `*-testing*`
- [ ] `internal/provisioning` cipher review:
  - constant-time MAC compare (`hmac.Equal`) on every branch
  - constant-time PKCS-7 unpad
  - structural validation before any cryptographic operation
  - fuzz test for `DecryptEnvelope` (corpus seeded from real envelopes)
- [ ] `internal/store/fsstore` review:
  - filesystem perms `0700` dir / `0600` files
  - atomic rename for every write
  - account.json never logs Password or PrivateKey
  - [x] at-rest encryption: AES-256-GCM + Argon2id (ADR 0012); wrong-passphrase
    fails closed via [`ErrWrongPassphrase`]; mode-mixing fails via
    [`ErrDirEncrypted`]/[`ErrDirPlaintext`]
- [ ] `internal/web` TLS posture:
  - `MinVersion: tls.VersionTLS12` (or 1.3) explicit
  - Signal's chat.signal.org pinned-CA option (off by default, available)
  - no credentials in URL query strings or log lines
- [ ] Receive pipeline (Phase 3+) decrypt-error handling:
  - bad ciphertext / wrong identity / replayed envelope each fail closed
    and surface a typed event without taking the connection down
  - `DecryptionErrorMessage` retry token round-trips
- [ ] Sealed-sender certificate validation against Signal's trust roots
- [ ] zkgroup credential cache eviction on identity-key change
- [ ] Code-level checklists:
  - `go vet ./...`, `staticcheck`, `gosec ./...` all clean
  - `govulncheck ./...` clean (or every finding triaged in this PR)
  - `golangci-lint run` with our pinned config clean
  - `go test -race -count=10 ./...` stable across 10 runs
  - fuzz targets run at least 5 minutes each in CI
- [ ] Documentation:
  - threat model written up under `docs/security/threat-model.md`
  - responsible-disclosure policy in `SECURITY.md`
  - public-key contact for security reports

External review (after the internal pass is clean):

- [ ] Engage an external auditor familiar with Signal/libsignal-FFI
      bindings (e.g. someone who reviewed `pkg/libsignalgo`)
- [ ] Publish the audit report and our remediation in
      `docs/security/audits/`

## Non-goals

- We will not implement the Signal protocol cryptography in Go. All crypto goes
  through libsignal.
- We will not target headless registration (creating a brand-new Signal account
  by phone number). Only linking as a secondary device.
- We will not ship a REST/HTTP wrapper in this repo (that can live in a separate
  repo on top of `pkg/signal`).
