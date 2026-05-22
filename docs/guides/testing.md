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

Ring-3 tests use the `e2e` build tag plus `SIGNAL_GO_E2E=1` so a casual
`go test ./...` never hits `chat.signal.org`. The suite in
`pkg/signal/e2e_test.go` covers **open**, **recv**, **send**, and
**group management** (`FetchGroup`, `SyncGroup`, optional `SendGroup`)
against a pre-linked `sqlstore` directory.

Full setup, env vars, and a manual runbook:
[`testing-e2e.md`](./testing-e2e.md).

Quick checklist:

- Linked store: `SIGNAL_E2E_STORE_DIR` (with `signal.db` from
  `sqlstore.OpenWithPassphrase`)
- Passphrase: `SIGNAL_E2E_PASSPHRASE` or `SIGNAL_E2E_PASSPHRASE_FILE`
- 1:1 peer ACI: `SIGNAL_E2E_PEER_ACI` (your phone)
- Recv: send a message from the peer, set `SIGNAL_E2E_RECV_CONTAINS`
- Group: `SIGNAL_E2E_GROUP_MASTER_KEY` (64 hex chars)

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
