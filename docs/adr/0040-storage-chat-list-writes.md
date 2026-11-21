# ADR 0040 — Storage service chat list writes

- Status: Accepted
- Date: 2026-05-28

## Context

[ADR 0027](./0027-storage-service-sync.md) shipped **pull-only** storage sync so
bots can read the phone's contact and group lists. Linked devices that create
groups or send DMs via signal-go updated Groups v2 state and message history on
Signal's servers, but **did not** update the encrypted storage manifest the
primary phone uses for its chat list. Users saw successful bot operations with
no corresponding conversation on the phone.

Official Signal clients publish CONTACT and GROUPV2 records with
`PUT /v1/storage/` whenever conversations are created or updated.

## Decision

1. **`internal/storage/push.go`** — merge upserts into the current manifest
   (preserving all existing identifier types), encrypt, and upload via
   `WriteOperation`.
2. **`internal/web.WriteStorageRecords`** — `PUT /v1/storage/`.
3. **`Client` hooks** (enabled by default; opt out with
   `[OpenOptions.DisableSyncStorageWrites]`):
   - After successful 1:1 send → upsert `ContactRecord`
   - After inbound DM → upsert contact for sender
   - After group create / add member / join / group send → upsert
     `GroupV2Record`
4. Writes require `AccountEntropyPool`; skip with debug log when storage is not
   configured or manifest is missing.
5. On success, refresh local caches via `[SyncStorage]`. Retry once on HTTP 409
   manifest conflict.

### Out of scope (unchanged)

- Storage manifest creation for accounts with no remote manifest
- `recordIkm` rotation writes
- AccountRecord / chat folder / sticker record edits
- Outbound `SyncMessage.FetchLatest(STORAGE_MANIFEST)` to nudge the primary
  (primary pulls on its own schedule after manifest version bumps)

## Consequences

**Pro** — Bot-created DMs and groups appear on the linked phone's chat list when
storage sync is configured.

**Pro** — Manifest merge preserves non-chat identifier types from the existing
manifest.

**Con** — Concurrent writes from phone and bot can 409; retry + re-pull mitigates
but does not eliminate races.

**Con** — Accounts without `AccountEntropyPool` still cannot publish (same as
pull-only storage sync).

Extends [ADR 0027](./0027-storage-service-sync.md).

## References

- Signal Desktop storage write path (`storage.preload.ts`)
- `proto/StorageService.proto` — `WriteOperation`, `ContactRecord`, `GroupV2Record`
