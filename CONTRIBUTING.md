# Contributing to signal-go

Thanks for your interest. This project is pre-alpha but welcomes focused
PRs that match the roadmap and conventions below.

## Before you code

1. Read [`CLAUDE.md`](./CLAUDE.md) — style, ADRs, docs, and roadmap rules
   apply to every contributor (human or agent).
2. Read [`docs/guides/getting-started.md`](./docs/guides/getting-started.md)
   for build prerequisites (`libsignal`, cgo, Windows MSYS2 notes).
3. Skim [`ROADMAP.md`](./ROADMAP.md) so your change fits an open phase item.

## Local checks

```sh
task setup      # once: tools + git hooks
task libsignal  # once: libsignal_ffi.a
task test
task lint
```

The [pre-push hook](./.githooks/pre-push) runs vet → lint → test when
`libsignal_ffi.a` is present.

## Releases (maintainers)

See [`docs/guides/releasing.md`](./docs/guides/releasing.md). Summary:

1. Add a `## [x.y.z] - date` section to `CHANGELOG.md`.
2. Merge to `main`.
3. Run **Actions → Create release tag** (workflow `create-release-tag.yml`).
4. When **Release** finishes, publish the draft GitHub Release.

Do not push release tags from a laptop unless you know you are duplicating
what the workflow already does.

## Pull requests

- Rebase onto `main`; keep PRs focused (one concept when possible).
- Update **docs**, **ROADMAP** tick-boxes, and **CHANGELOG** `[Unreleased]`
  when behavior or user-visible flags change.
- File an **ADR** when a future contributor could reasonably second-guess
  the decision ([`docs/adr/`](./docs/adr/)).
- Do not add runtime Go dependencies without updating
  [ADR 0002](./docs/adr/0002-no-third-party-go-deps.md).

## Security

Report vulnerabilities per [`SECURITY.md`](./SECURITY.md). Do not open public
issues for exploit details.

## License

By contributing, you agree your work is licensed under the project's
[AGPL-3.0-only](./LICENSE) terms, consistent with `libsignal`.
