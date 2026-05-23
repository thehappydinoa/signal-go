# ADR 0036: Linked-device display name encryption (Go)

**Status:** Accepted  
**Date:** 2026-05-23

## Context

`PUT /v1/devices/link` carries `accountAttributes.name`. Official clients
encrypt this field so the plaintext device label is not visible to TLS
terminators that log JSON. The pinned libsignal v0.94.1 **cbindgen** surface
does not expose a `signal_device_name_*` helper; the algorithm lives in
Signal Android (`DeviceNameCipher.kt`) as standard X25519 agreement plus
HMAC-SHA256 key separation and AES-256-CTR.

## Decision

1. Implement the cipher in **`internal/devicename`**, mirroring Signal
   Android's `DeviceNameCipher` (including UTF-8 handling for `"cipher"` /
   `"auth"` labels and zero IV for CTR).

2. Use **libsignal's** [`libsignal.Agree`](../../internal/libsignal/agree.go)
   for the X25519 shared secret so scalar multiplication stays inside the
   vetted Rust build.

3. Use **stdlib** `crypto/hmac`, `crypto/sha256`, and `crypto/cipher` for the
   KDF chain and AES-CTR (same primitives Android uses via `javax.crypto`).

4. At link time, encrypt against the **ACI identity public key** from the
   `ProvisionMessage` (the account identity the secondary shares after link).

5. Wire [`signal.Link`](../../pkg/signal/link.go) / [`buildLinkRequest`](../../pkg/signal/link.go):
   non-empty `LinkOptions.DeviceName` → base64-standard protobuf
   `signalservice.DeviceName` as the JSON `name` field; empty name stays
   omitted.

6. Expose [`devicename.Decrypt`](../../internal/devicename/cipher.go) for
   tests and operator diagnostics only (not a public `pkg/signal` API).

## Consequences

- Device names match Android's linked-device list behaviour.
- We maintain a small crypto surface outside libsignal; any Android-side
  change to `DeviceNameCipher` must be ported here and covered by the
  round-trip unit test.
- No libsignal pin bump is required solely for this feature.

## Alternatives considered

- **Bump libsignal** until cbindgen exports device-name helpers: rejected
  as unnecessary churn for a stable, small algorithm already public in
  Signal-Android.
- **Plaintext name forever:** rejected — inconsistent with other clients
  and exposes the label on the link JSON hop.
