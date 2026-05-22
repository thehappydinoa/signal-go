# ADR 0031: Transfer archive frame import (v1)

**Status:** Accepted  
**Date:** 2026-05-22

## Context

[ADR 0030](./0030-linked-device-transfer-archive.md) shipped link-and-sync receive:
poll transfer archive, download ciphertext, validate via libsignal, return
`ArchiveBytes`. Callers still start with an empty contact/group cache and no
imported peer identity keys.

libsignal v0.94.1 exposes backup **validation** FFI (`MessageBackupValidator`,
`OnlineBackupValidator`) but no store-population API. Frame decryption uses
AES-256-CBC + HMAC-SHA256 + gzip; frames are varint-length-delimited protobuf
(`signal.backup.Frame` from libsignal's `backup.proto`).

Backups are an **application snapshot** (contacts, groups, chats, chat items),
not libsignal `SessionRecord` / `SenderKeyRecord` blobs. Sessions rebuild from
live traffic after link.

## Decision

1. **Vendor `backup.proto`** from the pinned libsignal tag and generate Go types
   under `internal/proto/gen/backuppbg`.

2. **`internal/backup`** package:
   - Decrypt validated ciphertext using `MessageBackupKey` AES/HMAC material
     (legacy + forward-secrecy header formats).
   - Iterate varint-delimited frames; validate incrementally with
     `OnlineBackupValidator` when importing.
   - Map frames to store writes (see scope below).

3. **Import scope (v1):**
   - **Contacts:** ACI/PNI/E164, profile key, display names, blocked flag;
     contact `identityKey` → [`store.IdentityStore`].
   - **Groups:** 32-byte master key, title from snapshot, blocked flag.
   - **Deferred:** `ChatItem` message history (no message store),
     `AccountData` settings merge, sticker packs, distribution lists.

4. **`store.BackupImportStore`** — new interface for durable contact/group list
   entries (implemented by `sqlstore`; in-memory test double in `memstore`).

5. **Public API:**
   - [`signal.ImportTransferArchive`](../../pkg/signal/backup_import.go) for
     standalone import after validation.
   - [`LinkOptions.ImportTransferArchive`](../../pkg/signal/link.go) (default
     `true` when `LinkAndSync` and stores are set) runs import inside
     [`SyncTransferArchive`](../../pkg/signal/link_sync.go) after validation.
   - [`OpenOptions.BackupImportStore`](../../pkg/signal/client.go) loads
     imported profile keys into the client on open.

6. **Docs / ROADMAP:** mark Phase 7 frame-import follow-up done for v1 scope;
   message-history import remains a separate track.

## Consequences

- Linked devices with link-and-sync restore contact identity keys and group
  master keys locally without storage-service sync.
- Full chat history parity requires a future message store and `ChatItem`
  import path.
- `backup.proto` must be re-vendored when bumping the libsignal pin if the
  schema changes.
- Import runs after account persistence during link; partial import failures
  surface as link errors (fail closed).

## Alternatives considered

- **Wait for libsignal JSON export FFI:** `BackupJsonExporter` exists in Rust
  but is not exposed to cgo; rejected to avoid blocking on upstream.
- **Import everything including ChatItems into JSON files:** no query path for
  bots; rejected.
- **Only return parsed frames to callers:** pushes persistence burden to every
  integrator; rejected for CLI/sqlstore users.
