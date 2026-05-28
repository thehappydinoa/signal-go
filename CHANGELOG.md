# Changelog

All notable changes to **signal-go** are documented here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) from `v0.1.0`
onward. Pre-1.0 tags may break API without a major bump.

Architectural *why* lives in [`docs/adr/`](./docs/adr/README.md); this file
is *what* changed and *when*.

## [Unreleased]

Nothing yet.

## [0.2.0] - 2026-05-27

### Added

- Pre-built `libsignal_ffi.a` artifacts published under dedicated
  `libsignal-v*` GitHub Releases; `task libsignal` now downloads the
  correct platform artifact automatically — **no Rust or cargo required**
  for tagged releases ([ADR 0037](./docs/adr/0037-libsignal-prebuilt-artifacts.md)).
- `task libsignal:download` — download-only task; fails fast when no
  pre-built artifact exists rather than falling back to cargo.
- `go generate ./internal/libsignal/` bootstrap via
  `tools/libsignal_setup.go` (pure Go, no extra tools) for library
  consumers who do not have `task` installed.
- `scripts/download-libsignal.sh` — portable download helper with SHA256
  verification; called by `build-libsignal.sh` as a fast path.
- `.github/workflows/libsignal-artifacts.yml` — matrix workflow that
  builds and publishes pre-built `.a` + `.sha256` files on every libsignal
  version bump (triggers on changes to `scripts/build-libsignal.sh`).
- Three bot examples: `examples/middleware-bot` (middleware composition:
  logging, recovery, rate limiting), `examples/poll-bot` (group poll
  workflows using reactions), `examples/wizard-bot` (multi-stage signup
  conversation via `bot.Wizard`).
- Rate-limit retry middleware and local prekey-fetch rate limiting in
  `pkg/signal` client (avoids thundering-herd on `PUT /v2/keys`).
- CodeQL autobuild step for improved Go project instrumentation.

### Fixed

- Integer overflow in allocation size computation in
  `internal/libsignal` (CodeQL alert #1).
- Device name cipher KDF now matches Signal Android's `DeviceNameCipher`
  — correct synthetic IV derivation and key schedule
  ([ADR 0036](./docs/adr/0036-linked-device-name-cipher.md)).

### Changed

- CI workflow ignores markdown-only changes in pushes and PRs, avoiding
  unnecessary `libsignal_ffi.a` rebuilds on doc-only commits.

## [0.1.0] - 2026-05-27

### Added

- `internal/profile` and `echo-bot run -memprofile` / `-cpuprofile` for long-running
  heap/CPU soaks; guide: [`docs/guides/profiling.md`](./docs/guides/profiling.md)
  (Phase 8 bake results recorded 2026-05-27).
- `signal.Client.SendEdit` for 1:1 outbound edits (`Content.editMessage`).
- Encrypted linked-device display name at `PUT /v1/devices/link` (Android-compatible
  cipher; [ADR 0036](./docs/adr/0036-linked-device-name-cipher.md)).
- Optional `OnChatItem` on link-and-sync / `ImportTransferArchive` to stream
  transfer-archive `ChatItem` frames as protobuf bytes ([ADR 0031](./docs/adr/0031-transfer-archive-frame-import.md)).
- E2e test suite (`go test -tags=e2e`, `task test:e2e`): open, recv, send,
  and group management (`FetchGroup`, `SyncGroup`, optional `SendGroup`) against
  a linked `sqlstore` directory. Guide: [`docs/guides/testing-e2e.md`](./docs/guides/testing-e2e.md).
- Terminal QR for `signal-go link` via audited `github.com/skip2/go-qrcode`
  ([ADR 0035](./docs/adr/0035-go-qrcode-cli-qr.md)); `-no-qr` and `NO_COLOR`
  skip rendering.
- **Trigger Release for tag** workflow to start **Release** for an existing `v*`
  tag (recovery).

### Fixed

- Device linking now authenticates `PUT /v1/devices/link` with the account's
  e164 number as the HTTP Basic username (the provisioning code travels only in
  `verificationCode`), matching signal-cli / libsignal-service-java. Previously
  the provisioning code was sent as the username, which the server rejects.
- Linked-device capabilities now match signal-cli
  (`storage`, `versionedExpirationTimer`, `attachmentBackfill`, `spqr`). The
  previous set omitted `attachmentBackfill` and `spqr`, both of which
  Signal-Server requires for new devices, causing `PUT /v1/devices/link` to
  fail with HTTP 422 "Missing device capabilities".
- **Create release tag** now dispatches **Release** after the tag push. Pushes
  made with the default `GITHUB_TOKEN` do not trigger other workflows on GitHub.

## [0.1.0-rc2] - 2026-05-22

### Added

- **Create release tag** GitHub Actions workflow
  (`.github/workflows/create-release-tag.yml`): maintainer `workflow_dispatch`
  validates SemVer + `CHANGELOG.md`, pushes an annotated `v*` tag. Guide:
  [`docs/guides/releasing.md`](./docs/guides/releasing.md). *(Release dispatch
  fix landed after this tag — use **Trigger Release for tag** for `v0.1.0-rc2`.)*

### Changed

- Documentation pass: README, diagrams, security anchors, and release
  docs aligned with current feature set (groups v2, sealed sender, TLS
  pinning, `bin/signal-go` from `task build`).

## [0.1.0-rc1] - 2026-05-22

First tagged pre-release: cross-platform CLI binaries, Windows local-build
support, and TLS trust fixes for Signal's private CA.

### Added

- Cross-platform release pipeline ([ADR 0033](./docs/adr/0033-release-pipeline.md)):
  `.github/workflows/release.yml` builds `signal-go` on Linux (amd64/arm64),
  macOS (amd64/arm64), and Windows (amd64, experimental), packages archives
  + `.sha256`, and uploads to a draft GitHub Release on `v*` tag push.
  `workflow_dispatch` dry-run skips publish.
- `signal-go version` / `--version` (build tag, Go toolchain, VCS metadata).
- Windows local dev ergonomics: `.env.example`, `scripts/dev-env.sh`,
  `scripts/go.sh`, MinGW `fiat_p256_adx` link stubs in `build-libsignal.sh`,
  pre-push hook sources dev-env for cgo.
- Signal private TLS root pinning for `*.signal.org` — vendored
  `signal-messenger.cer` from Signal-iOS ([ADR 0034](./docs/adr/0034-signal-tls-root-pinning.md)).
- Mozilla NSS fallback roots via `golang.org/x/crypto/x509roots/fallback` for
  hosts where the OS trust store is empty (notably cgo Windows).

### Fixed

- **Release CI (macOS):** portable libsignal version extraction (`sed` instead of
  `grep -oP`); per-toolchain cgo `cServiceID` typedef split (`!darwin` vs
  `darwin`); unified `arduino/setup-protoc@v3` for all platforms.
- **Release CI (Windows):** `rustup target add` inside the libsignal clone;
  explicit `PROTOC` path; `#cgo windows LDFLAGS` for Win32 deps.
- **Release CI (attest):** build-provenance attestation gated behind repo
  variable `ENABLE_BUILD_PROVENANCE=true` (private user-owned repos lack
  Artifact Attestations); `continue-on-error` on the attest step.
- **TLS:** `signal-go link` and REST/WebSocket traffic to Signal now verify
  against Signal's private CA without installing it in the OS store.
- **Tests:** filesystem mode `0600` assertions skipped on Windows
  (`fsstore.AssertFileMode0600`).

### Changed

- `actions/setup-go` v5 → v6 across workflows.
- Security contact published: `signal-go-security@thehappydinoa.dev`
  ([`SECURITY.md`](./SECURITY.md)).
- ROADMAP: Phase B/C CI and release pipeline items marked done.

[Unreleased]: https://github.com/thehappydinoa/signal-go/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/thehappydinoa/signal-go/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.1.0
[0.1.0-rc2]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.1.0-rc2
[0.1.0-rc1]: https://github.com/thehappydinoa/signal-go/releases/tag/v0.1.0-rc1
