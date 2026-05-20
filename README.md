# signal-go

A Go library for the Signal Private Messenger.

`signal-go` lets your Go program act as a linked Signal device: pair via QR code,
then send and receive messages on behalf of an existing Signal account. All
cryptography is delegated to Signal's official Rust [`libsignal`][libsignal] via
a thin cgo binding; the protocol plumbing (websocket framing, REST, prekey
lifecycle, sealed sender, groups v2) is implemented in Go.

> **Status: pre-alpha, Phase 1.** Not yet usable. See [ROADMAP.md](./ROADMAP.md)
> for what works and what's next.

## Why

Existing options for talking to Signal from Go are either (a) shelling out to
`signal-cli` (a Java daemon) like [`signal-cli-rest-api`][cli-rest], or
(b) using [`signalmeow`][signalmeow], the AGPL-3.0 Go client extracted from the
mautrix-signal bridge. `signal-go` is a from-scratch implementation with a
small audited dependency surface (see [ADR 0002](./docs/adr/0002-no-third-party-go-deps.md))
that keeps all cryptography in Signal's own audited code.

Eventually, `signal-go` will ship a `pkg/bot` package (see
[ADR 0008](./docs/adr/0008-bot-framework.md)) that makes writing Signal
bots about as easy as Telegram or Slack Bolt.

## Architecture

```
+----------------------------------------+
|  pkg/signal               (public API) |
+----------------------------------------+
|  internal/provisioning    (QR linking) |
|  internal/web             (REST)       |
|  internal/ws              (websocket)  |
|  internal/store           (persistence)|
|  internal/proto/gen       (generated)  |
+----------------------------------------+
|  internal/libsignal       (cgo)        |
+----------------------------------------+
|  libsignal_ffi.a   (Signal's Rust lib) |
+----------------------------------------+
```

See [docs/adr/](./docs/adr/) for the architectural decisions behind each layer.

## Building

You need Go 1.24+, a C toolchain, Rust (for building libsignal once), and
`protoc`. Then:

```sh
task libsignal   # ~10 min the first time; cached afterwards
task proto       # regenerate Go from .proto files
task build       # build the library and demo CLI
task test        # run unit tests
task lint        # run golangci-lint
```

Run `task --list` to see all available tasks.

## Disclaimer

This project is not affiliated with, endorsed by, or supported by Signal
Messenger LLC. Per upstream [`libsignal`][libsignal]: "Use outside of Signal is
unsupported." APIs in libsignal can change without notice; we pin to a known
version.

[libsignal]: https://github.com/signalapp/libsignal
[signalmeow]: https://github.com/mautrix/signal/tree/main/pkg/signalmeow
[cli-rest]: https://github.com/bbernhard/signal-cli-rest-api

## License

[AGPL-3.0-only](./LICENSE). signal-go statically links Signal's
[`libsignal`](https://github.com/signalapp/libsignal), which is itself
AGPL-3.0-only, so the combined work is AGPL. If you deploy signal-go (or
anything built on it) as a network service, the AGPL §13 source-availability
obligation applies to your users. See [ADR 0009](./docs/adr/0009-licensing.md)
for the full reasoning and alternatives considered.
