# ADR 0023: Persistent GSE cache and invite-link join

**Status:** Accepted  
**Date:** 2026-05-22

## Context

[ADR 0020](./0020-group-endorsements-membership.md) deferred two Phase 5 items:

- **Persistent group send endorsement (GSE) cache** — endorsements were in-memory
  only; bots restarting had to re-fetch every group before [SendGroup].
- **Invite-link join** — groups could be fetched and modified by admins, but
  joining via `https://signal.group/#...` was not implemented.

Signal's storage service exposes:

- `GET /v2/groups/join/{base64(inviteLinkPassword)}` → `GroupJoinInfo`
- `PATCH /v2/groups/?inviteLinkPassword=...` for join actions

## Decision

1. **[GroupEndorsementStore]** — optional `OpenOptions.GroupEndorsementStore`
   with `memstore` + `fsstore/group_endorsements.json` (0600, atomic rename).
   [storeGroupSendEndorsements] write-through persists response, expiration, and
   per-member endorsement blobs. [groupSendTokenForRecipients] lazy-loads from
   disk on memory miss. [LeaveGroup] deletes persisted + in-memory entries.

2. **Invite link parsing** — [group.ParseInviteLinkURL] decodes the URL fragment
   as base64url `GroupInviteLink` protobuf (matches Signal-Android / signal-cli).

3. **Join APIs** on [Client]:
   - [ParseGroupInviteLink] / [PreviewGroupJoin] — preview title, member count,
     whether admin approval is required.
   - [JoinGroupViaInviteLink] — direct join (`AddMemberAction` with
     `joinFromInviteLink`) or pending request (`AddMemberPendingAdminApproval`)
     using the linked account's profile key + expiring credential presentation.

4. **Deferred:** inbound P2P `groupChange` application in the receive pipeline.
   Group log sync shipped in [ADR 0024](./0024-group-log-sync.md).

## Consequences

- [SendGroup] works after process restart when `GroupEndorsementStore` is wired
  and endorsements have not expired.
- Join requires a linked account with a profile key (same as Signal Desktop
  standalone join).
- `fsstore.NewGroupEndorsementStore(dir)` is separate from the account store;
  compose under the same data directory in CLI wiring.

## References

- [ADR 0020](./0020-group-endorsements-membership.md), [ADR 0022](./0022-phase5-finish.md)
- Signal storage-service `GroupsController` (`/v2/groups/join/`, invite query param)
- signal-cli `GroupInviteLinkUrl`, `GroupV2Helper.joinGroup`
