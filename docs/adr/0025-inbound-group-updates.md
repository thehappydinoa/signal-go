# ADR 0025: Inbound group update events and optional auto-sync

**Status:** Accepted  
**Date:** 2026-05-22

## Context

[ADR 0024](./0024-group-log-sync.md) added snapshot-based [SyncGroup] but
deferred inbound P2P `DataMessage.groupV2.groupChange` handling. Peers
deliver membership and metadata changes as DataMessages with an empty body
and a populated `groupChange` blob. Without dispatching these, bots only
learn about remote changes when they explicitly call [FetchGroup] or
[SyncGroup].

## Decision

1. **`GroupUpdateEvent`** — emit from `dispatch.go` when
   `DataMessage.groupV2.groupChange` is non-empty. If the body is empty,
   skip the companion [MessageEvent]; if both body and change are present,
   emit both events in order.

2. **Revision cache** — track the latest known revision per group (hex
   master key) in memory on [Client]. Populate after [FetchGroup],
   [FetchGroupLogs], and [SyncGroup]; delete on [LeaveGroup].

3. **Optional auto-sync** — [OpenOptions.AutoSyncGroupUpdates] (and
   [bot.Options.AutoSyncGroupUpdates]) spawn a background [SyncGroup]
   from the cached revision when an inbound update arrives. Failures are
   logged at WARN; the event is still emitted.

4. **Bot API** — [Bot.OnGroupUpdate] with a [GroupUpdate] handle exposing
   sender, group ID, revision, and the raw change bytes. [GroupUpdate.Sync]
   calls [SyncGroup] when the underlying client supports it.

5. **Deferred:** decrypt and apply individual `GroupChange.Actions` locally
   without fetching storage snapshots.

## Consequences

- Bots can react to peer-driven roster or title changes without polling.
- Auto-sync keeps local revision state fresh but adds background storage
  traffic; default off.
- [GroupUpdate.Sync] is a convenience wrapper; handlers may also call
  [Client.SyncGroup] directly via [Bot.Underlying].

## References

- [ADR 0024](./0024-group-log-sync.md)
- Signal `DataMessage.groupV2` in [`proto/SignalService.proto`](../../proto/SignalService.proto)
