# ADR 0018: Groups v2 bootstrap — fetch and decrypt group state

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 5 requires authenticated access to Signal's Groups v2 storage service,
decryption of the encrypted group protobuf, and a typed public API so bots can
inspect membership and admin roles (`m.Group().IsAdmin(m.Sender())`).

Signal-Android (`GroupsV2Api`, `GroupsV2AuthorizationString`) and the chat
server's `GET /v1/certificate/auth/group` endpoint define the credential and
authorization flow. Group state lives at `GET /v2/groups/` on
`storage.signal.org`, authorized via HTTP Basic auth whose username is
hex(`GroupPublicParams`) and password is hex(`AuthCredentialPresentation`).

## Decision

1. **`internal/libsignal/zkgroup.go`** wraps the pinned libsignal v0.94.1 FFI
   for production `ServerPublicParams`, `GroupSecretParams` derivation from a
   32-byte master key, blob/service-id decrypt, auth credential receive, and
   presentation creation.

2. **`internal/web/groups.go`** implements:
   - `GET /v1/certificate/auth/group` on the chat service (7-day credential
     window, cached per redemption day on the client).
   - `GET /v2/groups/` on the storage service (raw protobuf response).

3. **`internal/group/decode.go`** decrypts `groupspb.Group` fields (title,
   description, member ACIs and roles). Profile-key presentation members are
   rejected in this bootstrap — they are rare in established groups and land in
   a follow-up.

4. **`pkg/signal.FetchGroup(ctx, masterKey)`** returns a public `Group` with
   `Members`, `Admins()`, and `IsAdmin(aci)`. `Group.ID` is the hex-encoded
   master key, matching inbound `MessageEvent.GroupID`.

5. **`Client` holds a second `web.Client` pointed at `storage.signal.org`
   (`OpenOptions.GroupsStorageURL` overrides for tests) and an in-memory
   zkgroup auth credential cache keyed by UTC redemption day.

## Consequences

- Bots can fetch and inspect group membership once they hold the master key
  (from an inbound group message's `groupV2.masterKey`).
- Group send/receive (sender-key distribution, encrypt/decrypt) remain Phase 5
  follow-ups; `bot.Message.Reply` in groups is still blocked.
- Auth credentials are cached in-memory only; persistent cache can land with
  Storage Service sync.
- Tests use httptest fakes and libsignal round-trip vectors; no live group
  traffic in unit tests.

## References

- Signal-Android `GroupsV2Api.java`, `GroupsV2AuthorizationString.java`
- Signal-Server `CertificateController.getGroupAuthenticationCredentials`
- [`proto/Groups.proto`](../../proto/Groups.proto)
- [ROADMAP Phase 5](../../ROADMAP.md#phase-5--groups-v2-done)
