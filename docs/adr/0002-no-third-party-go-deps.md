# ADR 0002 — No third-party Go runtime dependencies

- Status: Accepted
- Date: 2026-05-20

## Context

The user requirement is to "avoid untrusted code" while trusting Signal. The
crypto question is settled by ADR 0001: everything goes through libsignal.
The remaining question is the Go layer.

The natural Go references — `mautrix/signal`'s `pkg/signalmeow` and
`pkg/libsignalgo` — are mature, well-tested, and AGPL-3.0. Vendoring them
would land us at a working client in days. We are choosing not to.

## Decision

`signal-go` keeps direct runtime dependencies to a small, audited allowlist.
Everything else is stdlib. Current allowlist:

| Module | Why it's permitted |
|---|---|
| `google.golang.org/protobuf` | Required by protoc-gen-go output; considered ecosystem-core. |
| `github.com/coder/websocket` | RFC 6455 client. **Zero transitive deps**, single maintainer, context-aware API, used in production by Cloudflare/Tailscale. We initially hand-rolled this (~250 LOC + tests, all green) but the maintenance burden of edge cases (close codes, fragmentation under load, permessage-deflate negotiation) isn't worth the small audit gain. |
| `golang.org/x/crypto` | Go team's staging area for cryptographic primitives. No transitive non-stdlib deps. We use `argon2` for passphrase-based key derivation (ADR 0012). Effectively part of the standard library; same maintainers as `crypto/*`. |

- HTTP: `net/http`.
- QR code rendering for the CLI demo: we'll print the `sgnl://` URL only
  and let the user paste it; if a real QR is desired we'll print an ANSI QR
  via a tiny hand-rolled encoder (no dep).

`signalmeow` and `signal-cli` remain reference implementations we read while
writing. We never compile their code into our binary.

Dev/test-only dependencies (testify, etc.) are permitted but discouraged.
Prefer the standard library `testing` package.

## Consequences

- **Pro**: Audit surface is exactly the code in this repo plus libsignal.
- **Pro**: No transitive license obligations (especially AGPL).
- **Pro**: Reproducible, hermetic builds.
- **Con**: We write more code (estimated 6–10k LOC for MVP).
- **Con**: We may rediscover bugs that downstream libs already fixed.
  Mitigate by leaning on libsignal for anything cryptographic and by
  testing extensively (ADR 0006).

## Exception process

Adding a runtime dep requires:
1. Listing it in the allowlist table above with a one-sentence justification.
2. Updating this ADR (small additions) or superseding it (large policy shift).
3. Verifying transitive deps are also acceptable (`go mod why -m <dep>` and
   `go list -m all`).
