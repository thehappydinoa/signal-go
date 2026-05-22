# ADR 0021: Group control messages (reactions, typing, receipts)

**Status:** Accepted  
**Date:** 2026-05-22

## Context

[ADR 0016](./0016-control-messages-and-edits.md) shipped 1:1 receipts, typing,
and reactions plus bot helpers on `*bot.Message`. [ADR 0019](./0019-group-sender-key.md)
shipped group text send via sender-key + multi-recipient delivery. Bot helpers
still returned `ErrReplyNotSupportedInGroup` for React, Typing, MarkRead, and
MarkViewed in group threads.

Signal's wire behaviour differs by control-message kind:

- **Reactions** ride in a `DataMessage.Reaction` with a `GroupContextV2`
  (master key + revision), delivered via the same sender-key multi-recipient
  path as group text.
- **Typing** is a top-level `TypingMessage` with a 32-byte `groupId`
  (derived from group public params, not the master key), also sender-key
  multi-recipient with `online=true`.
- **Read / viewed receipts** have no group field; they are sent 1:1 to the
  message author only (same as Signal Android / signal-cli).

## Decision

1. **`libsignal.GroupIdentifierFromMasterKey`** wraps
   `signal_group_public_params_get_group_identifier` for group typing.

2. **`Client.SendGroupReaction`** and **`Client.SendGroupTyping`** fetch
   group state, build the appropriate `Content`, and deliver via shared
   **`deliverGroupPayload`** (SKDM distribution + sender-key encrypt +
   multi-recipient PUT). Delivery flags mirror 1:1: reactions
   `urgent=true, online=false`; typing `urgent=false, online=true`.

3. **Read/viewed receipts in groups** reuse existing **`SendReceipt`** to
   `message.Sender` — no new group send API.

4. **`bot.Message` helpers** branch on `IsGroup()`:
   - React / Unreact / Typing → group send methods
   - MarkRead / MarkViewed → 1:1 receipt to author (group guard removed)

5. **`sendGroupContent`** (group change notifications) uses
   `deliverGroupPayload` with `distributeSKDM=false` to preserve prior
   behaviour.

## Consequences

- Bots can react and show typing indicators in groups once they hold the
  master key from an inbound group message.
- MarkRead/MarkViewed in groups notify only the message author, not all
  members — matching Signal client behaviour.
- `pkg/bot.Client` grows by two methods (`SendGroupReaction`,
  `SendGroupTyping`).

## References

- [ADR 0016](./0016-control-messages-and-edits.md), [ADR 0019](./0019-group-sender-key.md)
- Signal-Android `GroupSendUtil.sendTypingMessage`
- [ROADMAP Phase 5/6](../../ROADMAP.md)
