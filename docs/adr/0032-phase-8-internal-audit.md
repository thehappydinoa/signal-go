# ADR 0032 — Phase 8 internal security audit pass

- Status: Accepted
- Date: 2026-05-22
- Supersedes: nothing
- Superseded by: nothing

## Context

[ADR 0011](./0011-security-audit.md) defined the methodology for the
Phase-8 security audit gate that blocks the `v0.1.0` tag. The roadmap
turned that methodology into a concrete check-list of internal-review
items. This ADR records what we did to satisfy the *internal* portion of
that gate; the external auditor pass remains a separate, future ADR.

## Decision

The internal review is documented as complete with the following
work — every change shipped in the PR that lands this ADR:

### Memory safety + cgo boundary

- `CiphertextMessage` and `PreKeyBundle` now have
  `runtime.SetFinalizer` *plus* an idempotent `Destroy` that clears the
  finalizer when called explicitly. Before this ADR `CiphertextMessage`
  had neither, so any code path that held the wrapper past a GC cycle
  leaked. ([`internal/libsignal/session.go`](../../internal/libsignal/session.go))
- The package-level doc comment in
  [`internal/libsignal/doc.go`](../../internal/libsignal/doc.go) gained
  a "Memory-safety contract" section that codifies every invariant the
  audit checks: finalizer-once, `keepAlive` pairing, pinned
  `cgo.Handle`, single-free error path, release-only linkage.
- Sealed-sender certificate validation against the production trust
  root is unchanged and remains the canonical example of a
  fail-closed-on-malformed-input path.

### Cryptographic discipline

- `internal/provisioning.DecryptEnvelope` already used `hmac.Equal` and
  constant-time PKCS-7 unpad. The audit added two `go test -fuzz` targets
  ([`FuzzDecryptEnvelope`](../../internal/provisioning/fuzz_test.go),
  [`FuzzPKCS7Unpad`](../../internal/provisioning/fuzz_test.go)) seeded
  with both well-formed and pathological envelopes. Nightly CI runs
  each fuzz target for 5 minutes per
  [`.github/workflows/fuzz-nightly.yml`](../../.github/workflows/fuzz-nightly.yml).

### Storage hygiene

- `internal/store/fsstore` is unchanged: `0o700` dir, `0o600` files,
  `atomicWrite` via temp + rename, encrypted-by-default via AES-256-GCM
  + Argon2id ([ADR 0012](./0012-encrypted-store.md)). The audit confirmed
  this is right.
- `internal/account.Account` and `internal/account.Identity` now
  implement `slog.LogValuer` so an accidental
  `logger.Info("linked", "account", acct)` does not dump the password,
  private keys, profile key, or AccountEntropyPool. Tests pump a real
  account through a TextHandler and assert no secret bytes survive.

### TLS posture

- `internal/web.New` is now an alias for `NewWithOptions(...)` which
  clones `http.DefaultTransport` and sets
  `TLSClientConfig.MinVersion = tls.VersionTLS12`. The same explicit
  floor applies to `internal/ws.Dial` via `mergeTLSConfig`, even when
  the caller supplies a `*tls.Config` with a lower (or zero) MinVersion.
- `web.Options.PinnedRootCAs` is the documented opt-in for replacing
  the system trust store. `web.Options.InsecureSkipVerify` exists for
  the test harness and panics on the production base URL so a
  misconfigured deployment cannot silently disable verification.

### zkgroup credential cache

- `signal.Client.InvalidateGroupAuthCache` is a new public method that
  drops every cached zkgroup auth credential. PNI-change sync handlers
  will call it; today's identity-rotation paths self-heal via the
  existing `ReceiveAuthCredentialWithPni` error branch, but an explicit
  hook is the audit's expectation.

### Tooling

- `staticcheck` joined the CI matrix
  ([`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)) and runs
  cleanly today.
- `task staticcheck` and `task fuzz` give local parity with CI; `task
  ci` invokes them.
- `gosec` remains gated on triage per
  [ADR 0013](./0013-ci-github-actions.md) Phase B.

### Documentation

- [`docs/security/threat-model.md`](../security/threat-model.md) is the
  long-form threat model referenced by ADR 0011.
- [`SECURITY.md`](../../SECURITY.md) lists private-vulnerability
  reporting, response targets, supported versions (none yet), and a
  safe-harbour clause. The PGP key + e-mail address remain placeholders
  pending the v0.1.0 cut.

## Consequences

- **Pro**: every internal-review checkbox in [ROADMAP §
  "Phase 8"](../../ROADMAP.md#phase-8--security-audit-internal-pass-done-external-pass-required-before-v010)
  except the optional `-race -count=10` matrix and the valgrind/ASAN
  bake is now satisfied. We are ready to tag `v0.1.0-rc1` once the
  external-audit gate decision is made.
- **Pro**: the threat model is publicly documented and CI now enforces
  the staticcheck + nightly-fuzz floor it depends on.
- **Con**: SECURITY.md still contains placeholder contact details. We
  will not cut `v0.1.0` until those are real, but the rest of the audit
  work is final.
- **Con**: an actual ASAN / memory-profile-driven sweep across long-running
  receive sessions is still pending. The methodology is documented
  inline in the threat model; the bake itself is a maintainer-bandwidth
  item, not a code item.
