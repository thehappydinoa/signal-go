# ADR 0033 — Release pipeline (Linux, macOS, Windows)

- Status: Accepted
- Date: 2026-05-22

## Context

[ADR 0013](./0013-ci-github-actions.md) carved the CI pipeline into
three phases. Phase A (lint + vet + test on `ubuntu-latest`) and Phase
B's broader Linux-only coverage (staticcheck, coverage, fuzz nightly)
have shipped. The remaining Phase B item (cross-platform validation) and
Phase C (a tagged-release workflow that produces signed binaries) are
both prerequisites for `v0.1.0` — see
[ROADMAP](../../ROADMAP.md#continuous-integration--quality-ongoing).

We need to answer three concrete questions:

1. Which platforms get an official `signal-go` binary on a release tag?
2. How do we build `libsignal_ffi.a` on each of those platforms?
3. How do we keep the release workflow stable when upstream libsignal
   only officially supports MSVC on Windows but cgo on Windows needs a
   GCC-compatible toolchain?

## Decision

### Supported platforms

| Platform        | Runner             | Cargo target              | Archive |
|-----------------|--------------------|---------------------------|---------|
| `linux/amd64`   | `ubuntu-latest`    | host default              | `.tar.gz` |
| `linux/arm64`   | `ubuntu-24.04-arm` | host default              | `.tar.gz` |
| `darwin/amd64`  | `macos-15-intel`   | host default              | `.tar.gz` |
| `darwin/arm64`  | `macos-latest`     | host default              | `.tar.gz` |
| `windows/amd64` | `windows-latest`   | `x86_64-pc-windows-gnu`   | `.zip`  |

Every leg builds natively on a runner that matches its target — no
cgo cross-compilation. Cross-compiling Go programs that link a C/Rust
static archive (cgo) involves shipping a sysroot, a matching linker,
and a libstdc++ for every arch; that's much more fragile than running
each leg on its native runner. GitHub now offers free-tier ARM Linux
and Apple Silicon runners, so the cost is no longer prohibitive.

### Windows: MinGW-w64, not MSVC

Upstream libsignal builds the Windows variants of their Java/Node
artifacts against the MSVC ABI (`*-pc-windows-msvc`) and uses
Chocolatey `nasm` to satisfy BoringSSL. We cannot do the same. Cgo on
Windows expects a GCC-compatible toolchain — building against an
MSVC-emitted `signal_ffi.lib` is unsupported in practice. Therefore we
build with the `x86_64-pc-windows-gnu` Rust target so Cargo emits
`libsignal_ffi.a`, and we link against MSYS2's `mingw-w64-x86_64-gcc`
on the runner.

This is the **less-trodden path** for BoringSSL builds. We flag the
Windows leg as `experimental: true` in the matrix: it runs with
`continue-on-error: true`, so a transient toolchain failure cannot
block a Linux/macOS release. We promote Windows to non-experimental
once we have shipped one clean release through it.

### Trigger model

`release.yml` runs on:

- **`push: tags: ['v*']`** — the real release path. Builds every leg
  and uploads to a **draft** GitHub Release. A maintainer reviews the
  generated notes + asset list and clicks **Publish** manually. Cut a
  release with the `:` git tag, then go check the draft.
- **`workflow_dispatch`** — manual dry-run. Builds every leg and keeps
  the resulting archives as 7-day workflow artifacts; the publish step
  is gated behind `startsWith(github.ref, 'refs/tags/v')`, so a dry-run
  cannot accidentally cut a release.

`workflow_dispatch` is the mechanism we use to validate Windows /
macOS plumbing between releases — there is no separate "cross-platform
CI" job on every PR. PR latency stays low, and the Windows leg
specifically is too expensive to gate every PR on.

### Caching

Each matrix leg keys an `actions/cache@v5` entry on
`libsignal-${version}-${runner}-${arch}-${flavour}`. A re-run on the
same tag (or two close tags) avoids the 5–10 minute libsignal build.
We deliberately scope keys per-runner and per-flavour so:

- `windows-latest-x86_64-gnu` does not collide with a future
  `windows-latest-x86_64-msvc` if we add one.
- `ubuntu-latest` and `ubuntu-24.04-arm` keep separate `.a` files.

### Binary packaging

Each leg produces `signal-go-<version>-<os>-<arch>.{tar.gz,zip}`
containing `signal-go[.exe]`, `LICENSE`, `NOTICE`, and `README.md`.
A `.sha256` sidecar is uploaded alongside the archive so downstream
consumers can verify integrity without trusting only the GitHub
Release page.

The Go binary is linked with `-ldflags="-s -w -X main.version=<tag>"`;
`signal-go version` / `signal-go --version` prints the embedded tag,
Go toolchain, and the `vcs.{revision,time,modified}` block that
`go build -buildvcs=true` (the default) writes to
`runtime/debug.ReadBuildInfo`.

### Provenance

Every archive built from a real tag push *can be* signed by Sigstore
via [`actions/attest-build-provenance@v3`](https://github.com/actions/attest-build-provenance).
The attestation lands on the GitHub Release run and is verifiable with
`gh attestation verify <archive> --repo thehappydinoa/signal-go`. Dry-
runs (`workflow_dispatch`) skip the attest step — Sigstore declines to
attest non-tag refs anyway, and a dry-run shouldn't pollute the public
transparency log.

**Why "can be" rather than "is".** GitHub's Artifact Attestations
feature [is not available for user-owned private repositories](https://docs.github.com/rest/repos/attestations#create-an-attestation):
the API returns

```
Failed to persist attestation: Feature not available for
user-owned private repositories. To enable this feature,
please make this repository public.
```

`signal-go` is currently a user-owned private repo, so the attest
step is gated behind an opt-in repo variable:
`vars.ENABLE_BUILD_PROVENANCE == 'true'`. To enable:

1. Make the repo public (or move it to an org plan that supports
   attestations).
2. Set the repo variable `ENABLE_BUILD_PROVENANCE` to `true` under
   *Settings → Secrets and variables → Actions → Variables*.

`continue-on-error: true` is belt-and-suspenders: even if the feature
is enabled but a transient Sigstore issue surfaces, the rest of the
release pipeline still produces archives + checksums on the draft
release.

### Build-script changes

[`scripts/build-libsignal.sh`](../../scripts/build-libsignal.sh) now:

- Detects `Darwin` and `MINGW*/MSYS*/CYGWIN*` from `uname -s`.
- Honours an optional `CARGO_TARGET` env var. Empty (default) means
  "use Cargo's host default" — preserves existing developers' build
  caches on Linux. Set to `x86_64-pc-windows-gnu` for the Windows leg.
- Skips the `.note.GNU-stack` patch on non-Linux (it's an ELF-only
  workaround for the BoringSSL `.S` objects; harmless absence on
  Mach-O / PE-COFF).
- Stores `<version>-<os>-<arch>` in the on-disk stamp so a
  cross-platform agent re-syncing the repo does not silently reuse an
  incompatible `.a`. Legacy `<version>`-only stamps are migrated in
  place.

### cgo platform bindings

[`internal/libsignal/cgo.go`](../../internal/libsignal/cgo.go) gains a
`#cgo windows LDFLAGS` line that links the Win32 surface area
libsignal's transitive deps (rustls, boring-sys, tokio, hyper, ring)
reach for: `ws2_32, userenv, bcrypt, advapi32, ntdll, kernel32,
user32, crypt32, secur32, ncrypt, psapi, iphlpapi` plus `stdc++` and
`pthread` from MinGW. The Linux line additionally suppresses GNU ld's
executable-stack warning via `-Wl,--no-warn-execstack`, mirroring the
[Phase 7](../../ROADMAP.md#phase-7--niceties-planned-out-of-mvp)
fix already applied at the archive level.

## Consequences

**Pro** — five binaries per release tag, none requiring a maintainer
to leave CI. The same `task libsignal` invocation works on every
supported host. Draft-release workflow keeps a human in the loop
before public publish.

**Pro** — `workflow_dispatch` gives us a Windows / macOS smoke test we
can run on demand against a feature branch, without adding the
5-10-minute Windows leg to every PR.

**Con** — the Windows leg is fragile. BoringSSL + MinGW-w64 is not the
combination upstream libsignal ships in CI, so a transient build break
is more likely there than on Linux/macOS. The `experimental: true`
marker contains the blast radius until we have a few clean releases
under our belt.

**Con** — five matrix legs means a release tag spends GitHub Actions
minutes proportional to how many uncached libsignal builds happen. In
the worst case (every cache invalidated) we pay 25–30 runner-minutes
per release. Acceptable for the cadence we expect.

## Future

- Code signing on macOS (`codesign` + Developer ID) and Windows
  (`signtool` + EV cert). Out of scope for the first cut; once we
  have a project Apple ID / cert this is a small follow-up.
- Homebrew tap + Scoop manifest after `v0.1.0` ships once and the
  archive layout has stabilised.
- Promote Windows out of `experimental: true` after the first clean
  release.
- Optional `.deb` / `.rpm` packages for the Linux artifacts.
