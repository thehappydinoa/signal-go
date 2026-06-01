# signal-go

[![CI](https://github.com/thehappydinoa/signal-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/thehappydinoa/signal-go/actions/workflows/ci.yml)
[![CodeQL](https://github.com/thehappydinoa/signal-go/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/thehappydinoa/signal-go/actions/workflows/codeql.yml)
[![Latest release](https://img.shields.io/github/v/release/thehappydinoa/signal-go)](https://github.com/thehappydinoa/signal-go/releases/latest)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0--only-blue)](./LICENSE)
[![Go version](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](./go.mod)
[![libsignal](https://img.shields.io/badge/libsignal-v0.94.3-orange)](./scripts/build-libsignal.sh)
[![Threat model](https://img.shields.io/badge/security-threat--model-2e7d32)](./docs/security.md)

A Go library and CLI that lets your program act as a linked **Signal**
secondary device. Cryptography flows through Signal's official Rust
[`libsignal`][libsignal] via a thin cgo binding; protocol plumbing
(websockets, REST, prekey lifecycle, sealed sender, groups v2) is
implemented in Go.

## Quick start

**From source** (Linux, macOS, or [Windows/MSYS2](./docs/guides/getting-started.md#windows-git-bash--msys2)):

```sh
git clone https://github.com/thehappydinoa/signal-go
cd signal-go
task setup      # once per clone: tools + git hooks
task libsignal  # once: build or download libsignal_ffi.a
task build      # → bin/signal-go
./bin/signal-go link -store ./.signal-data
```

**Pre-built binaries** — [GitHub Releases](https://github.com/thehappydinoa/signal-go/releases/latest).

**As a library** — `import "github.com/thehappydinoa/signal-go/pkg/signal"`.
You still need `libsignal_ffi.a`; see the [getting-started guide](./docs/guides/getting-started.md).

Full walkthrough: [`docs/guides/getting-started.md`](./docs/guides/getting-started.md).

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
    store["Persistence<br/>(account · store · sqlstore · seal)"]:::store

    bot --> pub
    pub --> proto
    pub --> store
    proto --> crypto
    crypto --> store
```

Full breakdown: [`docs/diagrams/architecture.md`](./docs/diagrams/architecture.md).

## Documentation

| Topic | Link |
|-------|------|
| Build, link, Windows setup | [`docs/guides/getting-started.md`](./docs/guides/getting-started.md) |
| Creating a Signal bot | [`docs/guides/creating-a-bot.md`](./docs/guides/creating-a-bot.md) |
| Cutting a release | [`docs/guides/releasing.md`](./docs/guides/releasing.md) |
| Testing strategy | [`docs/guides/testing.md`](./docs/guides/testing.md) |
| Bot examples | [`examples/`](./examples/) |
| Architecture diagrams | [`docs/diagrams/`](./docs/diagrams/) |
| Security + threat model | [`docs/security.md`](./docs/security.md) |
| Architecture decisions | [`docs/adr/`](./docs/adr/) |
| Changelog | [`CHANGELOG.md`](./CHANGELOG.md) |
| Roadmap | [`ROADMAP.md`](./ROADMAP.md) |

## Contributing

Read [`CONTRIBUTING.md`](./CONTRIBUTING.md) and [`CLAUDE.md`](./CLAUDE.md), then run
`task setup && task libsignal && task test && task lint` before opening a PR.

Security issues: [`SECURITY.md`](./SECURITY.md) — **do not** file public GitHub issues for vulnerabilities.

## License

[AGPL-3.0-only](./LICENSE). `signal-go` statically links AGPL-licensed `libsignal`;
network deployments must comply with AGPL §13. See [ADR 0009](./docs/adr/0009-licensing.md).

---

*Not affiliated with or endorsed by Signal Messenger LLC.
Upstream `libsignal` is "use outside of Signal is unsupported"; we pin to a fixed tag.*

[libsignal]: https://github.com/signalapp/libsignal
