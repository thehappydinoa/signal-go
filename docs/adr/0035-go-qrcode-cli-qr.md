# ADR 0035 — `github.com/skip2/go-qrcode` for CLI provisioning QR

- Status: Accepted
- Date: 2026-05-22

## Context

Secondary-device linking prints a `sgnl://linkdevice?...` URL. The Signal
mobile app expects a **QR scan**, not a pasted URL ([getting-started](../guides/getting-started.md)).
ADR 0002 originally deferred QR to “print URL only” or a hand-rolled ANSI
encoder. We now allow a small audited dependency instead.

## Candidates reviewed (2026-05-22)

| Module | License | External transitive deps | Terminal API | Maintenance |
|--------|---------|--------------------------|--------------|-------------|
| `github.com/skip2/go-qrcode` | MIT | **None** (in-repo `bitset`, `reedsolomon`) | `QRCode.ToSmallString()` | Last commit 2020-06; stable encoder |
| `github.com/mdp/qrterminal/v3` | MIT | `rsc.io/qr`, `golang.org/x/term` | Built-in | Active 2025 |
| `github.com/yeqown/go-qrcode/v2` | MIT | `github.com/yeqown/reedsolomon` | Via image/writer | Active 2025 |
| `rsc.io/qr` | BSD-3 (Go Authors) | None | Roll your own | Mature, minimal |

Commands run during review:

```sh
go get <module>@latest && go list -m all   # count transitive modules
go mod why -m <each transitive>
govulncheck ./...                          # after pinning skip2
```

## Decision

Add **`github.com/skip2/go-qrcode`** pinned to
`v0.0.0-20200617195104-da1b6568686e` (only published pseudo-version).

Rationale:

1. **Smallest trust surface** — zero non-stdlib modules beyond the root import.
2. **MIT** — compatible with AGPL-3.0-only project distribution.
3. **Fit** — `ToSmallString()` renders compact half-block ANSI suitable for
   provisioning URLs (~150–300 bytes → QR version 4–7).
4. **Scope** — used only in `internal/qrterminal` for CLI UX; not on the
   wire, not in `pkg/signal` crypto paths.

Rejected for this use case:

- **qrterminal** — fine library, but adds `rsc.io/qr` when skip2 already encodes.
- **Hand-rolled** — ADR 0002 allowed it, but Reed–Solomon + version tables are
  not worth re-auditing for a one-line UX win.

## Integration

- New [`internal/qrterminal`](../../internal/qrterminal/) wraps encode + TTY
  checks (`golang.org/x/term`, already allowlisted).
- `signal-go link` calls it from `OnURL` when stdout is a terminal and
  `-no-qr` is unset; always prints the URL as fallback.
- `NO_COLOR` or non-TTY → URL only (scripts, CI, pipes).

## Security notes

- QR content is the public provisioning URL (ephemeral key material already
  in the query string shown to the user). Encoding bugs could break linking UX
  but do not weaken at-rest or transport crypto.
- Library performs no I/O and no network access.

## Consequences

- **Pro**: Phone linking works without an external QR generator.
- **Pro**: Audit story is one MIT module, no transitive chain.
- **Con**: Encoder unmaintained since 2020; revisit if encoding fails on future
  longer URLs (e.g. more `capabilities`). Mitigate with unit tests on realistic
  `buildLinkURL` payloads.

## Verification

- `go test ./internal/qrterminal/...`
- `govulncheck ./...` clean after pin
- Manual: `task build && ./bin/signal-go link` on a TTY, scan with Signal app
