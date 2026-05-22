# ADR 0026: Attachment cipher and CDN pipeline

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 7 tracks attachments (CDN3 upload/download, attachment cipher). The
ROADMAP mentions "attachment cipher via libsignal", but libsignal v0.94.1 FFI
exposes **incremental MAC primitives** only — no high-level `AttachmentCipher`
type (same situation as ProfileCipher in [ADR 0017](./0017-profile-fetch.md)).

Signal clients today use attachment **v2** (Signal Desktop): log padding,
AES-256-CBC (with PKCS7 block padding), trailing HMAC-SHA256, SHA-256 digest,
and optional incremental MAC (`video/mp4`). Legacy libsignal-service-java
PKCS7-only padding remains supported on decrypt via [DecryptAuto].

Upload uses `GET /v4/attachments/form/upload?uploadLength=N` and CDN3 TUS
creation-with-upload to `signedUploadLocation`.

## Decision

### Layering

| Layer | Responsibility |
|-------|----------------|
| `internal/libsignal` | `incremental_mac` / `validating_mac` FFI wrappers |
| `internal/attachment` | v2 encrypt/decrypt, legacy PKCS7, sticker HKDF |
| `internal/web` | Upload form, TUS upload, CDN download |
| `pkg/signal` | `SendAttachment`, `SendGroupAttachment`, `DownloadAttachment`, inbound `MessageEvent.Attachments` |
| `pkg/bot` | `Message.ReplyAttachment`, `Message.Attachments()` |

### Wire formats

**v2 (primary):** log-padded plaintext → AES-CBC → `IV || ciphertext || HMAC`;
digest = `SHA256(blob)`; incremental MAC over encrypted blob for streaming
types.

**Legacy:** PKCS7-padded plaintext (libsignal-service-java); decrypted when v2
fails and no incremental MAC metadata is present.

### CDN

- Form: authenticated `GET /v4/attachments/form/upload`
- Upload: TUS POST for `cdn == 3`; simple POST for CDN 2
- Download: `GET {cdnHost}/attachments/{cdnKey}` (public)

## Consequences

- Bots can send and receive file attachments in 1:1 and group threads.
- Outbound attachments interoperate with official Signal clients (v2 + CDN3).
- Incremental MAC generation is limited to `video/mp4`; other types omit it but
  decrypt inbound incremental attachments via libsignal validating MAC.

## References

- [ADR 0017](./0017-profile-fetch.md)
- Signal Desktop `AttachmentCrypto.node.ts`, `uploadAttachment.preload.ts`
- libsignal `GET /v4/attachments/form/upload` ([`ws/messages.rs`](https://github.com/signalapp/libsignal))
