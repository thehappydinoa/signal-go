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

## Validating wire-protocol changes

Some behaviour — device linking, prekey upload, the chat websocket — is
defined by Signal's server, which we can't unit-test against. A wrong
assumption about the wire format passes every local test and only fails in
production. We validate these changes in layers, cheapest first:

0. **Spec from the reference, by triangulation.** Treat signal-cli /
   libsignal-service-java as the protocol source of truth, and cross-check
   against [Signal-Server](https://github.com/signalapp/Signal-Server) when
   the two might disagree. One source can be misread; agreement between the
   client and the server is the bar. (Example: linking is REST
   `PUT /v1/devices/link`, Basic-authed with the **e164 number** as the
   username, provisioning code in the body as `verificationCode`, with
   `attachmentBackfill` + `spqr` capabilities — confirmed in both repos.)
1. **Static**: `gofmt`, `go build`, `go vet`. Note cgo packages link only
   once `libsignal_ffi.a` is built (`task libsignal` / CI), so a pure-Go
   `go build ./internal/web` is the most you can compile without it.
2. **Contract unit tests**: assert the exact bytes we emit — method, path,
   the *decoded* Authorization username, capability keys, body fields — not
   just "a request was made". The link-auth bug shipped because the test
   only checked for a `Basic ` prefix; `link_test.go` now decodes the header
   and asserts the e164. A fake only catches a wrong claim if you assert the
   claim.
3. **CI**: builds libsignal and runs the `-race` suite + govulncheck — the
   first place cgo packages compile and the full suite runs.
4. **Live e2e** (gated): the only layer that proves the server *accepts* the
   request. A change to a server-facing request is not "done" until one live
   run passes — see [testing-e2e.md](./testing-e2e.md) for the runbook and
   the status-code → cause map (403 = token, 422 = capabilities/prekeys,
   499 = stale User-Agent).

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
