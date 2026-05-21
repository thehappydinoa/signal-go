# ADR 0013 — GitHub Actions CI

- Status: Accepted
- Date: 2026-05-20

## Context

We have a Taskfile that runs lint / vet / test / build locally and a
[Phase 8 checklist](../../ROADMAP.md#phase-8--security-audit-planned-required-before-v010)
that lists the tooling we want passing before `v0.1.0`. None of it
runs automatically on PRs yet — every contributor is on the honour
system. We need automation before the project grows past trivially-
reviewable size.

GitHub Actions is the obvious vehicle: free for public repos, native
PR-status integration, no other infrastructure to operate.

## Decision

Three workflows under `.github/workflows/`:

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` | push to `main`, PRs to `main` | build libsignal (cached), lint, vet, test, govulncheck |
| `codeql.yml` | **weekly schedule only** + `workflow_dispatch` | GitHub's CodeQL security scanning for Go |
| `dependabot.yml` (config) | weekly | bump `go.mod` modules + action versions |

**Why CodeQL is schedule-only**: running it on every push/PR alongside
`ci.yml` meant two parallel jobs both building `libsignal_ffi.a` (the
cache can't deduplicate when both miss simultaneously). That doubled
our Actions quota for marginal per-PR signal. CodeQL is a periodic
security scan, not a per-PR gate. Weekly runs cache-hit from any
recent PR run (caches live 7 days) and stay fast.

### libsignal caching strategy

The slow path: `task libsignal` clones `signalapp/libsignal` at a
pinned tag and runs `cargo build --release -p libsignal-ffi` (~5–10
minutes on a fresh runner, ~188 MB output).

The CI strategy:

1. A dedicated `libsignal` job extracts the pinned version from
   `scripts/build-libsignal.sh`, restores `internal/libsignal/lib/` from
   `actions/cache@v4` keyed on `(version, os, arch)`.
2. On cache miss, the job installs the Rust toolchain, runs the script,
   and the cache action saves the resulting `.a`.
3. Downstream jobs (`lint`, `vet`, `test`) consume the `.a` via
   `actions/upload-artifact` + `actions/download-artifact`. The
   artifact is scoped to the workflow run, so it's discarded after.

Net effect: first PR against a new libsignal version pays the build
cost once across all jobs; every subsequent run is cache-hit fast (~30s
per Go job).

### Matrix policy

CI matrix: `ubuntu-latest` only at first. macOS + Windows are tracked
as follow-ups; they need their own libsignal builds (different `.a`
ABIs / paths) and that's a complication best handled separately.

Go version pinned to `1.25` (the toolchain in `go.mod`). We do not
matrix multiple Go versions; if `go.mod` says `1.25`, that's what we
support. Bumping is a deliberate PR.

### Failure policy

- `lint`, `vet`, `test`, `build`: fail the PR.
- `govulncheck`: fail the PR. Vulnerabilities in transitive deps are
  blockers; triage in the same PR or pin a known-good version.
- `codeql`: report results; failure mode is "alert in Security tab",
  not "block PR" — too noisy for early-stage code.

### Secrets and trust

CI workflows run with `permissions: read-all` by default. CodeQL adds
`security-events: write` for itself. No PRs from forks get write
permissions; that's standard GitHub Actions behavior and we don't
override it.

## Consequences

- **Pro**: Contributors get immediate feedback. Reviewers can trust the
  status badge instead of running everything locally.
- **Pro**: The same Taskfile runs locally and in CI, so "works on my
  machine" stays honest.
- **Con**: First PR after a libsignal version bump is slow. Mitigation:
  intentional — version bumps already require rebuilding the FFI
  header locally and committing it, so the human's already paid the
  cost.
- **Con**: Cache eviction. GitHub purges caches not hit in 7 days; a
  long-quiet repo will rebuild on first wake-up. Acceptable.

## Future

- Cross-platform matrix once we have a contributor on macOS / Windows
  who can validate the libsignal build path on those hosts.
- Release workflow that builds `signal-go` binaries on tag push and
  attaches them to the GitHub Release. Lands with `v0.1.0`.
- gosec / staticcheck as separate jobs, after we triage the current
  findings (some are likely false-positives around cgo).
- Fuzz job on a nightly schedule (Phase 8).
