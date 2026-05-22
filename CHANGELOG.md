# Changelog

All notable changes to **signal-go** will land here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); this project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) once
we cut `v0.1.0` (until then everything is pre-1.0 and the `main`
branch is the only reference point).

A separate ADR — [`docs/adr/README.md`](./docs/adr/README.md) — tracks
*decisions* (why); this file tracks *changes* (what + when).

## [Unreleased]

### Fixed (release dry-run round 2)

- macOS legs of `release.yml` failed Go compilation with
  `cannot use cServiceID(...) (value of type *[17]_Ctype_uint8_t) as
  *_Ctype_SignalServiceIdFixedWidthBinaryBytes value`. Root cause:
  cgo's representation of `const SignalServiceIdFixedWidthBinaryBytes *`
  parameters differs between GCC and clang DWARF. GCC unwraps the
  array typedef; clang keeps the typedef name. Split the affected
  helpers (`cServiceID`, `cServiceIDPtr`) into per-toolchain files —
  `service_id_cgo_typedef_default.go` (`!darwin`, GCC-style) and
  `service_id_cgo_typedef_darwin.go` (clang-style). Call sites are
  now portable.
- Windows leg of `release.yml` failed libsignal_ffi.a build with
  `Could not find protoc` from the spqr crate's prost-build invocation.
  Even though MSYS2's protoc was on PATH via `GITHUB_PATH`, the cargo
  child-process lookup missed it. Set `PROTOC` env var explicitly on
  the Windows leg (empty elsewhere so PATH-based lookup still works).

### Fixed

- `release.yml` macOS legs now use a portable `sed` extractor for the
  pinned libsignal version. The original `grep -oP` worked on Linux
  (GNU grep) but exited the macOS jobs with "invalid option -- P" on
  BSD grep. Same fix mirrored into `ci.yml`, `fuzz-nightly.yml`, and
  `codeql.yml` for consistency.
- `scripts/build-libsignal.sh` now runs `rustup target add` from
  *inside* the libsignal clone so the target's standard library lands
  on the toolchain libsignal pins via its `rust-toolchain` file
  (nightly-2026-03-23), not on the system default (stable). This was
  the Windows release failure mode — `error[E0463]: can't find crate
  for core` while compiling `cfg-if` against the gnu target.

### Changed

- `actions/setup-go` bumped from `@v5` to `@v6` across every workflow.
  v6 was released 2025-09-04 with breaking changes around toolchain
  handling and a Node.js 24 runtime; GitHub-hosted runners are on
  ≥ v2.327.1 (already required).

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
