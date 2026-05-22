# ADR 0006 — Testing strategy

- Status: Accepted
- Date: 2026-05-20

## Context

A Signal client touches network, crypto, persistence, and async event flows.
We want to catch regressions without requiring a real phone in the loop for
every commit.

## Decision

Three concentric test rings:

### 1. Unit tests (`go test ./...`, runs always)

- Table-driven by default ([Go style guide pattern](https://google.github.io/styleguide/go/decisions#table-driven-tests)).
- One `_test.go` file per implementation file, same package, so we can test
  unexported helpers without resorting to `internal/testutil` games.
- Exported public-API tests live in `pkg/signal/*_test.go` and use the
  `signal_test` external test package so they exercise only the public
  surface.
- No network. No filesystem outside `t.TempDir()`. No real time —
  use `clock` injection where time matters.
- Run on every CI commit. Fast (target: <10s total).

### 2. Component tests (build tag `component`, runs in CI)

- Test a slice of the system with a fake Signal server: an in-process HTTP +
  websocket server that speaks the real wire protocol, fed scripted
  responses.
- Cover the provisioning flow, send pipeline, receive pipeline against the
  fake.
- These verify that our protobufs, ws framing, REST plumbing, and cgo
  bindings hang together.
- Require `libsignal_ffi.a` (i.e. `task libsignal` has run).
- Run with `go test -tags=component ./...`.

### 3. End-to-end tests (build tag `e2e`, manual)

- Talk to real `chat.signal.org` with a throwaway linked-device session.
- Gated behind `SIGNAL_GO_E2E=1` env var so they never run unintentionally.
- Document the manual run in [`docs/guides/testing-e2e.md`](../guides/testing-e2e.md).

### Crypto / FFI specifics

- Test cgo wrappers against libsignal's own test vectors where they exist.
  For Double Ratchet, X3DH, PQXDH, and sealed sender, libsignal ships test
  vectors we can re-use (sourced when needed; not vendored upfront).
- Every cgo function gets a unit test that exercises happy path + at least
  one error path.

### Coverage

- Target ≥80% line coverage for non-cgo packages.
- cgo packages: coverage tooling is unreliable across the boundary; we
  instead require every exposed function to have a dedicated test.
- `task cover` prints a per-package summary; CI fails if the project total
  drops below the threshold.

### Property-based tests

- Use `testing/quick` from the stdlib for proto round-trips, ws framing,
  and address parsing. No third-party fuzzers as a dep — stdlib `go test
  -fuzz` for fuzz targets.

## Consequences

- Three rings means three places to add a test for a new feature, but each
  ring has a clear job. Most contributions only touch ring 1.
- The fake Signal server in ring 2 is itself a non-trivial chunk of code
  (~1k LOC eventually). Worth it for fast, deterministic feedback.
