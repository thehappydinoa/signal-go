# ADR 0043 — Strip LLVM LTO sections from libsignal_ffi.a

- Status: Accepted
- Date: 2026-11-23

## Context

Go 1.26 introduced a security hardening check in the cgo linker that
rejects static archives containing `.llvmbc` or `.llvmcmd` sections.
These sections hold LLVM bitcode used for Link-Time Optimization (LTO)
and could theoretically be re-linked or executed at load time — the Go
team considers them a risk in pre-built archives linked via `#cgo
LDFLAGS`.

Rust's default release profile (`lto = true` or `lto = "thin"`) embeds
LLVM bitcode alongside native code in every `.o` inside the archive.
Since libsignal upstream builds `libsignal-ffi` as a Rust staticlib
with LTO, the resulting `libsignal_ffi.a` contains these sections in
most of its object files.

After upgrading to Go 1.26 (required because Go 1.25 reached
end-of-life with 11 unfixed stdlib CVEs), `go vet`, `go build`, and
`govulncheck` all fail with:

    disallowed LTO section ".llvmbc" in #cgo LDFLAGS: .../libsignal_ffi.a

## Decision

Strip `.llvmbc` and `.llvmcmd` sections from every object in
`libsignal_ffi.a` as a post-build step in `scripts/build-libsignal.sh`,
using `objcopy --remove-section`. The stripping runs:

1. After a fresh `cargo build` (main build path).
2. After restoring from cache (cache-hit early exit).
3. After downloading a pre-built artifact (`download-libsignal.sh` path).

The function follows the same extract → patch → repack pattern as the
existing `patch_gnu_stack` post-processor and is similarly idempotent.

We also bump the CI cache key (`v1` → `v2`) to force a cache miss so
existing cached archives get rebuilt with the stripping applied.

## Alternatives considered

- **Disable Rust LTO in the libsignal build**: this would require
  patching upstream's `Cargo.toml`, which we avoid to keep the pin
  clean. LTO also produces meaningfully smaller and faster native code.
- **Set `CGO_LDFLAGS_ALLOW` to bypass the Go check**: this defeats the
  security hardening Go 1.26 introduced and would need to be set in
  every CI job and every developer's environment.
- **Stay on Go 1.25**: not viable — Go 1.25 is end-of-life with 11
  unfixed stdlib CVEs that govulncheck flags.

## Consequences

- `libsignal_ffi.a` is ~5–15% smaller on disk (bitcode sections are
  large). No functional change — native code is unaffected.
- Requires `objcopy` on the build host (standard on Linux CI runners;
  ships with binutils). macOS and Windows are unaffected because Go's
  LTO check targets ELF only.
- If a future Go release adds additional disallowed sections, this
  function is the natural place to extend the strip list.
- If Rust/libsignal upstream stops embedding LTO bitcode (e.g. a
  `codegen-units = 1` build without bitcode), the strip becomes a no-op.
