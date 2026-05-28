## Summary

- What changed?
- Why was this needed?

## Validation

- [ ] `task test`
- [ ] `task lint`
- [ ] `go vet ./...`
- [ ] Other checks run (describe below)

## Docs and process checklist

- [ ] I updated docs for user-visible changes (see table below)
- [ ] I updated ROADMAP phase/item status if applicable
- [ ] I updated CHANGELOG `[Unreleased]` if applicable
- [ ] I added/updated an ADR when the decision could be second-guessed
- [ ] I did not add runtime dependencies without ADR 0002 allowlist update

<details>
<summary>Which docs? (expand)</summary>

| If you touched… | Also update… |
|---|---|
| `cmd/signal-go/main.go` (new flag) | `docs/guides/getting-started.md`, help text |
| `pkg/signal` public API | doc comment on type/func, `docs/guides/getting-started.md`, `CHANGELOG.md` |
| `internal/store/` or store wire format | `docs/diagrams/encrypted-store.md`, ADR 0012 (bump format version if wire-incompatible) |
| Any `internal/libsignal/` or new `crypto/*` | `docs/security.md` if threat model shifts; add a published test vector if one exists |
| Network protocol / new endpoint | matching diagram under `docs/diagrams/`, ROADMAP phase section |
| TLS trust / CA pinning | ADR 0034, `docs/security/threat-model.md` |
| New Go module dep | ADR 0002 allowlist table — **required** |
| New ADR | row in `docs/adr/README.md`, link from relevant code |
| New package | README "Architecture" section, `docs/diagrams/architecture.md` |

</details>

## Security checklist (if applicable)

- [ ] No secrets are logged
- [ ] Constant-time checks used where required
- [ ] Threat model/docs updated for security-sensitive behavior changes

## Testing notes

Describe scope, edge cases, and anything not covered.
