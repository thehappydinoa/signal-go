# ADR 0041 — Raise the Go toolchain floor to 1.26

- Status: Accepted
- Date: 2026-09-14

## Context

`go.mod` declared `go 1.25.0`. Two pressures pushed against that floor.

**1. Security.** `main` was already on `go1.25.11`, itself taken to clear
`govulncheck` findings. Five advisories still applied at that patch level,
three of them on the TLS path that [ADR 0034](./0034-signal-tls-root-pinning.md)
pins:

| ID | Package | Fixed in | Open at go1.25.11? |
|---|---|---|---|
| GO-2026-6090 | `crypto/tls` | go1.25.13 | yes |
| GO-2026-5856 | `crypto/tls` | go1.25.12 | yes |
| GO-2026-6218 | `net/url` | go1.25.13 | yes |
| GO-2026-5972 | `encoding/asn1` | go1.25.13 | yes |
| GO-2026-5026 | `net/http` | go1.25.13 | yes |
| GO-2026-5037 | `crypto/x509` | go1.25.11 | already cleared |
| GO-2026-5039 | `net/textproto` | go1.25.11 | already cleared |

A `go1.25.13` patch would have cleared the remaining five without touching the
floor. That is worth stating plainly: **security alone did not require this
ADR.** The floor moves for the second reason.

**2. Dependencies.** The whole `golang.org/x` family has moved to
`go >= 1.26`: `x/crypto`, `x/sys`, `x/term`, and — most relevant —
`x/crypto/x509roots/fallback`. On a 1.25 floor, `go get -u ./...` refuses
every one of them, leaving the `x509roots/fallback` CA bundle frozen at its
2026-05-22 snapshot. That bundle is a security input, not a convenience: it
is the trust anchor set backing our TLS verification. Staying on 1.25 meant
accepting an indefinitely-ageing root store.

`modernc.org/sqlite` ([ADR 0029](./0029-sqlite-store.md)) is on the same
trajectory: 1.53.0 was the newest release installable on a 1.25 floor.

## Decision

Set `go 1.26.0` in `go.mod`, with `toolchain go1.26.8`, and move every CI
workflow (`ci.yml`, `codeql.yml`, `fuzz-nightly.yml`, `release.yml`) to
`GO_VERSION: '1.26'`.

This raises the **minimum Go version for consumers of `pkg/signal`**. Per the
CLAUDE.md "When to ask first" list this affects backward compatibility after
the first tagged release (v0.1.0); it was explicitly agreed before landing.
Pre-1.0 tags may break compatibility without a major bump, and a toolchain
floor is the mildest form of that.

We continue **not** to matrix multiple Go versions
([ADR 0013](./0013-ci-github-actions.md)): `go.mod` is the single source of
truth and CI tracks it.

### Linter pin

`golangci-lint` refuses to lint a module whose targeted Go version exceeds the
Go version that built the linter binary, and the GitHub action installs a
*prebuilt* release rather than building from source. The previous pin,
v2.12.2, ships built with `go1.26.2` — below our `toolchain go1.26.8` — and is
rejected. The pin therefore moves to **v2.13.2**, which ships built with
`go1.27.0`.

This coupling is easy to miss because it does not reproduce locally: a
`go install` of the same version rebuilds it with the local toolchain and
works fine. Check the *release binary*, not a locally built one:

```sh
go version "$(command -v golangci-lint)"
```

## Consequences

- **Pro**: `govulncheck ./...` reports 0 affected; module-level advisories
  dropped from 4 to 1 (uncalled).
- **Pro**: `x509roots/fallback` is current again, so the pinned-root TLS path
  is no longer verifying against a stale bundle.
- **Pro**: Unblocks `x/crypto`, `x/sys`, `x/term`, and `modernc.org/sqlite`
  1.58.0.
- **Con**: Anyone importing `pkg/signal` must now be on Go 1.26+. This is the
  first time we have raised the floor after a tagged release; it belongs in
  the release notes for the next tag, not just the changelog.
- **Con**: The toolchain floor and the `golangci-lint` pin are now coupled.
  Raising `toolchain` can break CI lint without any local symptom. The comment
  in `ci.yml` records why.

## Alternatives considered

- **Stay on 1.25 and take only the 1.25.13 patch.** Clears the reachable CVEs
  but freezes the CA bundle and the whole `x/` tree indefinitely. Rejected:
  it trades a one-time consumer cost for permanent drift on a security input.
- **Drop the `toolchain` directive** so the linter targets `1.26.0` and the
  older pin keeps working. Rejected: the `toolchain` pin is what makes local
  builds reproducible; bending it to satisfy a linter is backwards.
