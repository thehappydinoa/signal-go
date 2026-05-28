# ADR 0037 — Pre-built libsignal_ffi.a artifacts for downstream consumers

- Status: Accepted
- Date: 2026-05-27

## Context

Building `libsignal_ffi.a` from source requires Rust, cargo, nasm, protoc,
and a C++ toolchain — a significant barrier for downstream consumers who
embed `pkg/signal` as a library and have no use for the Signal protocol
implementation details. The build takes 5–10 minutes on first run and the
required toolchain varies by platform (Linux, macOS, Windows/MinGW).

[ADR 0004](./0004-libsignal-pin.md) pins libsignal to a known tag and
commits only the cbindgen-generated header; the static library (`.a`) is
gitignored and must be produced locally. [ADR 0033](./0033-release-pipeline.md)
ships pre-built `signal-go` CLI binaries but does not distribute the `.a`.

We need a mechanism that lets downstream library users (and contributors on
fresh machines) obtain `libsignal_ffi.a` without installing Rust.

## Decision

### Pre-built artifact distribution

A dedicated GitHub Actions workflow
(`.github/workflows/libsignal-artifacts.yml`) builds `libsignal_ffi.a` on
all five supported platforms (same matrix as `release.yml`) and publishes
the artifacts to a GitHub Release tagged **`libsignal-<VERSION>`** (e.g.
`libsignal-v0.94.1`).  Each asset is accompanied by a `.sha256` checksum
file.

Asset naming convention:

```
libsignal-ffi-<VERSION>-<os>-<arch>.a
libsignal-ffi-<VERSION>-<os>-<arch>.a.sha256
```

where `<os>` ∈ {`linux`, `darwin`, `windows`} and `<arch>` ∈ {`amd64`, `arm64`}.

The `libsignal-*` tag is separate from signal-go version tags (`v*`) so
that:
- The artifact release is decoupled from the CLI binary release cycle.
- The same `.a` can be reused across multiple signal-go releases that pin
  the same libsignal version.
- The pre-release flag on the artifact release keeps it off the "latest
  release" listing for end users.

The workflow triggers on:
- **push to `main`** when `scripts/build-libsignal.sh` changes (i.e. when
  the libsignal version is bumped), so artifacts are always current.
- **`workflow_dispatch`** for manual re-runs (e.g. adding a new platform
  without bumping the version).

### Download-first in build-libsignal.sh

`scripts/build-libsignal.sh` now tries
`scripts/download-libsignal.sh` before invoking cargo.  If the download
succeeds the script skips the entire Rust build.  The caller sets
`SKIP_DOWNLOAD=1` in the `libsignal-artifacts.yml` workflow itself (which
is what produces the artifacts) to prevent a circular dependency.

This means the common case — a developer on a release branch, or a library
consumer — gets a <30 second setup instead of a 5–10 minute build, with no
change to the `task libsignal` invocation.

### go generate bootstrap (Option B)

`internal/libsignal/cgo.go` carries a `//go:generate` directive pointing
at `tools/libsignal_setup.go`, a `//go:build ignore` pure-Go program that
uses only the standard library to:

1. Walk up from the working directory to find the module root.
2. Parse the pinned version from `scripts/build-libsignal.sh`.
3. Map `runtime.GOOS`/`runtime.GOARCH` to the asset naming.
4. Download from the GitHub Release URL and verify SHA256.
5. Write the `.a` and the version stamp.

Library consumers who do not have `task` installed can run:

```sh
go generate ./internal/libsignal/
# or equivalently:
go run tools/libsignal_setup.go
```

This requires only Go (already required for the project) and network access.

### Security model

- **Integrity**: SHA256 checksums are verified before the `.a` is installed.
  A checksum mismatch aborts with an error.
- **Authenticity**: The asset is served over GitHub's TLS.  We trust GitHub
  to serve what we uploaded; no additional signing is applied to the `.a`
  itself.  This is the same trust model as `go get` for Go modules.
- **Fallback**: When no pre-built artifact is available (unsupported
  platform, development commit, network failure) the tooling falls back to
  `cargo build` with a clear error message.  No silent fallback to an
  untrusted source.

## Consequences

**Pro** — library consumers and fresh contributors no longer need Rust,
cargo, nasm, or protoc.  `go run tools/libsignal_setup.go` replaces the
5–10 minute build.

**Pro** — `task libsignal` remains a single entry point; the download is
a transparent fast path.

**Con** — the `libsignal-artifacts.yml` workflow adds CI minutes (5 matrix
legs × 5–10 min = 25–50 min) on every libsignal version bump.  Acceptable
given the infrequency of bumps.

**Con** — artifacts between releases (development commits on `main` that
have not yet bumped the version) do not get new artifacts; the download
falls back to `cargo build`.  This is intentional — we only distribute
verified, pinned artifacts.

**Con** — Windows `.a` is still experimental (see
[ADR 0033](./0033-release-pipeline.md)); the Windows artifact leg runs
with `continue-on-error: true`, matching the CLI release matrix.

## Future

- Re-evaluate whether to also include the `.a` files in the main `v*`
  release assets once the release cadence stabilises.
- Consider Sigstore attestation for the `.a` files once the repo is public
  and the main binary attestation path is exercised.
