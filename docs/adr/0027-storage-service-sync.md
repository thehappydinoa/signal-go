# ADR 0027: Storage Service sync (pull-only v1)

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 7 tracks Storage Service sync — the encrypted contact list and
Groups v2 chat list that official Signal clients keep in
`storage.signal.org`. Linked devices receive `SyncMessage.FetchLatest
{STORAGE_MANIFEST}` and `SyncMessage.Keys {accountEntropyPool}` over the
chat websocket when the primary device updates storage or rotates keys.

At link time we already persist `AccountEntropyPool` from the provisioning
message ([ADR 0012](./0012-encrypted-store.md)), but until this ADR we
never derived storage keys or called `/v1/storage/*`.

Signal Desktop derives keys as:

1. `masterKey = AccountEntropyPool.deriveSvrKey()` (libsignal FFI)
2. `storageServiceKey = HMAC-SHA256(masterKey, "Storage Service Encryption")`
3. `manifestKey = HMAC-SHA256(storageServiceKey, "Manifest_{version}")`
4. `itemKey = HMAC-SHA256(storageServiceKey, "Item_{base64(storageID)}")`
   (or HKDF when `recordIkm` rotation is present)

Manifest and item blobs use AES-256-GCM with a 12-byte IV prefix (Desktop
`encryptProfile` / `decryptProfile`), not the profile version byte.

REST flow:

| Step | Host | Endpoint |
|------|------|----------|
| Auth | chat | `GET /v1/storage/auth` → `{username, password}` |
| Manifest | storage | `GET /v1/storage/manifest` or `.../version/{local}` (204 unchanged, 404 missing) |
| Read | storage | `PUT /v1/storage/read` with `ReadOperation` protobuf |

## Decision

### Scope (v1)

- **Pull-only:** fetch manifest + decrypt CONTACT and GROUPV2 records.
- **Public API:** `Client.SyncStorage`, `StoredContacts`, `StoredGroups`,
  `StorageManifestVersion`.
- **Auto-sync:** optional `OpenOptions.AutoSyncStorage` handles inbound
  `FetchLatest(STORAGE_MANIFEST)`; `SyncMessage.Keys.accountEntropyPool`
  updates and persists AEP via `AccountStore`.
- **Profile keys:** contact profile keys populate the existing UAK cache
  (`SetRecipientProfileKey`) for sealed-sender send.

### Layering

| Layer | Responsibility |
|-------|----------------|
| `internal/libsignal` | `DeriveSVRKey` FFI wrapper |
| `internal/storage` | Key derivation, AES-GCM decrypt, pull orchestration |
| `internal/web` | `/v1/storage/auth`, manifest, read REST |
| `pkg/signal` | `SyncStorage`, caches, sync-message hooks |
| `pkg/bot` | `Options.AutoSyncStorage` passthrough |

### Deferred

- Write/upload manifest (`PUT /v1/storage/`)
- `recordIkm` rotation path (HKDF item keys — decrypt supported, no local rotation state)
- AccountRecord, stories, stickers, call links, chat folders
- CDSI contact discovery, SQLite-backed store, backup/restore

## Consequences

- Bots and library callers can enumerate contacts and group master keys
  from storage without scraping message history.
- `StoredGroup.MasterKey` feeds existing `FetchGroup` / `SyncGroup`.
- Accounts linked before AEP enrollment return `ErrStorageNotConfigured`
  until the primary sends a keys sync.

## References

- Signal Desktop `storage.preload.ts`, `Crypto.node.ts`
- [ADR 0012](./0012-encrypted-store.md), [ADR 0017](./0017-profile-fetch.md)
- `proto/StorageService.proto`
