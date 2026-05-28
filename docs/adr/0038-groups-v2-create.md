# ADR 0038 — Groups v2 create and attribute changes

**Status:** Accepted  
**Date:** 2026-05-28

## Context

Phase 5 shipped fetch, send, membership changes (add/remove/leave/roles), and
invite-link **join**, but bots still could not **create** a group or change
metadata (title, description, invite link) without a phone-created group.
Signal-Android creates groups via `PUT /v2/groups/` with an encrypted initial
`groupspb.Group` protobuf (`GroupsV2Operations.createNewGroup` /
`GroupsV2Api.putNewGroup`).

## Decision

1. **`libsignal.GenerateGroupMasterKey`** wraps
   `signal_group_secret_params_generate_deterministic` + `get_master_key`.

2. **`internal/group`** adds wire builders:
   - `BuildNewGroupMessage` for PUT
   - `BuildModifyTitleActions`, `BuildModifyDescriptionActions`,
     `BuildEnableInviteLinkActions`
   - `FormatInviteLinkURL` / `GenerateInviteLinkPassword`

3. **`internal/web.PutGroup`** issues `PUT /v2/groups/`.

4. **`pkg/signal` public API**:
   - [CreateGroup] with [CreateGroupOptions] / [CreateGroupResult]
   - [SetGroupTitle], [SetGroupDescription]
   - [EnableGroupInviteLink], [GroupInviteLinkURL], [FormatGroupInviteLink]

5. After create, the client stores GSE from the response and sends a group
   update notification (empty `groupChange`) so members learn of the new group,
   matching Android's post-create fan-out at a minimal level.

Members without a known profile key are added as `membersPendingProfileKey`,
same as Signal-Android when credentials are unavailable.

## Consequences

- Bots can bootstrap alert groups without manual phone setup when member profile
  keys are known (inbound messages, [FetchProfile], [SetRecipientProfileKey]).
- Avatar upload on create remains out of scope (requires CDN form + patch).
- [AddMember] / [RemoveMember] from ADR 0022 are unchanged; create reuses the
  same presentation credential path for initial roster members.

## References

- Signal-Android `GroupsV2Operations.createNewGroup`, `GroupsV2Api.putNewGroup`
- [ADR 0018](./0018-groups-v2-bootstrap.md), [ADR 0022](./0022-phase5-finish.md),
  [ADR 0023](./0023-gse-persist-invite-join.md)
