# ADR 0017 — Profile fetch and UAK derivation

- Status: Accepted
- Date: 2026-05-22

## Context

Phase 4 sealed-sender delivery requires the recipient's 16-byte
unidentified access key (UAK). The UAK is derived from the recipient's
32-byte profile key via libsignal's `ProfileKey::derive_access_key` — not
HKDF, despite older comments elsewhere in the tree.

Before this ADR, callers had to invoke `Client.SetRecipientUAK` manually.
Profile keys arrive on the wire inside inbound `DataMessage.profileKey`
fields; display names and about text live in encrypted blobs fetched from
`GET /v1/profile/{aci}/{profileKeyVersion}`.

libsignal v0.94.1 exposes:

- `signal_profile_key_derive_access_key`
- `signal_profile_key_get_profile_key_version`
- `signal_aes256_gcm_siv_*`

It does **not** expose a `ProfileCipher` type. Name/about decryption uses
the same wire format as libsignal-service-java's `ProfileCipher`, built on
top of HKDF(profileKey, info="ProfileKey") + AES-256-GCM-SIV (version 1) or
legacy AES-256-GCM (version 0).

## Decision

### Layering

| Layer | Responsibility |
|-------|----------------|
| `internal/libsignal` | UAK derivation, profile key version, AES-GCM-SIV, service ID parse |
| `internal/profile` | ProfileCipher-compatible field decrypt |
| `internal/web` | `GET /v1/profile/{aci}/{version}` + `Unidentified-Access-Key` header |
| `pkg/signal` | `FetchProfile`, `SetRecipientProfileKey`, inbound profile-key cache |

### UAK auto-population

1. Inbound `DataMessage.profileKey` → store on `Client` → derive UAK.
2. `SetRecipientProfileKey` / `FetchProfile` → same path.
3. `sendContent` calls `ensureRecipientUAK` before checking `knownUAKs`.

Sealed-sender activates automatically once any of the above provides a
profile key; explicit `SetRecipientUAK` remains for callers that already
have the UAK without the profile key.

### Public API

```go
prof, err := client.FetchProfile(ctx, recipientACI, profileKey)
// profileKey may be nil if cached from an inbound message
client.SetRecipientProfileKey(aci, profileKey)
```

`Profile` exposes `GivenName`, `FamilyName`, `About`, `AboutEmoji`,
`AvatarPath`, and `DisplayName()`.

### Out of scope (Phase 5)

- Expiring profile key credential exchange (`?credentialType=expiringProfileKey`)
- Avatar CDN download
- Unversioned profile fetch (`GET /v1/profile/{aci}` without version)

## Consequences

- **Pro**: Sealed-sender works out of the box after the first inbound
  message carrying a profile key — the common bot case.
- **Pro**: ProfileCipher stays in Go atop libsignal AEAD primitives; no
  new third-party deps (ADR 0002 preserved).
- **Con**: Profile fetch requires the profile key upfront; contacts without
  a known key still fall back to basic-auth send until one arrives.
- **Con**: Legacy profile blobs (version 0 AES-GCM) use stdlib `crypto/aes`
  for decrypt — acceptable precedent (provisioning cipher uses Go AES-CBC).

## References

- [ADR 0015](./0015-sealed-sender-encrypt.md) — sealed-sender UAK dependency
- libsignal KAT: `rust/zkgroup/src/api/profiles/profile_key.rs`
