# ADR 0020: Group send endorsements and membership changes

**Status:** Accepted  
**Date:** 2026-05-22

## Context

[ADR 0019](./0019-group-sender-key.md) shipped group send/receive using the
legacy combined UAK (bitwise XOR of member profile-key UAKs). Signal Server
now prefers **group send endorsement tokens** (`Group-Send-Token` header on
`PUT /v1/messages/multi_recipient`). Endorsements arrive in
`GroupResponse.group_send_endorsements_response` and
`GroupChangeResponse.group_send_endorsements_response`.

Phase 5 also requires membership changes (leave, promote/demote). Signal
applies these via `PATCH /v2/groups/` with serialized `GroupChange.Actions`;
the server returns a signed `GroupChange` plus fresh endorsements when
membership changes.

## Decision

1. **`internal/libsignal/group_send_endorsement.go`** wraps GSE receive/combine,
   endorsement combine, and full-token derivation via libsignal v0.94.1 FFI.

2. **Endorsement cache** on `Client` (in-memory, keyed by hex master key):
   - Populated on every successful [FetchGroup]
   - Used by [SendGroup] when valid; combined UAK remains fallback
   - Invalidated on [LeaveGroup]

3. **`web.SendMultiRecipientMessage`** accepts `MultiRecipientAuth` with
   either `Group-Send-Token` or combined UAK (mutually exclusive).

4. **Membership changes** via `internal/group/change.go` action builders and
   `web.PatchGroup`:
   - [LeaveGroup] — delete local member
   - [SetMemberRole] / [PromoteMember] / [DemoteMember] — admin-only role change
   - After role change, a group update `DataMessage` notifies peers (revision +
     optional serialized `GroupChange`)

5. **Deferred:** invite-link join, group log sync (`GET /v2/groups/logs/{version}`),
   persistent endorsement cache in fsstore.

## Consequences

- Group send no longer requires every member's profile key when endorsements
  are present from a recent fetch.
- Leave/role changes require storage-service auth (same zkgroup path as fetch).
- Endorsements expire; callers must re-fetch before send if cached material is stale.
- Add-member remains blocked on profile-key presentation FFI (same bootstrap
  gap as ADR 0018).

## References

- [ADR 0018](./0018-groups-v2-bootstrap.md), [ADR 0019](./0019-group-sender-key.md)
- Signal-Server `MessageController.sendMultiRecipientMessage`
- [`proto/Groups.proto`](../../proto/Groups.proto)
