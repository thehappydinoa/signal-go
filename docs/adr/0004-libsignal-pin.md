# ADR 0004 — Pin libsignal to a fixed upstream tag

- Status: Accepted
- Date: 2026-05-20

## Context

libsignal's README states explicitly: "Use outside of Signal is unsupported.
The API may change at any time." We need predictable builds.

We also need to decide whether to ship pre-built `libsignal_ffi.a` binaries
or always build from source.

## Decision

- Pin to a specific upstream tag in `scripts/build-libsignal.sh`. Current
  pin: **`v0.94.1`**.
- `task libsignal` clones at the pinned tag and `cargo build --release -p
  libsignal-ffi`. Build is reproducible from source.
- Output `libsignal_ffi.a` is written to `internal/libsignal/lib/` and is
  **gitignored**. We do not commit binaries.
- The cbindgen-generated `signal_ffi.h` is **committed** under
  `internal/libsignal/include/`. It is small (~2.8k LOC, plain C header),
  reviewable, and version-locked to the pinned tag. `task libsignal` will
  overwrite it during builds, and CI checks for drift.
- Upgrading libsignal:
  1. Bump `LIBSIGNAL_VERSION` default in `scripts/build-libsignal.sh`.
  2. Run `task libsignal` to refresh the header.
  3. Commit the header diff in the same PR as any cgo-binding changes.
  4. Add a note to `CHANGELOG.md`.

## Consequences

- **Pro**: Reproducible builds; the only mutable input is the upstream tag.
- **Pro**: Header diffs surface upstream API changes in code review.
- **Con**: First build on any machine is slow (~10 min) and requires Rust.
- **Con**: We must update promptly when upstream releases security fixes.

## Future

- Optional GitHub Actions release pipeline that builds `.a` for
  `linux/{amd64,arm64}` and `darwin/{amd64,arm64}` and publishes them as
  release artifacts. Consumers could then `task libsignal:download` instead
  of building. Out of scope until v0.1.0.
