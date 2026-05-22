# Security

Short version of [ADR 0011](./adr/0011-security-audit.md) and
[ADR 0012](./adr/0012-encrypted-store.md). For the full Phase-8
threat model see [`docs/security/threat-model.md`](./security/threat-model.md).
If you're reporting a vulnerability, jump to [Reporting](#reporting).

## Threat model

We defend against:

1. **Network attacker** on the wire to `chat.signal.org` — TLS 1.2+ to
   `*.signal.org` pins Signal's private root ([ADR 0034](./adr/0034-signal-tls-root-pinning.md));
   message content is protected by the Signal protocol via `libsignal`.
2. **Malicious peer** crafting envelopes to crash us, steal state, or
   forge an identity-key swap — every decrypt path fails closed; the
   IdentityStore enforces trust-on-first-use semantics.
3. **Local filesystem attacker** with read access to the store
   directory (stolen disk, leaked backup, snapshot) — credentials are
   sealed with AES-256-GCM by default; the key is never persisted.
4. **Buggy embedding application** that mishandles a returned slice —
   the public API hands out clones, never internal buffers.

We do **not** defend against:

- An attacker with code execution inside your process.
- Signal Messenger LLC as the service operator.
- Micro-architectural side channels at the cgo boundary.

## Cryptography

- **Protocol crypto** (Double Ratchet, X3DH, PQXDH, Sealed Sender,
  zkgroup, profile cipher, attachment cipher, sender certificates)
  flows through Signal's official [`libsignal`](https://github.com/signalapp/libsignal)
  Rust library. We pin to a fixed tag and statically link.
- **Commodity primitives we use directly**:
  - AES-256-GCM (`crypto/aes` + `crypto/cipher`) for the at-rest store
  - HMAC-SHA256, AES-256-CBC, HKDF for the provisioning cipher (key
    material itself flows through libsignal ECDH)
  - Argon2id (`golang.org/x/crypto/argon2`) for passphrase KDF —
    OWASP-recommended params: `t=3, m=64 MiB, p=4`

## Credentials at rest

By default, `signal-go link` prompts for a passphrase. The 32-byte AES
key is derived from your passphrase via Argon2id, and never written to
disk. The salt + KDF parameters live in `kdf.json` next to the
encrypted blob; tampering with them produces a wrong key and the GCM
tag check fails, so the corrupted state is detected cleanly.

If you need a non-interactive deployment, use `-passphrase-file <path>`
or instantiate `fsstore.NewWithKey([32]byte)` directly from your own
code and source the key from an OS keyring, HSM, or whatever your
threat model demands.

For details: [encrypted-store diagram](./diagrams/encrypted-store.md)
and [ADR 0012](./adr/0012-encrypted-store.md).

## License-driven trust

`signal-go` is AGPL-3.0-only and statically links the AGPL-licensed
`libsignal`. The combined binary is AGPL. If you deploy `signal-go`
(or anything built on it) as a network service, AGPL §13 applies and
you must offer source to your users.

We do this on purpose: every official Signal client (Signal-Desktop,
Signal-iOS, Signal-Android, signal-cli, signalmeow) is AGPL-family
licensed. The license forces a level playing field that supports the
security properties we care about. See [ADR 0009](./adr/0009-licensing.md).

## Audit status

Pre-alpha. We have **not yet** had an external audit. The Phase-8
internal-review checklist in the
[roadmap](../ROADMAP.md#phase-8--security-audit-internal-pass-done-external-pass-required-before-v010)
is what we satisfy before cutting `v0.1.0`. The internal pass is
documented in [ADR 0032](./adr/0032-phase-8-internal-audit.md);
[`docs/security/threat-model.md`](./security/threat-model.md) is the
canonical write-up.

## Reporting

Please **do not** open GitHub issues for security problems. The
canonical disclosure policy lives at [`SECURITY.md`](../SECURITY.md);
the short version:

1. Preferred: open a private advisory via GitHub Private Vulnerability
   Reporting.
2. Alternate: e-mail <signal-go-security@thehappydinoa.dev>. PGP is
   available on request; mention it in your first message and we'll
   exchange keys before you send the report body.

We acknowledge within 72 hours and triage within a week. Once a fix is
ready, we coordinate disclosure with you (default 90 days from triage).

If you would prefer Signal as the contact channel, mention it in your
email and we'll switch.
