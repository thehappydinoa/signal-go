# ADR 0011 — Security audit methodology and threat model

- Status: Accepted
- Date: 2026-05-20

## Context

`signal-go` is a Signal client. Mishandling messages, keys, or identities
can leak conversation content, impersonate users, or break end-to-end
guarantees that users rely on. Before we cut `v0.1.0` and encourage
real-account use, we need a documented audit pass with clear pass/fail
criteria.

This ADR does *not* replicate libsignal's own audit — Signal's Rust
cryptography is upstream's responsibility and we treat it as trusted
(ADR 0001, ADR 0009). The audit's scope is **everything between
libsignal and the user**: our cgo boundary, our protocol plumbing, our
persistence layer, and our public API.

## Threat model

### Adversaries we defend against

1. **Network attacker** with full read/write on the wire to
   `chat.signal.org`. Already addressed at the TLS layer by libsignal +
   our `chat` package; the audit verifies our TLS posture and any
   off-band metadata we add.
2. **Malicious peer**: another Signal user (or someone impersonating
   one) crafting envelopes designed to crash us, exfiltrate state, or
   convince our store to accept a forged identity key. The decrypt
   pipeline + identity store + sealed-sender certificate validation must
   fail closed on every malformed input.
3. **Local file-system attacker** with read access to our store
   directory after we've shut down. Our defences are FS permissions
   (`0700`/`0600`), not encryption at rest; we document this clearly.
   A future ADR may add at-rest encryption, but it is out of scope for
   v0.1.0.
4. **Buggy / hostile in-process Go code** (e.g. an embedding application
   that mishandles a returned slice). Our public API hands out clones,
   not aliased slices, where the data could be cached internally.

### Adversaries we do **not** defend against

- An attacker with code execution inside the host process. Once
  arbitrary Go can run alongside us, the password file is readable
  regardless of what we do.
- Signal itself, the operator of the service. Both Signal and any party
  it cooperates with can observe message metadata at the server (this
  is a property of Signal-the-service, not signal-go).
- Side-channel attacks on cgo boundary crossings (timing, cache).
  libsignal addresses these in its own code; the boundary itself is
  considered too coarse to be a useful side channel.

## Methodology

The audit is staged: **internal review** before **external review**.

### Internal review

A repository-level checklist (mirrored in ROADMAP Phase 8) covering:

- **cgo correctness**: every Rust-owned buffer is freed exactly once;
  every `cgo.Handle` is paired with a `Delete`; finalizers don't double-
  free; pointer ownership matches libsignal's documented rules.
- **Cryptographic discipline**: constant-time comparisons (`hmac.Equal`,
  `subtle.ConstantTimeCompare`) on every MAC / padding check; structural
  validation precedes any cryptographic operation so we never feed
  unverified data to libsignal in a way that could leak side-channel
  information.
- **Storage hygiene**: filesystem permissions; atomic writes via
  temp-file + rename; no key material in logs (we'll add an
  `slog.Replacer` that scrubs `password` and `*Key` fields).
- **TLS posture**: `tls.Config{MinVersion: VersionTLS12}` (or 1.3)
  explicit on every dialer; opt-in CA pinning for users who want it;
  no credentials embedded in URLs.
- **Failure modes**: every fallible operation produces a typed error
  the caller can branch on; the receive loop never dies on a single
  bad envelope.
- **Tooling**: `go vet`, `staticcheck`, `gosec`, `govulncheck`,
  `golangci-lint` configured and clean; `-race -count=10`; fuzz targets
  for the parsing-heavy boundaries (provisioning cipher, websocket
  message envelope, content protobuf).

### External review

After the internal pass is green:

1. Identify reviewers familiar with Signal protocols and Go cgo
   bindings (a reviewer who has touched `pkg/libsignalgo` would be
   ideal).
2. Provide them the pinned commit, threat model (this ADR), and the
   internal-checklist output.
3. Publish their report + our remediation under
   `docs/security/audits/<YYYY-MM>-<auditor>.md`.

### Bug-bounty / responsible disclosure

A separate `SECURITY.md` at the repository root will document:

- How to report a vulnerability (encrypted contact channel)
- Our PGP key or age public key for the contact
- Expected response time
- Coordinated-disclosure expectations

This file lands in the same PR as the audit prep.

## "Pass" definition

We consider the internal review passed when:

1. The Phase-8 checklist in ROADMAP.md has every item checked.
2. CI runs `vet + staticcheck + gosec + govulncheck + golangci-lint +
   -race -count=10 ./...` on every PR and all green.
3. The fuzz corpora live in `testdata/fuzz/` and CI runs them for at
   least 5 minutes each on a nightly schedule.

We consider the external review passed when the auditor's findings are
either fixed in `main` or explicitly accepted with a written rationale
in the audit report.

Only after both passes can we cut `v0.1.0`.

## Consequences

- **Pro**: Users of `signal-go` get a documented trust story rather
  than "we wrote it carefully, trust us".
- **Pro**: Forces us to wire `gosec` / `govulncheck` / fuzz into CI now,
  before the codebase is too large.
- **Con**: External audit costs money or favours. Mitigation: we land
  v0.1.0-rc on the internal pass alone, and treat the external pass as
  a separate v0.1.0 gate.
