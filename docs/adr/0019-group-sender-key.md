# ADR 0019: Groups v2 sender-key distribution and group send/receive

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 5 requires group message encrypt/decrypt using Signal's sender-key
protocol, SKDM distribution to members, and multi-recipient sealed-sender
delivery. The libsignal v0.94.1 FFI already exposes
`signal_group_encrypt_message`, `signal_group_decrypt_message`,
`signal_process_sender_key_distribution_message`, and
`signal_sealed_sender_multi_recipient_encrypt`; the Go store bridge had
sender-key callbacks but no `SenderKeyStoreStruct()` accessor.

Inbound group ciphertext arrives as `Envelope_UNIDENTIFIED_SENDER` with a
multi-recipient sealed-sender payload whose inner USMC type is sender-key (7).
SKDMs arrive as standalone `Content.senderKeyDistributionMessage` over 1:1
sessions.

## Decision

1. **`internal/libsignal/sender_key.go`** wraps SKDM create/process/serialize,
   group encrypt/decrypt, UUID helpers, and `StoreHandle.SenderKeyStoreStruct()`.

2. **Receive path**
   - `cipher.EnvelopeDecryptor` tries
     `MultiRecipientMessageForSingleRecipient` before sealed-sender decrypt.
   - `decryptUSMCInner` handles `CiphertextSenderKey` via `GroupDecryptMessage`.
   - `Client.dispatchContent` processes inbound SKDMs before routing other
     content variants.
   - `stripContentPadding` applied before Content protobuf parse.

3. **Send path — `Client.SendGroup(ctx, masterKey, text)`**
   - Fetches membership via existing `FetchGroup`.
   - Caches a random distribution UUID per group master key (in-memory).
   - Fan-outs SKDM to each member over the existing 1:1 `sendContent` pipe.
   - Encrypts padded Content with sender key; wraps in USMC with group master key.
   - Delivers via `PUT /v1/messages/multi_recipient` with combined UAK (bitwise
     XOR of member UAKs). Requires known UAK/profile key per member.

4. **`bot.Message.Reply`** in groups decodes `GroupID` (hex master key) and
   calls `SendGroup`.

5. **Deferred:** group send endorsement tokens (preferred server auth for
   multi-recipient), persistent sender-key / distribution-ID storage in fsstore,
   group reactions/typing/receipts.

## Consequences

- Bots can reply in groups once they hold member UAKs (from inbound profile keys
  or explicit `SetRecipientProfileKey` / `FetchProfile`).
- Combined UAK is legacy but still accepted by Signal Server; endorsement tokens
  land in a follow-up.
- Distribution IDs are in-memory only; restarting the client generates a new ID
  (SKDM re-distribution on next send handles this).
- No live group traffic in unit tests; httptest fakes + libsignal round-trips.

## References

- [ADR 0018](./0018-groups-v2-bootstrap.md)
- [ADR 0015](./0015-sealed-sender-encrypt.md)
- Signal-Server `MessageController.sendMultiRecipientMessage`
- [ROADMAP Phase 5](../../ROADMAP.md#phase-5--groups-v2-in-progress)
