# Changelog

All notable changes to **signal-go** will land here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once
we cut `v0.1.0` (until then everything is pre-1.0 and the `main`
branch is the only reference point).

A separate ADR — [`docs/adr/README.md`](./docs/adr/README.md) — tracks
*decisions* (why); this file tracks *changes* (what + when).

## [Unreleased]

### Added

- Cross-platform release pipeline ([ADR 0033](./docs/adr/0033-release-pipeline.md)).
  `.github/workflows/release.yml` builds `signal-go` natively on five
  runners (linux amd64/arm64, darwin amd64/arm64, windows amd64) on
  every `v*` tag push, packages `.tar.gz`/`.zip` + `.sha256`, and
  uploads to a draft GitHub Release. `workflow_dispatch` provides a
  no-publish dry-run path. Windows is `experimental: true` until the
  first clean release.
- `scripts/build-libsignal.sh` now detects Darwin and MSYS/MINGW hosts,
  honours an optional `CARGO_TARGET` override (for Windows MinGW-gnu
  cross-builds), and embeds `<version>-<os>-<arch>` in the on-disk
  stamp.
- `internal/libsignal/cgo.go` gains a `#cgo windows LDFLAGS` line
  covering the Win32 surface area libsignal's transitive deps reach for.
- `signal-go version` / `signal-go --version` prints the build-tagged
  version, Go toolchain, and `vcs.{revision,time,modified}` from the
  embedded `debug.ReadBuildInfo` block.

### Changed

- Real e-mail contact published for security reports:
  <signal-go-security@thehappydinoa.dev>. PGP exchange on request.
  See [`SECURITY.md`](./SECURITY.md).
- ROADMAP Phase B (cross-platform CI runners) and Phase C (release
  pipeline) ticked. Code signing + Homebrew/Scoop tracked as
  post-v0.1.0 follow-ups.

[Unreleased]: https://github.com/thehappydinoa/signal-go/compare/main...HEAD
