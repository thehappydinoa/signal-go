# ADR 0003 — Vendor `.proto` files from Signal-Android

- Status: Accepted
- Date: 2026-05-20

## Context

Signal's wire protocol is defined in protobuf files that live in several
repos:

- `signalapp/Signal-Desktop` — `protos/` (missing `Provisioning.proto` and
  `WebSocketResources.proto`)
- `signalapp/Signal-Android` — `lib/libsignal-service/src/main/protowire/`
  (complete) and `core/network/src/main/protowire/WebSocketResources.proto`
- `signalapp/Signal-iOS` — Swift bindings, harder to extract from
- `signalapp/libsignal` — `rust/protocol/src/proto/` contains the
  *internal* protocol protos (session record format, sender key state) but
  not the *service* protos

Signal-Android is the only single repo that has the complete set we need
for QR linking + send/receive + groups v2.

## Decision

Vendor the relevant `.proto` files from `signalapp/Signal-Android` under
`proto/`. Source repo, commit, and fetch date recorded in
`proto/UPSTREAM.txt`.

Files vendored (current set):

| File                       | Purpose                                       |
|----------------------------|-----------------------------------------------|
| `Provisioning.proto`       | QR-link provisioning messages                 |
| `WebSocketResources.proto` | Signal's wrapper over RFC-6455 websocket      |
| `SignalService.proto`      | Envelope, Content, DataMessage, SyncMessage   |
| `Groups.proto`             | Group v2 server protocol                      |
| `StorageService.proto`     | Encrypted contact/group storage sync          |
| `StickerResources.proto`   | Sticker pack metadata                         |
| `DeviceName.proto`         | Linked-device display name encoding           |

Generated Go code lands in `internal/proto/gen/<package>/` and is checked
in (so consumers don't need protoc unless they regenerate). Regeneration
is `task proto`.

## Consequences

- **Pro**: One trusted source, one commit pin.
- **Pro**: Generated code is reviewable in PRs.
- **Con**: We must manually re-vendor when Signal evolves the protocol.
  `task proto:check` will diff against upstream to catch drift.
- **Con**: Generated files inflate the repo. Acceptable.
