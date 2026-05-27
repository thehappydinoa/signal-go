# signal-go threat model

This document is the canonical Phase-8 deliverable described in
[ADR 0011](../adr/0011-security-audit.md). It expands the short version
in [`docs/security.md`](../security.md) with concrete component-level
defences.

## Scope

The threat model covers *our* code: everything between Signal's official
[`libsignal`](https://github.com/signalapp/libsignal) Rust library and
the embedding application. We treat libsignal itself as trusted upstream
([ADR 0001](../adr/0001-overall-architecture.md),
[ADR 0009](../adr/0009-licensing.md)).

## Assets

| Asset | Where it lives | Loss impact |
| --- | --- | --- |
| ACI/PNI long-term identity keys | `internal/account.Account.{ACI,PNI}Identity`, persisted by `internal/store/fsstore` | Account impersonation |
| Account HTTP Basic password | `Account.Password` | Full account takeover |
| `AccountEntropyPool` (master backup key) | `Account.AccountEntropyPool` | Full backup decryption |
| ProfileKey | `Account.ProfileKey` (self) and `Client.knownProfileKeys` (peers) | Profile metadata exposure |
| Per-recipient session state | libsignal-owned, persisted via `internal/store.SignalStores` | Future-message forgery |
| zkgroup auth credentials | `Client.groupAuthCreds` | Group impersonation until cred expiry |
| Sender certificate | `Client.senderCert` | Time-bounded sealed-sender impersonation |

## Adversaries

### A1 — Network attacker on chat.signal.org

Has full read/write on the wire to Signal's servers. Defence:

- libsignal's Noise/Signal-protocol layer protects content end-to-end.
- Our `internal/web` and `internal/ws` clients hardcode TLS 1.2 as the
  floor via `tls.Config{MinVersion: tls.VersionTLS12}` and call
  `http.DefaultTransport.Clone()` so HTTP/2 and connection pooling are
  preserved without the caller losing the floor. See
  [`internal/web/client.go`](../../internal/web/client.go) and
  [`internal/ws/client.go`](../../internal/ws/client.go).
- Callers can pin the trust store with `web.Options.PinnedRootCAs`
  ([ADR 0011 §"TLS posture"](../adr/0011-security-audit.md)) when their
  deployment policy requires it.
- Production `*.signal.org` hosts use Signal's **private** TLS root (vendored
  `signal-messenger.cer`), not public CAs — see
  [ADR 0034](../adr/0034-signal-tls-root-pinning.md) and
  [Certifiably Fine](https://signal.org/blog/certifiably-fine/).
- When the platform does not expose a usable system root pool (cgo Windows),
  [`internal/tlsroots`](../../internal/tlsroots/) also registers Mozilla NSS
  fallbacks via `golang.org/x/crypto/x509roots/fallback` for non-Signal hosts.
- No credentials are ever placed in URL query strings. `Credentials.Header`
  emits an `Authorization: Basic …` value; the `req.Path` and
  `req.Query` arguments to `web.Client.Do` are scrubbed at the call site.

### A2 — Malicious peer / forged envelope

Another Signal user (or someone impersonating one) sends an
adversarially-crafted envelope hoping to:

- panic the decrypt loop and crash us;
- produce a misleading typed event;
- pivot our identity store into accepting a forged identity-key swap.

Defences:

- Every parsing-heavy boundary is structurally validated *before* it
  feeds libsignal. The `internal/provisioning` cipher is the canonical
  example: version byte, length floor, MAC, padding length, and PKCS-7
  pad bytes are all checked with constant-time helpers before the
  protobuf unmarshal. The same boundary now has Go fuzz coverage; see
  [`FuzzDecryptEnvelope`](../../internal/provisioning/fuzz_test.go) and
  [`FuzzPKCS7Unpad`](../../internal/provisioning/fuzz_test.go) (5 min /
  target nightly via [`.github/workflows/fuzz-nightly.yml`](../../.github/workflows/fuzz-nightly.yml)).
- The chat receive loop (`pkg/signal.Client.processEnvelope`) catches
  every decrypt error, emits a `*DecryptionErrorEvent`, and continues —
  one bad envelope cannot kill the connection.
- Sealed-sender unwraps go through libsignal's `sealed_sender_decrypt`
  path, which validates the embedded `SenderCertificate` against the
  pinned production trust root (see
  [`internal/libsignal/sealed_sender.go`](../../internal/libsignal/sealed_sender.go)
  → `validateSenderCert`). A peer cannot forge sealed-sender mail.
- Identity-key trust is TOFU at the store layer; conflicting identities
  are surfaced to the caller through `IdentityStoreImpl.IsTrustedIdentity`
  rather than silently overwritten.

### A3 — Local filesystem attacker (cold disk)

Read access to the store directory after process shutdown (stolen
disk, leaked backup, snapshot). Defences:

- `internal/store/fsstore` defaults to AES-256-GCM at rest with an
  Argon2id-derived key the user supplies via `-passphrase-file` or
  prompt ([ADR 0012](../adr/0012-encrypted-store.md)); the encryption
  is now the documented default and the plaintext mode (`fsstore.New`)
  is test-only and refuses to open a directory that already holds an
  `account.enc`.
- Directory permission `0o700`, file permission `0o600`, writes go via
  `atomicWrite` (temp + rename in the same directory).
- The key is never persisted; the only on-disk auxiliary is
  `kdf.json`, holding the salt and KDF parameters. A wrong passphrase
  fails closed with `ErrWrongPassphrase` on GCM tag mismatch.

### A4 — Buggy / hostile in-process embedder

A misbehaving embedding application that:

- logs an `*account.Account` value at `slog.Info`;
- stashes a returned slice and mutates it.

Defences:

- `Account` and `Identity` implement `slog.LogValuer`. Logging an
  account through any `*slog.Logger` redacts `Password`, `PrivateKey`,
  `ProfileKey`, `AccountEntropyPool`, and shortens the phone number to
  its country code, while still showing identifiers and per-field
  presence so diagnostics remain useful. See
  [`internal/account/account.go`](../../internal/account/account.go).
- Public APIs hand back copies for any byte slice the client also
  caches internally (`signal.Client.SetRecipientProfileKey` clones on
  input, `FetchExpiringProfileKeyCredential` returns a fixed-size array,
  …).

### A5 — Out of scope

We do **not** defend against:

- An attacker with code execution inside the host process.
- Signal Messenger LLC as the service operator (metadata is visible to
  the service by design).
- Side-channel attacks at the cgo boundary itself. libsignal addresses
  microarchitectural timing inside its own code; one cgo crossing is
  considered too coarse to be a useful side channel.

## Memory-safety contract

See [`internal/libsignal/doc.go`](../../internal/libsignal/doc.go) for
the canonical statement. Summary:

- Every wrapper around a Rust-owned opaque pointer holds a
  `runtime.SetFinalizer` that frees it exactly once.
- Wrappers with explicit `Destroy` methods clear the finalizer so the
  same pointer is never freed twice. The Phase-8 sweep added this to
  `CiphertextMessage` and `PreKeyBundle`, which previously relied on
  manual destruction.
- Every borrowed `[]byte` passed via `borrowed(...)` is paired with a
  `keepAlive(...)` so the GC cannot reclaim the backing slice while
  Rust still has the pointer.
- Every `cgo.Handle` is created via `savePointer`, pinned, and freed via
  `deletePointer` in the C-to-Go callback path.
- Every `*SignalFfiError` is freed exactly once by `checkError`.
- The release `libsignal_ffi.a` is the only build we ever link
  (`scripts/build-libsignal.sh` pins `cargo build --release`); no
  `*-testing` variants ever enter a production binary.

## Audit checklist (Phase 8 internal review)

See [ROADMAP §"Phase 8 — Security audit"](../../ROADMAP.md#phase-8--security-audit-internal-pass-done-external-pass-required-before-v010).
Items currently satisfied:

- [x] cgo memory-safety sweep — `CiphertextMessage` finalizer added,
      doc comment formalises the contract.
- [x] `internal/provisioning` cipher review — constant-time MAC,
      constant-time PKCS-7 unpad, structural validation, fuzz coverage.
- [x] `internal/store/fsstore` review — `0o700`/`0o600`, atomic
      rename, encrypted-by-default, no secret-bearing field is ever
      passed to `slog`.
- [x] `internal/web` TLS posture — explicit `MinVersion: TLS 1.2`,
      opt-in CA pinning, `InsecureSkipVerify` panics on any prod
      `chat.signal.org` base URL.
- [x] Sealed-sender certificate validation — fixed production trust root
      and per-message validate via libsignal.
- [x] zkgroup credential cache eviction hook — `InvalidateGroupAuthCache`.
- [x] `go vet`, `staticcheck`, `golangci-lint`, `govulncheck` clean on
      `main`. `gosec` deferred (post-triage, ADR 0013).
- [x] Fuzz corpora live with the package; CI runs them 5 minutes / target
      nightly.
- [x] Long-running receive heap/CPU profile bake — see
      [`docs/guides/profiling.md`](../guides/profiling.md#phase-8-long-running-receive-bake-recorded).

Pending items (handed off to a follow-up):

- [ ] `go test -race -count=10` matrix run on CI (today is `-count=1`).
- [ ] `valgrind --tool=memcheck` / ASAN profile of a cgo-linked test
      binary — methodology documented here, run gated by maintainer
      bandwidth.
- [ ] External audit per [ADR 0011](../adr/0011-security-audit.md).
