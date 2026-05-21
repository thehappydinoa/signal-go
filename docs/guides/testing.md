# Testing

`signal-go`'s test strategy is three rings: unit (always), component
(CI), and end-to-end (manual / gated). See [ADR 0006](../adr/0006-testing-strategy.md)
for the full rationale.

## Running tests

```sh
task test               # unit tests across all packages, with -race
task test:component     # component tests (in-process fake Signal server)
task cover              # tests + per-package coverage summary
task lint               # golangci-lint
```

Don't have `task`? The equivalent is:

```sh
go test -race -count=1 ./...
go test -race -count=1 -tags=component ./...
golangci-lint run
```

## What's tested today

- **Unit**: cgo wrappers (Curve25519, ML-KEM, XEdDSA sign/verify,
  ECDH, HKDF — including RFC 5869 vectors), provisioning cipher
  round-trip + tamper / wrong-recipient rejection, prekey generation
  + signature verification, REST client (URL building, basic-auth,
  JSON, error mapping), websocket WebSocketMessage routing,
  fsstore — plaintext + AES-256-GCM + Argon2id + mode-mixing safety,
  store sub-interfaces (memstore conformance + the cgo-free callback
  shells in `internal/libsignal/stores_impl.go`)
- **Component**: the public `signal.Link` driven against a fake server
  that speaks both the provisioning ws and the REST `/v1/devices/link`
  + `/v2/keys` endpoints (`pkg/signal/link_test.go`)

## End-to-end testing against real Signal

```sh
SIGNAL_GO_E2E=1 task test:e2e
```

This is gated behind an environment variable so a casual `go test`
never tries to pair against a real Signal account. The current e2e
target is "perform a real link" — the receive/send tests land with
Phases 3 / 4.

You'll need:
- A spare phone or VoIP number with a Signal account (you'll be the
  "primary device")
- Network egress to `chat.signal.org`
- 30 seconds of your attention to scan the QR

The harness prints the QR-link URL and waits.

## Adding tests for a new package

We default to table-driven tests in the same package
(`package foo` not `package foo_test`) so unexported helpers can be
covered directly. External-test packages (`foo_test`) are reserved for
exercises that should only touch the public surface (e.g. `pkg/signal`).

For cgo-touching code, mind the wrinkle that `_test.go` files can't
`import "C"` in a package that already uses cgo elsewhere. Put cgo-only
test helpers in a non-`_test.go` file gated by `//go:build !signalgo_no_test_helpers`
or, simpler, refactor so the business logic is cgo-free and the cgo
glue is thin. See `internal/libsignal/stores.go` ↔ `stores_impl.go` for
the pattern.

## Fuzzing

Targets land with the Phase 8 security work. The current list of
intended fuzz surfaces:

- `provisioning.DecryptEnvelope` (corpus seeded from real envelopes)
- `ws.WebSocketMessage` proto round-trip
- Anything that parses bytes coming from the network or from disk
