# signal-go

<!--
  CI + CodeQL badges:
  GitHub's actions/workflows/*.yml/badge.svg URLs don't render for
  unauthenticated viewers of private repos, so we use static shields.io
  badges until the repo goes public. When that happens, swap back to:

    [![CI](https://github.com/thehappydinoa/signal-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/thehappydinoa/signal-go/actions/workflows/ci.yml)
    [![CodeQL](https://github.com/thehappydinoa/signal-go/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/thehappydinoa/signal-go/actions/workflows/codeql.yml)
-->
[![CI](https://img.shields.io/badge/CI-GitHub%20Actions-2088FF?logo=githubactions&logoColor=white)](./.github/workflows/ci.yml)
[![CodeQL](https://img.shields.io/badge/CodeQL-weekly-2088FF?logo=github)](./.github/workflows/codeql.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0--only-blue)](./LICENSE)
[![Go version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](./go.mod)
[![libsignal](https://img.shields.io/badge/libsignal-v0.94.1-orange)](./scripts/build-libsignal.sh)
[![Status](https://img.shields.io/badge/status-pre--alpha-red)](./ROADMAP.md)
[![Threat model](https://img.shields.io/badge/security-threat--model-2e7d32)](./docs/security.md)

A Go library that lets your program act as a linked **Signal**
secondary device. Cryptography flows through Signal's official Rust
[`libsignal`][libsignal] via a thin cgo binding; the protocol plumbing
(websockets, REST, prekey lifecycle, sealed sender, groups v2) is
implemented in Go.

> **Pre-alpha.** Linking, real-time receive, and 1:1 send all work
> end-to-end with encrypted-at-rest persistence and an in-process
> two-client test. Sealed-sender, auto-retry on device mismatch, and
> groups v2 are the next slices. Track progress in the
> [roadmap](./ROADMAP.md).

```sh
go install github.com/thehappydinoa/signal-go/cmd/signal-go@latest  # once we tag v0.1.0
signal-go link -store ./.signal-data
```

The CLI prompts for a passphrase, prints a `sgnl://linkdevice?...` URL
to scan, and persists an AES-256-GCM-encrypted account on disk.
See [`docs/guides/getting-started.md`](./docs/guides/getting-started.md)
for the full walkthrough.

## Highlights

- **Trust-preserving by design** — every cryptographic operation goes
  through the same `libsignal` that ships in the official Signal apps.
  No re-implemented protocol crypto.
- **PQXDH-ready** — Curve25519 + ML-KEM 1024 prekey generation and
  upload at link time, matching Signal's 2026 mandate.
- **Encrypted credentials at rest** — AES-256-GCM with an Argon2id-
  derived key (or a caller-supplied raw key). Passphrase prompts +
  `-passphrase-file` for non-interactive deployments. See
  [the encrypted-store diagram](./docs/diagrams/encrypted-store.md).
- **Small dependency surface** — `coder/websocket`, `google.golang.org/protobuf`,
  and `golang.org/x/crypto` (Argon2id). Everything else is stdlib.
- **Send + receive working** — `signal.Client.Send(ctx, recipient, text)`
  handles bundle fetch, session establishment, and message encryption;
  receive is a real-time chat-ws loop emitting typed events on
  `Client.Events()`.
- **Bot dispatch out of the box** — `pkg/bot` wraps the client with
  `OnText`, `OnPrefix`, `OnRegex`, `OnCommand("/help")`,
  `OnReaction`/`OnAnyReaction`, `OnEdit`, plus DM/Group/From scopes,
  per-handler + global middleware, and `*Message` helpers (`Reply`,
  `React`, `Typing`, `MarkRead`, `MarkViewed`).
  ([ADR 0008](./docs/adr/0008-bot-framework.md))

## Architecture

```mermaid
flowchart TB
    classDef pub fill:#d6f5d6,stroke:#3a7d3a,color:#000
    classDef proto fill:#dde7ff,stroke:#3a5fb8,color:#000
    classDef crypto fill:#ffe7c2,stroke:#a3661a,color:#000
    classDef store fill:#f5d6e8,stroke:#a13a78,color:#000

    pub[pkg/signal<br/><i>public API</i>]:::pub
    bot[pkg/bot<br/><i>OnText / OnRegex / OnCommand</i>]:::pub
    proto["Protocol layer<br/>(provisioning · web · ws · prekeys · chat)"]:::proto
    crypto[internal/libsignal<br/><i>cgo + libsignal_ffi.a</i>]:::crypto
    store["Persistence<br/>(account · store · fsstore · memstore)"]:::store

    bot --> pub
    pub --> proto
    pub --> store
    proto --> crypto
    crypto --> store
```

Full breakdown: [`docs/diagrams/architecture.md`](./docs/diagrams/architecture.md).

## What works · what's next

| Phase | Status | Scope |
|---|---|---|
| [1 — Foundation](./ROADMAP.md#phase-1--foundation-done) | ✅ | cgo to libsignal, ws layer, QR-link handshake |
| [2 — Complete the link](./ROADMAP.md#phase-2--complete-the-link-done-except-where-noted) | ✅ | ProvisioningCipher, prekey gen, REST registration, prekey upload |
| [Encrypted store](./docs/adr/0012-encrypted-store.md) | ✅ | AES-256-GCM at rest, Argon2id passphrase mode |
| [3 — Receive](./ROADMAP.md#phase-3--receive-done) | ✅ | authenticated chat ws, libsignal decrypt, typed events, prekey top-up |
| [4 — Send 1:1](./ROADMAP.md#phase-4--send-11-done) | ✅ done | bundle fetch + session establish + encrypt + `PUT /v1/messages` + sealed-sender + auto-retry + multi-device + control messages + profile fetch / UAK auto-derive |
| [5 — Groups v2](./ROADMAP.md#phase-5--groups-v2-planned) | ⏳ | zkgroup + sender keys + membership / admin surface |
| [6 — Bot framework](./ROADMAP.md#phase-6--bot-framework-in-progress) | 🔧 in progress | OnText / OnRegex / OnCommand / OnReaction / OnEdit + DM/Group/From scopes + middleware + `Reply`/`React`/`Typing`/`MarkRead`; conversation state next |
| [6 — Bot framework](./ROADMAP.md#phase-6--bot-framework-done) | ✅ | dispatchers, middleware, wizard, group updates |
| [7 — Niceties](./ROADMAP.md#phase-7--niceties-planned-out-of-mvp) | ⏳ | storage sync ✅, CDSI, SQLite, backup |
| [8 — Security audit](./ROADMAP.md#phase-8--security-audit-planned-required-before-v010) | ⏳ | internal + external review gates `v0.1.0` |

## Docs

- 📐 **[Diagrams](./docs/diagrams/)** — architecture, QR-link, encrypted store, receive, send
- 🛠️ **[Getting started](./docs/guides/getting-started.md)** — build, link your first device
- 🧪 **[Testing](./docs/guides/testing.md)** — unit, component, e2e against real Signal
- 🔒 **[Security](./docs/security.md)** — threat model, encrypted-at-rest, reporting
- 📋 **[Roadmap](./ROADMAP.md)** — staged plan through `v0.1.0`
- 📜 **[ADRs](./docs/adr/)** — every architectural decision recorded
  ([0001 architecture](./docs/adr/0001-overall-architecture.md),
  [0002 deps](./docs/adr/0002-no-third-party-go-deps.md),
  [0009 license](./docs/adr/0009-licensing.md),
  [0010 receive](./docs/adr/0010-phase-3-receive.md),
  [0011 audit](./docs/adr/0011-security-audit.md),
  [0012 encrypted store](./docs/adr/0012-encrypted-store.md)…)

## Disclaimer

Not affiliated with, endorsed by, or supported by Signal Messenger LLC.
Upstream `libsignal` is published with the explicit caveat *"use
outside of Signal is unsupported"*; we pin to a fixed tag and ride the
API breaks ourselves.

## License

[AGPL-3.0-only](./LICENSE). `signal-go` statically links AGPL-licensed
`libsignal`, so the combined binary is AGPL. If you deploy `signal-go`
(or anything built on it) as a network service, AGPL §13 obliges you to
offer source to your users. See [ADR 0009](./docs/adr/0009-licensing.md)
for the full reasoning and alternatives considered.

[libsignal]: https://github.com/signalapp/libsignal
