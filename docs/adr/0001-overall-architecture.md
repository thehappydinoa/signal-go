# ADR 0001 — Overall architecture: cgo to libsignal, Go protocol layer

- Status: Accepted
- Date: 2026-05-20

## Context

We want a Go library that lets a Go program act as a linked Signal device.
There is no Signal-blessed Go SDK. The cryptography for Signal's protocols
(Double Ratchet, X3DH/PQXDH, sealed sender, zkgroup, profile cipher,
attachment cipher, message backups) lives exclusively in Signal's official
Rust library [`libsignal`](https://github.com/signalapp/libsignal). Each
official client (Signal-Desktop, Signal-iOS, Signal-Android, signal-cli)
writes its own non-crypto plumbing — websocket framing, REST, prekey
lifecycle, store interface — on top of libsignal.

Options considered:

1. **Pure-Go reimplementation of the Signal protocol.** Multi-year project,
   would require independent security review of every primitive, and would
   diverge from upstream on every change.
2. **Wrap `signal-cli` as a subprocess.** Pulls in the JVM, ties us to
   `signal-cli`'s release cadence, and the IPC layer (JSON-RPC) becomes a
   load-bearing contract we don't control.
3. **CGO to `libsignal_ffi.a` + Go protocol layer.** All cryptography goes
   through Signal's audited Rust code. The Go layer handles only wire
   format, REST, and state.

## Decision

Adopt option (3). The project is layered as:

```
pkg/signal              public API
internal/provisioning   QR linking flow
internal/web            REST client
internal/ws             websocket framing
internal/store          persistence interface
internal/proto/gen      generated protobufs
internal/libsignal      cgo wrapper around libsignal_ffi
```

`internal/libsignal` is the only package that uses cgo. Higher layers see
only Go types.

## Consequences

- **Pro**: All cryptography is Signal's own code. Upgrading libsignal is a
  matter of bumping a pinned tag and rebuilding.
- **Pro**: Clear seam between trusted (Rust) and our Go code, which makes
  security review tractable.
- **Con**: Cross-compilation requires the matching `libsignal_ffi.a` for
  every target platform. We address this in ADR 0004.
- **Con**: Anyone building from source needs Rust once. We provide a single
  `task libsignal` that handles it.
- **Con**: cgo crossings are not free. We keep the FFI surface coarse-grained
  (one call per Signal-level operation, not per primitive).
