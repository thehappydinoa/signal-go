# Contributing conventions (and instructions for Claude)

This file documents the working norms of the `signal-go` repository.
It's named `CLAUDE.md` because the [Claude Code](https://claude.ai/code)
agent reads files of this name automatically, but everything in it is
addressed at *any* contributor — agent or human. Following it keeps the
project consistent and reviewable.

## Three rules that override everything else

1. **Always file an ADR when making a decision that future contributors
   could reasonably second-guess.** Architecture, dependency choices,
   storage format, security trade-offs, license — all already have
   ADRs under [`docs/adr/`](./docs/adr/). Use the next free number;
   follow the existing format (Status, Date, Context, Decision,
   Consequences). An ADR is *cheap*; a regretted decision argued over
   in code review six months later is expensive.

2. **Always update the docs when changes invalidate them.** When you
   add a feature, change a flag, ship a new package, or alter a wire
   format, the corresponding section under
   [`docs/`](./docs/) (and possibly the README) is now wrong. Fix it in
   the same PR. No "I'll do the docs later" — that PR doesn't exist.

3. **Always update the [ROADMAP](./ROADMAP.md) when you finish, start,
   or re-scope a phase item.** The roadmap is the project's status
   page; it must reflect reality. Phase tick-boxes get checked at the
   end of the PR that does the work.

If you ever feel any of these three "doesn't apply here", that's a
strong signal you're about to make the project messier. Push through
the friction and do it anyway.

## What goes where

| Change | Updates |
|---|---|
| Add a package | README "Architecture" diagram, [`docs/diagrams/architecture.md`](./docs/diagrams/architecture.md), maybe a new ADR |
| Add a Go module dep | [ADR 0002](./docs/adr/0002-no-third-party-go-deps.md) allowlist table — *required*. PRs adding deps without updating this ADR get rejected. |
| Change a public API in `pkg/signal` | doc comment on the type/function, [`docs/guides/getting-started.md`](./docs/guides/getting-started.md) if user-visible, [`CHANGELOG.md`](./CHANGELOG.md) |
| Cut a release tag | [`CHANGELOG.md`](./CHANGELOG.md), [`docs/guides/releasing.md`](./docs/guides/releasing.md), then **Create release tag** workflow (not a manual `git tag` unless emergency) |
| Add a new CLI flag | [`cmd/signal-go/main.go`](./cmd/signal-go/main.go) help, [`docs/guides/getting-started.md`](./docs/guides/getting-started.md) example |
| Touch the on-disk store format | [ADR 0012](./docs/adr/0012-encrypted-store.md) (bump format version byte if wire-incompatible), [`docs/diagrams/encrypted-store.md`](./docs/diagrams/encrypted-store.md) |
| Touch a network protocol | matching diagram under [`docs/diagrams/`](./docs/diagrams/), the relevant Phase section in [ROADMAP](./ROADMAP.md) |
| Touch TLS trust for Signal hosts | [ADR 0034](./docs/adr/0034-signal-tls-root-pinning.md), [`docs/security/threat-model.md`](./docs/security/threat-model.md) |
| Add cryptography (anything new that uses `crypto/*` or `internal/libsignal`) | new test using a published vector if one exists, [`docs/security.md`](./docs/security.md) if the threat model shifts, Phase 8 audit checklist |
| Add a new ADR | add a row to [`docs/adr/README.md`](./docs/adr/README.md), link from any relevant code or docs |

## Code style

### Go

- `gofmt -s -w` clean. `go vet ./...` clean. `golangci-lint run` clean
  using the pinned [`.golangci.yml`](./.golangci.yml).
- One concept per package. Packages under `internal/` may grow many
  files; public packages under `pkg/` stay narrow.
- Errors get wrapped with `fmt.Errorf("scope: %w", err)`, where `scope`
  is the package or function name. No naked `return err` at exported
  function boundaries.
- Public functions get a doc comment that starts with the function name.
- Cgo goes through `internal/libsignal` only. Never in `pkg/*`. Never in
  tests (the Go toolchain refuses cgo in `*_test.go` of cgo-using
  packages; if you need to drive a callback in a test, factor the body
  into a Go-typed `*Impl` and test that — see
  [`internal/libsignal/stores.go`](./internal/libsignal/stores.go) ↔
  [`stores_impl.go`](./internal/libsignal/stores_impl.go) for the pattern).

### Tests

- Table-driven by default. One `_test.go` per implementation file, same
  package (so unexported helpers can be tested).
- External-test packages (`foo_test`) are for public-API contract tests
  only.
- Use `t.TempDir()` for filesystem-touching tests, not hard-coded paths
  in `/tmp`.
- For network code: an in-process fake on `httptest.NewServer`. No
  real `chat.signal.org` traffic in unit tests; reserve that for the
  `e2e` build tag.

See [`docs/guides/testing.md`](./docs/guides/testing.md) for the
three-ring testing strategy.

## Git workflow

- Branches: short-lived, named `claude/signal-go-<topic>` or
  `<author>/<topic>`. Rebased onto `main` before opening a PR.
- One concept per PR. If a PR's description has more than three sections
  it's probably two PRs.
- Commit messages: imperative mood, first line ≤ 72 chars, blank line,
  body in prose with bullet points for the changelog-style summary. The
  body should answer "why" more than "what" — the diff already shows
  the what.
- Don't merge your own PR unless that's the agreed workflow. Ask for
  review.

## Security-sensitive changes

- Never log: account passwords, private keys, profile keys, prekey
  secret material, `AccountEntropyPool`. `slog` is fine for everything
  else, but scrub these fields explicitly.
- Constant-time comparisons (`hmac.Equal`, `subtle.ConstantTimeCompare`)
  for any MAC / tag / passphrase check.
- File writes that involve secret material use atomic
  temp-file + rename + 0600. `internal/store/fsstore/encryption.go`
  is the reference.
- Touch crypto code → update [`docs/security.md`](./docs/security.md)
  if the threat model shifts, and add to the
  [Phase 8 audit checklist](./ROADMAP.md#phase-8--security-audit-internal-pass-done-external-pass-required-before-v010).

## Working with libsignal upstream

- We pin to a tag in [`scripts/build-libsignal.sh`](./scripts/build-libsignal.sh).
- Bumping the pin = checking the diff of the cbindgen-generated
  `internal/libsignal/include/signal_ffi.h` and reviewing every
  signature change against our cgo wrappers.
- libsignal's README says "use outside of Signal is unsupported".
  Treat their API as one that can break between releases; that's why
  we have the pin and the audit gate.

## When to ask first

- Anything that adds a runtime dependency.
- Anything that changes the on-disk store wire format (bump version).
- Anything that introduces a new network endpoint, capability flag,
  or persisted credential.
- Anything that loosens a security check (constant-time, validation
  order, error-path closure).
- Anything that affects backward compatibility of `pkg/signal` after
  the first tagged release.

## CI

GitHub Actions workflows under [`.github/workflows/`](./.github/workflows/)
([ADR 0013](./docs/adr/0013-ci-github-actions.md)):

- **`ci.yml`** runs on every push to `main` and every PR:
  - `libsignal` (one-shot, cached): builds `libsignal_ffi.a` from the
    pinned tag and uploads it as a workflow artifact
  - `lint` — golangci-lint against the committed `.golangci.yml`
  - `vet` — `go vet ./...`
  - `test` — `go test -race -count=1 ./...`
  - `build` — `go build ./...` + `bin/signal-go` + smoke-test `--help`
  - `govulncheck` — vulnerability scan against the dep tree
- **`codeql.yml`** runs CodeQL's `security-extended` query set on push,
  PR, and weekly schedule. Findings land in the *Security* tab; PRs
  aren't blocked on it.
- **`dependabot.yml`** — weekly bumps for Go modules + Actions
  versions. New direct deps still need an ADR 0002 allowlist update.

Locally these correspond to:
- `task lint` (golangci-lint)
- `task test` (`go test -race -count=1 ./...`)
- `task test:component`
- `go vet ./...`, `govulncheck ./...`

Future additions (tracked in the [Roadmap "Continuous integration & quality"](./ROADMAP.md#continuous-integration--quality-ongoing) section):
- macOS + Windows matrix
- `staticcheck` + (post-triage) `gosec`
- Coverage report on PR checks
- Release workflow on tag push
- Nightly fuzz job

### Pre-push hook

A git pre-push hook that mirrors the CI checks (vet → lint → test) lives in
[`.githooks/pre-push`](./.githooks/pre-push). Activate it once per clone:

```sh
task hooks:install      # or: git config core.hooksPath .githooks
```

`task setup` runs `hooks:install` automatically, so if you ran setup when you
first cloned you're already covered.

The hook requires `libsignal_ffi.a` to be built. If it is not present yet it
prints a warning and exits 0 (non-blocking) so a fresh clone is never
dead-locked. Build the library with `task libsignal` and subsequent pushes
will enforce the full check suite locally.

If your change touches anything that CI builds or tests, run it locally
first (`task test && task lint`) before pushing. The hook does this
automatically; CI is the final safety net, not the first line.

## License

This project is **AGPL-3.0-only** (driven by libsignal; see
[ADR 0009](./docs/adr/0009-licensing.md)). Don't add code under
incompatible licenses; copy-paste from MIT/BSD projects is fine, but
ensure the attribution is preserved.

---

If something in this file feels wrong or out of date, that's the
strongest signal to fix it. Open a PR amending CLAUDE.md alongside the
behavior change.
