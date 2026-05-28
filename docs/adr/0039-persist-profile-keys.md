# ADR 0039 — Persist recipient profile keys

- Status: Accepted
- Date: 2026-05-28

## Context

[ADR 0017](./0017-profile-fetch.md) caches recipient profile keys in memory
(`Client.knownProfileKeys`) when they arrive on inbound `DataMessage.profileKey`,
via `FetchProfile`, or `SetRecipientProfileKey`. That cache drives UAK
derivation for sealed-sender send and profile-key credential exchange for
Groups v2 `AddMember`.

With the SQLite store ([ADR 0029](./0029-sqlite-backed-store.md)), sessions
and prekeys survive restarts, but profile keys did not. A bot that received
one DM from a peer could send sealed-sender messages in-process, yet lost the
peer's profile key after restart — breaking group admin flows and e2e tests
unless the env var `SIGNAL_E2E_PEER_PROFILE_KEY` was set or the peer sent
another message.

Link-and-sync backup contacts (`backup_contacts.profile_key`) cover contacts
imported at link time only, not keys learned later at runtime.

## Decision

1. Add **`store.ProfileKeyStore`** with `LoadProfileKeys`, `StoreProfileKey`,
   and `DeleteProfileKey`.
2. Add SQLite table **`recipient_profile_keys`** (`aci`, `profile_key`) in
   `sqlstore`; implement **`memstore.ProfileKeyStore`** for tests.
3. Add **`OpenOptions.ProfileKeyStore`**. When set, `Client` persists on every
   successful `storeRecipientProfileKey` and deletes on
   `SetRecipientProfileKey(aci, nil)`.
4. On **`Open`**, load persisted keys after backup import so runtime-learned
   keys override stale backup entries for the same ACI.
5. Wire **`db.ProfileKeyStore()`** in `OpenFromStore`, e2e helpers, and
   examples alongside existing sqlstore accessors.

Profile keys remain in the same SQLite file as other crypto state (same
at-rest posture as sessions and `backup_contacts.profile_key`).

## Consequences

**Pro** — Sealed-sender and group `AddMember` work after restart once a peer
has sent any message carrying a profile key.

**Pro** — E2e group-admin tests no longer require `SIGNAL_E2E_PEER_PROFILE_KEY`
after a single prior DM.

**Con** — Another sensitive column in `signal.db`; callers must protect the
store directory and passphrase like other key material.

**Con** — Duplicate storage with `backup_contacts.profile_key` for imported
contacts; acceptable — backup rows are import metadata, `recipient_profile_keys`
is the operational runtime cache.

Extends [ADR 0017](./0017-profile-fetch.md); does not change the public
`SetRecipientProfileKey` / `FetchProfile` API.

## References

- [ADR 0017](./0017-profile-fetch.md), [ADR 0029](./0029-sqlite-backed-store.md)
- `pkg/signal/profile.go`, `internal/store/sqlstore/profile_key.go`
