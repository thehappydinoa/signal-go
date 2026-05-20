#!/usr/bin/env bash
# Build libsignal_ffi as a static library, pinned to a known-good upstream tag.
#
# Outputs:
#   internal/libsignal/lib/libsignal_ffi.a
#   internal/libsignal/include/signal_ffi.h
#
# Re-running with the same LIBSIGNAL_VERSION is a no-op if the artifacts exist.
# Override LIBSIGNAL_VERSION or pass FORCE=1 to rebuild.
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

if [[ "${FORCE:-0}" != "1" && -f "$LIB_DIR/libsignal_ffi.a" && -f "$STAMP" && "$(cat "$STAMP")" == "$LIBSIGNAL_VERSION" ]]; then
  echo "libsignal_ffi.a already built for $LIBSIGNAL_VERSION (use FORCE=1 to rebuild)"
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
# The cbindgen-generated header lives in the Swift consumer tree of upstream.
cp -f "$BUILD_DIR/swift/Sources/SignalFfi/signal_ffi.h" "$INCLUDE_DIR/signal_ffi.h"
echo "$LIBSIGNAL_VERSION" > "$STAMP"

echo ">> done"
ls -lh "$LIB_DIR/libsignal_ffi.a" "$INCLUDE_DIR/signal_ffi.h"
