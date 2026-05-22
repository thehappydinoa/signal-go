# ADR 0026: Attachment cipher in Go (classic AES-CBC + HMAC)

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 7 tracks attachments (CDN3 upload/download, attachment cipher). The
ROADMAP mentions "attachment cipher via libsignal", but libsignal v0.94.1 FFI
exposes **incremental MAC primitives** only — no high-level `AttachmentCipher`
type (same situation as ProfileCipher in [ADR 0017](./0017-profile-fetch.md)).

libsignal-service-java's `AttachmentCipherInputStream` / `OutputStream`
define the wire format Signal clients use for message attachments:

- 64-byte combined key (32-byte AES-256 key + 32-byte HMAC key), stored in
  `AttachmentPointer.key`
- Blob layout: `IV (16) || AES-CBC ciphertext || HMAC-SHA256 (32)`
- HMAC covers IV and ciphertext (not the trailing MAC bytes)
- Transmitted digest: `SHA256(blob_without_mac || mac)` — stored in
  `AttachmentPointer.digest`
- Sticker packs derive the 64-byte key via HKDF(packKey, info="Sticker Pack")

Newer attachments may also carry `incrementalMac` + `chunkSize` for chunked
verification; that path uses libsignal's `incremental_mac` / `validating_mac`
FFI and is deferred.

## Decision

1. **`internal/attachment`** — classic AttachmentCipher encrypt/decrypt,
   `NewKey`, `ExpandStickerPackKey`, and `CiphertextLength` helpers.

2. **Crypto in Go** atop stdlib `crypto/aes`, `crypto/cipher`, and
   `crypto/hmac`; HKDF for sticker keys via existing
   [libsignal.HKDFSHA256].

3. **Constant-time checks** — `subtle.ConstantTimeCompare` for MAC and digest
   verification.

4. **Deferred (follow-up PRs):**
   - Incremental-MAC attachment path (`AttachmentPointer.incrementalMac`)
   - CDN3 upload form + download HTTP (`internal/web`)
   - `Client.SendAttachment`, inbound attachment events, `bot.ReplyAttachment`

## Consequences

- Unblocks CDN and send/receive integration without waiting for upstream FFI.
- Matches libsignal-service-java test vectors (round-trip, MAC/digest failure).
- ROADMAP Phase 7 "attachment cipher" item can be split: cipher done, CDN next.

## References

- [ADR 0017](./0017-profile-fetch.md) — precedent for Go-side cipher
- libsignal-service-java `AttachmentCipherInputStream.java`
- [`proto/SignalService.proto`](../../proto/SignalService.proto) `AttachmentPointer`
