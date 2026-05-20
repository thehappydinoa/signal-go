# ADR 0005 — Storage interface + filesystem reference implementation

- Status: Accepted
- Date: 2026-05-20

## Context

A linked Signal device must persist:

- Account state (ACI/PNI UUIDs, device ID, account-wide registration IDs)
- ACI and PNI identity keypairs
- Trusted-identity table (keyed by recipient address)
- One-time prekeys, signed prekeys, Kyber prekeys (both ACI and PNI)
- Sessions (Double Ratchet state, keyed by `(address, deviceId)`)
- Sender keys (groups v2, keyed by `(distribution UUID, sender)`)
- Profile keys per recipient
- Group state (master key, latest revision, member list)

libsignal callbacks (`SignalIdentityKeyStore`, `SignalPreKeyStore`,
`SignalSignedPreKeyStore`, `SignalKyberPreKeyStore`, `SignalSessionStore`,
`SignalSenderKeyStore`) are invoked synchronously from FFI; we hand them Go
closures bridged through `cgo.Handle`.

Users will plausibly want: in-memory (tests), filesystem (single-process),
SQLite (production), and potentially custom (cloud KMS).

## Decision

Define a `Store` interface in `internal/store` with per-domain sub-interfaces
(`IdentityStore`, `PreKeyStore`, `SignedPreKeyStore`, `KyberPreKeyStore`,
`SessionStore`, `SenderKeyStore`, `ProfileStore`, `GroupStore`). The
top-level `Store` aggregates them.

Ship two reference implementations:

1. `internal/store/memstore` — in-memory, used by tests.
2. `internal/store/fsstore` — directory of JSON files (one per record kind).
   Adequate for personal use; not concurrent-safe across processes.

A SQLite-backed impl is post-MVP and lives in a separate package so it can
pull in cgo'd SQLite as an opt-in dep.

## Consequences

- **Pro**: Pluggable; users with HSMs/KMS can implement the interface.
- **Pro**: Tests get a fast in-memory backend.
- **Con**: Cross-cutting transactions (e.g. "consume one-time prekey and
  install session atomically") need a transactional `Store.Update(func(tx)
  error)` method. We will add this once we hit the case in Phase 3.
