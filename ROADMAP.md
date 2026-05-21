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

## Phase 3 — Receive **(done)**

- [x] Authenticated websocket with auto-reconnect + backoff
      (`internal/chat.Connection`)
- [x] Envelope dispatch loop (`pkg/signal.Client.processEnvelope`)
- [x] Sealed-sender unwrap → session decrypt (Double Ratchet via libsignal)
      (`internal/libsignal` decrypt + `internal/cipher.EnvelopeDecryptor`)
- [x] Content protobuf decode → typed events
      (`*MessageEvent`, `*ReceiptEvent`, `*TypingEvent`, `*SyncMessageEvent`)
- [x] Decryption-error handling (`*DecryptionErrorEvent` emitted without
      killing the receive loop; retry token pending Phase 4 send)
- [x] Public API: `signal.Open` + `signal.Client` with buffered `Events()`
      channel, `Decryptor` interface for pluggable decrypt backends
- [x] Prekey rotation on use; top-up endpoint
      (`internal/prekeymaint.Maintainer`, `PUT /v2/keys` after inbound prekey decrypt)

## Phase 4 — Send 1:1 **(in progress)**

- [x] Establish session: prekey bundle fetch (`GET /v2/keys/{aci}/{dev}`)
      → `ProcessPreKeyBundle` (`pkg/signal.Client.Send` first-call path)
- [x] Encrypt with session cipher; emit basic-auth `PUT /v1/messages/{uuid}`
      with the wire-format envelope (`web.SendMessage` + `MessageEnvelope`)
- [x] Mismatched / stale device responses surfaced as typed errors
      (`*web.MismatchedDevicesError`, `*web.StaleDevicesError`)
- [x] Auto-retry on `*MismatchedDevicesError` / `*StaleDevicesError`
      (refresh bundles, drop stale sessions, resend — at-most-one retry)
- [x] Multi-device fan-out (GET `/*` on first send; per-recipient device
      list cached in-memory; one envelope per device in a single PUT)
- [x] Sealed-sender encrypt → server doesn't see our ACI as the sender.
      Per-device USMC via `signal_unidentified_sender_message_content_new`
      + serialization; sender-cert fetch from `/v1/certificate/delivery`
      with 5-minute-headroom cache ([ADR 0015](docs/adr/0015-sealed-sender-encrypt.md)).
      Activated automatically once the recipient's UAK is provided via
      `Client.SetRecipientUAK` (populated by profile fetch, Phase 5).
- [x] Unidentified-access certificate refresh + cache (sender cert cached
      in `Client.senderCert`; re-fetched when < 5 min from expiry)
- [ ] Profile fetch (decrypt with profile key via libsignal `ProfileCipher`)
- [ ] Read/delivery receipts

## Phase 5 — Groups v2 **(planned)**

- [ ] zkgroup credential cache (server params + auth credentials)
- [ ] Group master key handling, GroupSecretParams
- [ ] Fetch + decrypt group state (`/v1/groups`)
- [ ] **Surface group membership + admin roles** on the public API:
      parse the decrypted group state into typed Go (`signal.Group{
      Title, Description, Avatar, Members[], Admins[]}`), expose a
      `(*bot.Bot).Groups(ctx)` to list joined groups, and a
      `(*bot.Message).Group(ctx)` accessor to fetch the parsed group
      for a group-thread message. Needed so bots can branch on
      `m.Group().IsAdmin(m.Sender())` for restricted commands.
- [ ] Sender-key distribution; group message encrypt/decrypt
- [ ] Group membership changes (join/leave/role)

## Phase 6 — Bot framework **(in progress)**

A higher-level `pkg/bot` package on top of `pkg/signal` that makes Signal
bots as ergonomic as Telegram or Slack Bolt:

- [x] `bot.Open(ctx, opts)` — loads the persisted account, connects the
      chat ws, returns a dispatcher; `bot.Wrap(client)` for tests
- [x] Pattern dispatchers: `OnText`, `OnPrefix`, `OnRegex`, `OnCommand("/foo")`,
      first-match-wins ordering, `ErrPass` to fall through
- [x] `Reply` helper on `*Message` (1:1 only — group reply lands with Phase 5)
- [x] Custom error handler via `Bot.OnError`
- [x] Graceful shutdown via `Bot.Close` + `Bot.Run(ctx)`; structured
      logging via the injected `*slog.Logger`
- [x] Scopes: `.DM()` (direct-message only), `.Group()` (group only),
      `.From(aci)` (sender filter)
- [ ] Group `Reply` once Phase 5 surfaces the group identifier + send
      path; `ReplyAttachment`, `React`, `Typing`, `MarkRead`
- [ ] Reaction and edit event handlers
- [x] Middleware chain: `Bot.Use(MiddlewareFunc)` for global middleware;
      `Match.Use(MiddlewareFunc)` for per-handler middleware; outermost-first
      ordering; `ErrPass` still causes dispatcher to try the next handler
- [ ] Conversation state (sessions / wizards) via in-memory or persistent store

See [ADR 0008](./docs/adr/0008-bot-framework.md) for the API sketch.

## Phase 7 — Niceties **(planned, out of MVP)**

- [ ] Attachments (CDN3 upload/download, attachment cipher via libsignal)
- [ ] Storage Service sync (contacts, group list)
- [ ] CDSI contact discovery
- [ ] SQLite-backed store
- [ ] Backup/restore (linked-device "synchronized start")
- [ ] **Suppress the `missing .note.GNU-stack section implies executable stack`
      linker warning** on every Go build that links libsignal_ffi.a. The
      warning comes from a BoringSSL assembly object inside libsignal's
      static archive that lacks the `.note.GNU-stack` ELF section, so
      GNU ld assumes it wants an executable stack and warns. The Go-
      produced binary is fine (Go's linker injects PT_GNU_STACK as
      non-exec regardless), it's just noisy. Options:
      1. Post-process `libsignal_ffi.a` in `scripts/build-libsignal.sh`:
         extract objects, run `objcopy --add-section .note.GNU-stack=/dev/null`
         (or a small `as` snippet) on the offending member, repack.
      2. Patch upstream BoringSSL / submit a PR to `signalapp/boring`
         to add `.note.GNU-stack` to the affected `.S` files (the
         long-term fix; tracked at the libsignal layer).
      3. **Done (stop-gap shipped)**: `-Wl,--no-warn-execstack` added to
         `internal/libsignal/cgo.go` linux `#cgo LDFLAGS`. Hides the
         warning without fixing the root cause. The long-term fix (upstream
         BoringSSL `.note.GNU-stack` patch) is tracked at the libsignal
         layer.

## Phase 8 — Security audit **(planned; required before v0.1.0)**

A focused review before we cut a `v0.1.0` tag and put `signal-go` in front
of real Signal accounts. Scope is **our Go code and our cgo boundary** —
libsignal itself is out of scope (we trust upstream Signal). See
[ADR 0011](./docs/adr/0011-security-audit.md) for the methodology,
threat model, and what "pass" means.

Internal review (we do this before any external work):

- [ ] **Memory safety + profiling audit** (`internal/libsignal` and all
      cgo boundaries):
  - Run `go test -run=. -memprofile=mem.out ./...` and inspect with
    `go tool pprof` for unexpected heap growth across long-running
    receive + send sessions
  - Verify every `*Buffer` lifetime, `keepAlive`, finalizer, and
    `cgo.Handle` is correctly accounted for — a missed `keepAlive`
    allows the GC to collect a slice whose backing array is still
    referenced by Rust
  - Confirm `CiphertextMessage` (currently no finalizer in `session.go`)
    is either explicitly destroyed or harmlessly short-lived before any
    GC cycle
  - Run `valgrind --tool=memcheck` (or `sanitizers` via `CGO_CFLAGS=
    -fsanitize=address`) on a cgo-linked test binary to catch
    use-after-free and double-free in the C/Rust boundary
  - No Go pointers cross into Rust except via the documented
    borrowed/owned rules in `doc.go`
  - Errors free their underlying `SignalFfiError` exactly once
  - Confirm we link the *release* `libsignal_ffi.a`, not any
    `*-testing*` variant
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

## Continuous integration & quality **(ongoing)**

Cross-cutting infrastructure that runs on every PR and merge to `main`.
Design: [ADR 0013](./docs/adr/0013-ci-github-actions.md).

Phase A — bootstrap (this PR):
- [x] `.github/workflows/ci.yml`: lint + vet + build + test + govulncheck
      on `ubuntu-latest`, with cached `libsignal_ffi.a` keyed on the
      pinned tag
- [x] `.github/workflows/codeql.yml`: CodeQL security scanning, weekly
      schedule + manual dispatch (deliberately not per-PR — avoids a
      second parallel libsignal build per event)
- [x] `.github/dependabot.yml`: weekly bumps for Go modules + actions
- [x] CI status badge in [`README.md`](./README.md)

Phase B — broaden:
- [ ] macOS runners (`macos-latest`) with their own libsignal cache
- [ ] Windows runners (`windows-latest`) once we've validated the cgo
      build path
- [ ] `staticcheck` and (post-triage) `gosec` as separate jobs
- [x] Coverage report uploaded as a PR check (`ci.yml` `cover` job;
      `task cover` locally)

Phase C — release pipeline (lands with v0.1.0):
- [ ] `.github/workflows/release.yml`: build `signal-go` binaries on
      tag push; publish to the GitHub Release
- [ ] Cross-compiled binaries for linux/{amd64,arm64} + darwin/arm64;
      Windows iff Phase B Windows runner is green
- [ ] Nightly fuzz job (Phase 8 dependency)

## Non-goals

- We will not implement the Signal protocol cryptography in Go. All crypto goes
  through libsignal.
- We will not target headless registration (creating a brand-new Signal account
  by phone number). Only linking as a secondary device.
- We will not ship a REST/HTTP wrapper in this repo (that can live in a separate
  repo on top of `pkg/signal`).
