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
# Dev VM notes (cloud agents / minimal images): BoringSSL needs nasm, protoc,
# and a working C++ toolchain. If the default clang cannot link -lstdc++, use:
#   export CC=gcc CXX=g++
# When linking Go tests against libsignal_ffi.a you may also need:
#   export CGO_LDFLAGS="-lstdc++"
set -euo pipefail

LIBSIGNAL_VERSION="${LIBSIGNAL_VERSION:-v0.94.1}"
REPO_URL="https://github.com/signalapp/libsignal.git"

repo_root() { git -C "$(dirname "$0")/.." rev-parse --show-toplevel; }
ROOT="$(repo_root)"
LIB_DIR="$ROOT/internal/libsignal/lib"
INCLUDE_DIR="$ROOT/internal/libsignal/include"
BUILD_DIR="$ROOT/.build/libsignal"
STAMP="$LIB_DIR/.version"

mkdir -p "$LIB_DIR" "$INCLUDE_DIR" "$BUILD_DIR"

# patch_gnu_stack appends a zero-length `.note.GNU-stack` ELF section to
# every member of libsignal_ffi.a missing one. BoringSSL ships a handful
# of assembly objects without the section, so GNU ld emits a noisy
# "executable stack" warning on every Go link. The Go-produced binary
# itself is fine (Go injects PT_GNU_STACK as non-exec regardless), but
# the warning is alarming. Patching is idempotent.
patch_gnu_stack() {
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
        objcopy --add-section .note.GNU-stack=/dev/null "$obj" "$obj.patched"
        mv "$obj.patched" "$obj"
        patched=$((patched + 1))
      fi
    done
    if ls *.o >/dev/null 2>&1; then
      ar rcs "$LIB_DIR/libsignal_ffi.a" *.o
    fi
    echo ">> patched $patched object(s) missing .note.GNU-stack"
  )
  rm -rf "$PATCH_DIR"
}

if [[ "${FORCE:-0}" != "1" && -f "$LIB_DIR/libsignal_ffi.a" && -f "$STAMP" && "$(cat "$STAMP")" == "$LIBSIGNAL_VERSION" ]]; then
  echo "libsignal_ffi.a already built for $LIBSIGNAL_VERSION (use FORCE=1 to rebuild)"
  # The patch is idempotent and cheap; re-running guarantees existing
  # caches built before the patch landed (Phase 7 option 1) get fixed.
  patch_gnu_stack
  exit 0
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

echo ">> cargo build --release -p libsignal-ffi  (this takes a while)"
(
  cd "$BUILD_DIR"
  # libsignal-ffi declares crate-type = ["staticlib"], so this produces libsignal_ffi.a
  cargo build --release -p libsignal-ffi
)

SRC_LIB="$BUILD_DIR/target/release/libsignal_ffi.a"
if [[ ! -f "$SRC_LIB" ]]; then
  echo "build did not produce $SRC_LIB" >&2
  exit 1
fi

echo ">> installing artifacts"
cp -f "$SRC_LIB" "$LIB_DIR/libsignal_ffi.a"

patch_gnu_stack

# The cbindgen-generated header lives in the Swift consumer tree of upstream.
cp -f "$BUILD_DIR/swift/Sources/SignalFfi/signal_ffi.h" "$INCLUDE_DIR/signal_ffi.h"
echo "$LIBSIGNAL_VERSION" > "$STAMP"

echo ">> done"
ls -lh "$LIB_DIR/libsignal_ffi.a" "$INCLUDE_DIR/signal_ffi.h"
