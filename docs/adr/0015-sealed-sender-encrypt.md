# ADR 0015 — Sealed-sender encrypt: per-device USMC + sender-cert cache

- Status: Accepted
- Date: 2026-05-21

## Context

Signal's sealed-sender feature ([Signal blog](https://signal.org/blog/sealed-sender/))
prevents the server from learning who is sending a message. Without it the
`PUT /v1/messages/{aci}` endpoint includes HTTP Basic auth credentials that
carry the sender's ACI. With it, the sender is hidden behind a
`SenderCertificate` signed by Signal's server and embedded inside an
`UnidentifiedSenderMessageContent` (USMC) envelope; the PUT is made without
an Authorization header, using an `Unidentified-Access-Key` (UAK) header
derived from the recipient's profile key.

We have two FFI functions that could implement outbound sealed sender:

1. `signal_sealed_sender_multi_recipient_encrypt` — designed for group
   messages where the same sender-key ciphertext is encrypted for many
   recipients in one call.
2. Per-device: `signal_encrypt_message` + `signal_unidentified_sender_message_content_new`
   + `signal_unidentified_sender_message_content_serialize` — constructs one
   USMC per destination device from that device's Double Ratchet ciphertext.

## Decision

### Per-device USMC (not `multi_recipient_encrypt`)

For 1:1 messages the `multi_recipient_encrypt` path is designed for group
sender-key distribution and is inappropriate here. We use the per-device
approach:

1. For each device: `EncryptMessage` → `CiphertextMessage`
2. `NewUSMC(cipher, senderCert)` wraps the ciphertext in a USMC
3. `usmc.Serialize()` gives the bytes for the envelope `Content` field
4. Envelope type = `CiphertextTypeUnidentifiedSender` (6)
5. PUT is made with `Unidentified-Access-Key: {base64(uak)}` header via
   `web.SendMessageUnidentified`

This mirrors how Signal-Desktop handles 1:1 sealed sender.

### Sender-certificate cache

`GET /v1/certificate/delivery` returns a certificate valid for ~24 hours.
We cache it in `Client.senderCert` with a `certExpiry` timestamp and
re-fetch when the expiry is within 5 minutes.

The 5-minute headroom avoids using an about-to-expire cert in flight.

### UAK dependency on profile fetch (Phase 5)

The `Unidentified-Access-Key` is derived from the recipient's profile key
via HKDF-SHA256. We do not yet implement profile fetch (`/v1/profile/{aci}`
is Phase 5). Until then:

- `Client.knownUAKs` (a `map[string][]byte`) holds UAKs set externally via
  `SetRecipientUAK(aci, uak)`.
- `Send` checks for a UAK before attempting sealed sender; if none is set it
  falls back to basic-auth delivery with no error or warning to the caller.
- Phase 5 will call `SetRecipientUAK` after decrypting each profile.

This layering means sealed sender is fully operational when the UAK is
available, and the fallback is transparent to callers.

### Retry parity

Both `deliverBasicAuth` and `deliverSealed` implement the same at-most-one
retry on 409/410 via `handleSendError`. `handleSendError` was simplified to
return only the updated address list (no longer builds the request internally),
so both delivery paths rebuild the request with their own format after session
fixup.

## Consequences

- **Pro**: Server privacy for 1:1 messages activates automatically once Phase 5
  populates `knownUAKs`. No API surface change needed at that time.
- **Pro**: Per-device USMC matches Signal's own client behaviour; the protocol
  is correct.
- **Con**: UAK dependency means sealed sender cannot be used until Phase 5
  completes. Basic-auth is the interim path.
- **Con**: `multi_recipient_encrypt` is not used here; it will be needed for
  group sealed-sender (Phase 5 / group v2).
- **Security**: The `SenderCertificate` is validated on the receive path
  (`sealed_sender.go`). On the send path we trust the cert fetched from Signal's
  servers over TLS. Future Phase 8 audit should confirm expiry enforcement.

## Linked records

- [Phase 4 roadmap — sealed sender](../../ROADMAP.md#phase-4--send-11-in-progress)
- [ADR 0010 — Phase 3 receive](./0010-phase-3-receive.md)
- [Signal blog: sealed sender](https://signal.org/blog/sealed-sender/)
