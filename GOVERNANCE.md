# Governance

signal-go is currently maintainer-led.

## Roles

- Maintainers: review and merge PRs, triage issues, and cut releases.
- Contributors: propose and implement changes through issues and PRs.

## Decision process

- Day-to-day changes are decided in PR review.
- Any decision that future contributors could reasonably second-guess
  must be recorded as an ADR under [docs/adr](./docs/adr/).
- In case of disagreement, maintainers seek rough consensus first;
  unresolved decisions are made by maintainer call and documented.

## Merge policy

- At least one maintainer review is required for non-trivial changes.
- CI must be green for the jobs relevant to changed paths.
- Security-sensitive changes may require extra review and tests.

## Release policy

- Releases are cut through the Actions workflow documented in
  [docs/guides/releasing.md](./docs/guides/releasing.md).
- Maintainers are responsible for changelog quality and release notes.

## Scope and stability

- Until v0.1.0, the project is pre-alpha and APIs may evolve quickly.
- We aim to avoid unnecessary breaking changes, but we prioritize
  correctness and security while the API is still settling.
