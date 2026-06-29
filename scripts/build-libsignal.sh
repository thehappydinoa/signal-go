#!/usr/bin/env bash
# Build libsignal_ffi as a static library, pinned to a known-good upstream tag.
#
# Outputs:
#   internal/libsignal/lib/libsignal_ffi.a
#   internal/libsignal/include/signal_ffi.h
#
# Re-running with the same LIBSIGNAL_VERSION is a no-op if the artifacts exist.
# Override LIBSIGNAL_VERSION or pass FORCE=1 to rebuild.
#
# Supported hosts:
#   linux/amd64, linux/arm64    — native build
#   darwin/amd64, darwin/arm64  — native build
#   windows/amd64               — build via Git Bash / MSYS2; we explicitly
#                                 target x86_64-pc-windows-gnu so the resulting
#                                 .a links against MinGW-w64 (what cgo expects).
#
# Dev VM notes (cloud agents / minimal images): BoringSSL needs nasm, protoc,
# and a working C++ toolchain. If the default clang cannot link -lstdc++, use:
#   export CC=gcc CXX=g++
# When linking Go tests against libsignal_ffi.a you may also need:
#   export CGO_LDFLAGS="-lstdc++"
set -euo pipefail

# Apply repo-local dev env (MinGW PATH, TEMP, PROTOC_INCLUDE, CGO_ENABLED, …).
# shellcheck source=dev-env.sh
source "$(dirname "$0")/dev-env.sh"

LIBSIGNAL_VERSION="${LIBSIGNAL_VERSION:-v0.96.4}"
REPO_URL="https://github.com/signalapp/libsignal.git"

repo_root() { git -C "$(dirname "$0")/.." rev-parse --show-toplevel; }
ROOT="$(repo_root)"
LIB_DIR="$ROOT/internal/libsignal/lib"
INCLUDE_DIR="$ROOT/internal/libsignal/include"
BUILD_DIR="$ROOT/.build/libsignal"
STAMP="$LIB_DIR/.version"

mkdir -p "$LIB_DIR" "$INCLUDE_DIR" "$BUILD_DIR"

# detect_host classifies the host OS/arch so the rest of the script can pick
# the right cargo target triple and post-processing steps.
detect_host() {
  HOST_OS="$(uname -s)"
  HOST_ARCH="$(uname -m)"
  case "$HOST_OS" in
    Linux)
      OS_KIND="linux"
      ;;
    Darwin)
      OS_KIND="darwin"
      ;;
    MINGW*|MSYS*|CYGWIN*)
      OS_KIND="windows"
      ;;
    *)
      echo "unsupported host OS: $HOST_OS" >&2
      exit 1
      ;;
  esac

  case "$HOST_ARCH" in
    x86_64|amd64) ARCH_KIND="amd64" ;;
    aarch64|arm64) ARCH_KIND="arm64" ;;
    *)
      echo "unsupported host arch: $HOST_ARCH" >&2
      exit 1
      ;;
  esac

  # Map to a Rust target triple. CARGO_TARGET may be overridden externally
  # (e.g. by the release workflow to cross-compile). When empty we let
  # Cargo pick its default for the host, which keeps the legacy
  # `target/release/` output path and avoids invalidating existing
  # incremental-build caches on dev machines.
  if [[ -z "${CARGO_TARGET:-}" ]]; then
    case "$OS_KIND/$ARCH_KIND" in
      linux/amd64|linux/arm64|darwin/amd64|darwin/arm64)
        CARGO_TARGET=""  # use host default
        ;;
      windows/amd64)
        # cgo on Windows expects the MinGW (windows-gnu) ABI; the MSVC
        # default would produce signal_ffi.lib, which won't link.
        CARGO_TARGET="x86_64-pc-windows-gnu"
        ;;
      *)
        echo "unsupported host platform: $OS_KIND/$ARCH_KIND" >&2
        exit 1
        ;;
    esac
  fi
}
detect_host

# patch_gnu_stack appends a zero-length `.note.GNU-stack` ELF section to
# every member of libsignal_ffi.a missing one. BoringSSL ships a handful
# of assembly objects without the section, so GNU ld emits a noisy
# "executable stack" warning on every Go link. The Go-produced binary
# itself is fine (Go injects PT_GNU_STACK as non-exec regardless), but
# the warning is alarming. Patching is idempotent.
#
# Only meaningful on Linux: ELF is Linux-only, so macOS (Mach-O) and
# Windows (PE/COFF) skip this step.
patch_gnu_stack() {
  if [[ "$OS_KIND" != "linux" ]]; then
    return 0
  fi
  if ! command -v ar >/dev/null || ! command -v objcopy >/dev/null || ! command -v readelf >/dev/null; then
    echo ">> warning: ar/objcopy/readelf not found; skipping GNU-stack patch" >&2
    return 0
  fi
  echo ">> patching libsignal_ffi.a for .note.GNU-stack"
  PATCH_DIR="$(mktemp -d)"
  (
    cd "$PATCH_DIR"
    ar x "$LIB_DIR/libsignal_ffi.a"
    patched=0
    for obj in *.o; do
      [[ -f "$obj" ]] || continue
      if ! readelf -S "$obj" 2>/dev/null | grep -q '\.note\.GNU-stack'; then
        if objcopy --add-section .note.GNU-stack=/dev/null "$obj" "$obj.patched"; then
          mv "$obj.patched" "$obj"
          patched=$((patched + 1))
        else
          echo ">> warning: objcopy failed to patch $obj; leaving as-is" >&2
          rm -f "$obj.patched" || true
        fi
      fi
    done
    if ls *.o >/dev/null 2>&1; then
      ar rcs "$LIB_DIR/libsignal_ffi.a" *.o
    fi
    echo ">> patched $patched object(s) missing .note.GNU-stack"
  )
  rm -rf "$PATCH_DIR"
}

# BoringSSL only assembles fiat_p256_adx_{mul,sqr} for Mach-O/ELF. MinGW emits
# COFF, so bcm.cc.obj references those symbols without defining them. Compile
# portable fallbacks and append to libsignal_ffi.a (idempotent).
needs_windows_fiat_adx_stubs() {
  [[ "$OS_KIND" == "windows" ]] || return 1
  [[ -f "$LIB_DIR/libsignal_ffi.a" ]] || return 1
  ! nm "$LIB_DIR/libsignal_ffi.a" 2>/dev/null | grep -q ' T fiat_p256_adx_mul'
}

patch_windows_fiat_adx_stubs() {
  if ! needs_windows_fiat_adx_stubs; then
    return 0
  fi
  local boring_out stub_o cxx
  boring_out="$(find "$BUILD_DIR/target" -path '*/build/boring-sys-*/out/boringssl' -type d 2>/dev/null | head -1)"
  if [[ -z "$boring_out" || ! -f "$boring_out/include/openssl/base.h" ]]; then
    echo ">> warning: boring-sys build tree not found; cannot add fiat_p256_adx stubs" >&2
    echo ">>   Re-run without a cache hit (FORCE=1) after a full cargo build." >&2
    return 1
  fi
  cxx="${CXX:-g++}"
  stub_o="$BUILD_DIR/fiat_p256_adx_stub.o"
  echo ">> appending Windows fiat_p256_adx stubs to libsignal_ffi.a"
  "$cxx" -c -O2 -DOPENSSL_NO_ASM=1 -DOPENSSL_STATIC=1 \
    -I"$boring_out/include" -I"$boring_out" \
    -o "$stub_o" "$ROOT/scripts/win_fiat_p256_adx_stub.cc"
  ar q "$LIB_DIR/libsignal_ffi.a" "$stub_o"
}

# Stamp format is `<version>-<os>-<arch>`. Older builds wrote just `<version>`;
# treat those as compatible to avoid pointless rebuilds on dev machines that
# upgrade through this commit.
STAMP_NEW="$LIBSIGNAL_VERSION-$OS_KIND-$ARCH_KIND"
STAMP_LEGACY="$LIBSIGNAL_VERSION"
if [[ "${FORCE:-0}" != "1" && -f "$LIB_DIR/libsignal_ffi.a" && -f "$STAMP" ]]; then
  current="$(cat "$STAMP")"
  if [[ "$current" == "$STAMP_NEW" || "$current" == "$STAMP_LEGACY" ]]; then
    if [[ "$current" == "$STAMP_LEGACY" ]]; then
      echo "$STAMP_NEW" > "$STAMP"
    fi
    echo "libsignal_ffi.a already built for $LIBSIGNAL_VERSION on $OS_KIND/$ARCH_KIND (use FORCE=1 to rebuild)"
    # The patch is idempotent and cheap; re-running guarantees existing
    # caches built before the patch landed (Phase 7 option 1) get fixed.
    patch_gnu_stack
    patch_windows_fiat_adx_stubs || true
    exit 0
  fi
fi

# Fast path: download a pre-built artifact rather than invoking cargo.
# SKIP_DOWNLOAD=1 disables this (used by the libsignal-artifacts CI workflow
# that is itself responsible for producing the artifacts).
if [[ "${SKIP_DOWNLOAD:-0}" != "1" ]]; then
  if bash "$(dirname "$0")/download-libsignal.sh"; then
    # Patches are cheap and idempotent; apply regardless of download vs build.
    patch_gnu_stack
    patch_windows_fiat_adx_stubs || true
    echo ">> done"
    ls -lh "$LIB_DIR/libsignal_ffi.a" "$INCLUDE_DIR/signal_ffi.h" 2>/dev/null || true
    exit 0
  fi
  echo ">> pre-built artifact unavailable; falling back to cargo build"
fi

# Shallow-clone (or refresh) at the pinned tag.
if [[ ! -d "$BUILD_DIR/.git" ]]; then
  echo ">> cloning $REPO_URL at $LIBSIGNAL_VERSION"
  git clone --depth=1 --branch "$LIBSIGNAL_VERSION" "$REPO_URL" "$BUILD_DIR"
else
  echo ">> updating clone to $LIBSIGNAL_VERSION"
  git -C "$BUILD_DIR" fetch --depth=1 origin "refs/tags/$LIBSIGNAL_VERSION:refs/tags/$LIBSIGNAL_VERSION"
  git -C "$BUILD_DIR" checkout -q "$LIBSIGNAL_VERSION"
fi

# libsignal-message-backup/build.rs (json feature) calls prost_build::compile_protos
# with &[PROTOS_DIR] where PROTOS_DIR="protos" (the output subdir), but the proto
# files live in "src/". Newer protoc (>=3.21) is strict about --proto_path matching
# and rejects the mismatch. Patch the include dir to "src" before cargo runs.
# Safe to apply unconditionally: grep is a no-op when the pattern isn't present.
_msg_backup_build_rs="$BUILD_DIR/rust/message-backup/build.rs"
if [[ -f "$_msg_backup_build_rs" ]] && \
   grep -qF 'compile_protos(PROTOS, &[PROTOS_DIR])' "$_msg_backup_build_rs"; then
  echo ">> patching rust/message-backup/build.rs: fix prost_build proto include dir (upstream bug)"
  # sed -i requires an explicit empty extension on BSD/macOS; .bak + rm is portable.
  sed -i.bak 's/compile_protos(PROTOS, \&\[PROTOS_DIR\])/compile_protos(PROTOS, \&["src"])/' \
    "$_msg_backup_build_rs" && rm -f "${_msg_backup_build_rs}.bak"
fi

CARGO_FLAGS=(build --release -p libsignal-ffi)
if [[ -n "$CARGO_TARGET" ]]; then
  CARGO_FLAGS+=(--target "$CARGO_TARGET")
  # Ensure the rustup target is installed on the toolchain that cargo
  # will actually invoke. libsignal upstream pins a nightly via
  # $BUILD_DIR/rust-toolchain; running `rustup target add` from the
  # signal-go repo root would resolve to the system default (stable)
  # and silently leave the pinned nightly without the cross-target
  # std lib — which is how the first Windows release run failed
  # ("error[E0463]: can't find crate for `core`" on cfg-if).
  if command -v rustup >/dev/null 2>&1; then
    echo ">> ensuring rustup target $CARGO_TARGET is installed for libsignal's pinned toolchain"
    (cd "$BUILD_DIR" && rustup target add "$CARGO_TARGET") || true
  fi
fi

echo ">> cargo ${CARGO_FLAGS[*]}  (this takes a while)"
(
  cd "$BUILD_DIR"
  # libsignal-ffi declares crate-type = ["staticlib"], so this produces
  # libsignal_ffi.a on every supported host (including windows-gnu).
  cargo "${CARGO_FLAGS[@]}"
)

if [[ -n "$CARGO_TARGET" ]]; then
  SRC_LIB="$BUILD_DIR/target/$CARGO_TARGET/release/libsignal_ffi.a"
else
  SRC_LIB="$BUILD_DIR/target/release/libsignal_ffi.a"
fi
if [[ ! -f "$SRC_LIB" ]]; then
  echo "build did not produce $SRC_LIB" >&2
  echo "contents of build output dir:" >&2
  ls -la "$(dirname "$SRC_LIB")" 2>&1 | head -30 >&2 || true
  exit 1
fi

echo ">> installing artifacts"
cp -f "$SRC_LIB" "$LIB_DIR/libsignal_ffi.a"

patch_gnu_stack
patch_windows_fiat_adx_stubs

# The cbindgen-generated header lives in the Swift consumer tree of upstream.
cp -f "$BUILD_DIR/swift/Sources/SignalFfi/signal_ffi.h" "$INCLUDE_DIR/signal_ffi.h"
echo "$STAMP_NEW" > "$STAMP"

echo ">> done"
ls -lh "$LIB_DIR/libsignal_ffi.a" "$INCLUDE_DIR/signal_ffi.h"
