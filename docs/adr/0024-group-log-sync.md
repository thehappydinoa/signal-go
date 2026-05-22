# ADR 0024: Group log sync via storage snapshots

**Status:** Accepted  
**Date:** 2026-05-22

## Context

Phase 5 deferred **group log sync** (`GET /v2/groups/logs/{version}`) after
GSE persistence and invite-link join ([ADR 0023](./0023-gse-persist-invite-join.md)).

Signal's storage service returns paginated `GroupChanges` protobufs. Each entry
may include a post-change `groupState` snapshot alongside the signed
`groupChange`. Full client implementations apply deltas locally; that requires
decrypting and verifying every change action.

Inbound P2P `DataMessage.groupV2.groupChange` updates are still ignored by the
receive pipeline — bots should call [SyncGroup] or [FetchGroup] explicitly.

## Decision

1. **`internal/web/groups_logs.go`** — `FetchGroupLogs` with
   `Cached-Send-Endorsements` header, query params (`limit`, `includeFirstState`,
   `includeLastState`, `maxSupportedChangeEpoch`), and 206 pagination via
   `Content-Range`.

2. **Public API** on [Client]:
   - [FetchGroupLogs] — one page, decoded `GroupChanges`
   - [SyncGroup] — page until caught up; decode the **latest `groupState`
     snapshot** from the log; fall back to [FetchGroup] if no snapshot present
   - Refresh GSE from the final page when the server includes fresh endorsements

3. **Change application strategy:** snapshot-based for now. We do not decrypt or
   apply individual `GroupChange.Actions` locally. This matches the common case
   where the storage service populates `groupState` when `includeLastState=true`.

4. **Deferred:** inbound P2P `groupChange` application in `dispatch.go`; local
   delta application without snapshots.

## Consequences

- Bots can catch up from a known revision without a full group refetch when log
  entries carry state snapshots.
- Multi-page fetches zero the cached GSE expiration header on subsequent pages
  (per storage-service semantics) so the last page can return fresh tokens.
- Groups whose log pages omit snapshots still work via [FetchGroup] fallback.

## References

- [ADR 0023](./0023-gse-persist-invite-join.md)
- Signal storage-service `GroupsController.getGroupLogs`
- [`proto/Groups.proto`](../../proto/Groups.proto) `GroupChanges`
