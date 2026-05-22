# ADR 0030: Linked-device transfer archive receive (v1)

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Signal's "synchronized start" flow lets a newly linked secondary device
download an encrypted message-history archive from the primary during
provisioning. The primary uploads ciphertext to the attachments CDN and
registers `{cdn,key}` via `PUT /v1/devices/transfer_archive`; the secondary
polls `GET /v1/devices/transfer_archive?timeout=…` until the archive is
ready, downloads `/attachments/{key}`, and decrypts/validates the bundle.

signal-go already completed device linking (Phase 2) and ships libsignal
FFI bindings for message-backup validation, but had no transfer-archive
client, no `backup3` provisioning capability, and ignored
`ProvisionMessage.ephemeralBackupKey`.

Importing backup frames into the local store (sessions, messages, groups)
is substantial work and depends on store schema decisions already tracked
for SQLite. We still want a shippable v1 that proves the receive path end
to end.

## Decision

1. **Provisioning:** when `LinkOptions.LinkAndSync` is true, advertise
   `backup3` in the linking URL (matching current Signal Desktop).

2. **Post-link receive:** if the decrypted `ProvisionMessage` contains a
   32-byte `ephemeralBackupKey`, call `SyncTransferArchive`:
   - poll `GET /v1/devices/transfer_archive` (204 = not ready, 200 = payload)
   - download via existing `DownloadAttachmentCDN`
   - derive `MessageBackupKey` from the ephemeral key + ACI-derived backup ID
     (no forward-secrecy token — device-transfer path)
   - validate with `signal_message_backup_validator_validate` and
     `Purpose::DeviceTransfer`

3. **Scope limit (v1):** stop after successful validation. Return the
   validated ciphertext in `SyncTransferArchiveResult.ArchiveBytes`; do
   **not** import frames into `fsstore`/`sqlstore` yet.

4. **GNU-stack warning:** post-process `libsignal_ffi.a` in
   `scripts/build-libsignal.sh` with `objcopy --add-section .note.GNU-stack`
   on archive members missing the section; remove the
   `-Wl,--no-warn-execstack` stop-gap from `cgo.go`.

5. **Go wrappers:** add `internal/libsignal` sync input-stream callbacks,
   `MessageBackupKey`, and `ValidateMessageBackup`; add
   `internal/web/transfer_archive.go`.

## Consequences

- Linked devices can complete link-and-sync receive and prove backup
  integrity, but still start with an empty local message store until a
  follow-up implements frame import.
- Callers opt in via `LinkAndSync`; default linking behaviour is unchanged.
- CI/dev clones must rebuild `libsignal_ffi.a` after pulling (existing
  requirement); the build script now patches ELF objects once per build.
- Future import work should reuse `ArchiveBytes` or re-download from CDN
  and share protobuf frame parsing with libsignal's backup schema.

## Alternatives considered

- **Full import in this PR:** too large; mixes crypto/receive with store
  migration and bot state.
- **Keep `-Wl,--no-warn-execstack` only:** hides the warning without fixing
  the underlying BoringSSL object; rejected per ROADMAP option 1.
- **`backup5` capability string:** upstream Desktop v7.47 still advertises
  `backup3`; we match Desktop rather than guessing a newer token.
