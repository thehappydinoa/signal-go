# ADR 0022: Profile-key presentations, add/remove member, distribution persistence, wizard

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 5 deferred several items after endorsements and control messages:

- **Profile-key presentation members** — `decode.go` rejected `Member.presentation`,
  breaking fetch for groups that use zkgroup presentations (common for newer members).
- **Add member** — requires expiring profile key credentials + presentations.
- **Remove member** — admin kick (distinct from self-[LeaveGroup]).
- **Distribution UUID persistence** — sender-key distribution IDs were in-memory only;
  sender-key *records* already persist via [SenderKeyStore].
- **Bot wizard sugar** — [ADR 0008](./0008-bot-framework.md) deferred a multi-step
  builder on top of [Convo.SetStage].

## Decision

1. **`internal/libsignal/profile_key_presentation.go`** wraps credential request
   context, credential receive, presentation create, and presentation UUID extract.

2. **`group.decodeMember`** decrypts presentation members via
   [ProfileKeyPresentationUUIDCiphertext] + [GroupSecretParamsDecryptServiceID].

3. **Membership APIs** on [Client]:
   - [AddMember] — admin adds member; fetches expiring profile key credential via
     `GET /v1/profile/{aci}/{version}/{requestHex}?credentialType=expiringProfileKey`,
     builds presentation, PATCH addMembers.
   - [RemoveMember] — admin removes another member (deleteMembers).

4. **[GroupDistributionStore]** — optional `OpenOptions.GroupDistributionStore`
   (`memstore` + `fsstore/group_dists.json`) persists hex master key → distribution UUID.

5. **Bot wizard** — [Bot.Wizard], [Wizard.Step], [Wizard.Register], [Wizard.Begin],
   [Wizard.Advance], plus [Bot.OnAnyText] for stage-gated steps.

## Consequences

- [FetchGroup] works for groups whose members use profile-key presentations.
- [AddMember] requires the target's 32-byte profile key (inbound message,
  [FetchProfile], or [SetRecipientProfileKey]).
- Invite-link join shipped in [ADR 0023](./0023-gse-persist-invite-join.md).
  Group log sync remains deferred.
- `fsstore.NewGroupDistributionStore(dir)` is separate from account store; CLI wiring
  can compose both under the same data directory.

## References

- [ADR 0018](./0018-groups-v2-bootstrap.md), [ADR 0020](./0020-group-endorsements-membership.md)
- libsignal `zkgroup/tests/integration_tests.rs` (credential round-trip timestamps)
- [ROADMAP Phase 5/6](../../ROADMAP.md)
