#!/usr/bin/env bash
# Download a pre-built libsignal_ffi.a from the GitHub Releases of this repo.
#
# Exits 0 on success (artifact installed and stamp written).
# Exits non-zero if the artifact is unavailable — the caller should fall back
# to building from source via build-libsignal.sh.
#
# Environment:
#   LIBSIGNAL_VERSION   override the pinned version (default: read from build-libsignal.sh)
#   GITHUB_REPO         override "thehappydinoa/signal-go"
#   FORCE               set to 1 to re-download even if the stamp already matches
#
# Normally called by build-libsignal.sh as a fast path; can also be run
# directly when Rust is unavailable.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
repo_root() { git -C "$SCRIPT_DIR/.." rev-parse --show-toplevel; }
ROOT="$(repo_root)"

# Read the pinned version from the build script if not overridden.
LIBSIGNAL_VERSION="${LIBSIGNAL_VERSION:-$(
  sed -n 's/^LIBSIGNAL_VERSION="${LIBSIGNAL_VERSION:-\([^}]*\)}".*/\1/p' \
    "$SCRIPT_DIR/build-libsignal.sh"
)}"
if [[ -z "$LIBSIGNAL_VERSION" ]]; then
  echo ">> download-libsignal: cannot read LIBSIGNAL_VERSION from build-libsignal.sh" >&2
  exit 1
fi

GITHUB_REPO="${GITHUB_REPO:-thehappydinoa/signal-go}"
LIB_DIR="$ROOT/internal/libsignal/lib"
STAMP="$LIB_DIR/.version"

# Detect host OS and arch (mirrors detect_host in build-libsignal.sh).
HOST_OS="$(uname -s)"
HOST_ARCH="$(uname -m)"
case "$HOST_OS" in
  Linux)             OS_KIND="linux"   ;;
  Darwin)            OS_KIND="darwin"  ;;
  MINGW*|MSYS*|CYGWIN*) OS_KIND="windows" ;;
  *)
    echo ">> download-libsignal: unsupported OS: $HOST_OS" >&2
    exit 1
    ;;
esac
case "$HOST_ARCH" in
  x86_64|amd64)  ARCH_KIND="amd64" ;;
  aarch64|arm64) ARCH_KIND="arm64" ;;
  *)
    echo ">> download-libsignal: unsupported arch: $HOST_ARCH" >&2
    exit 1
    ;;
esac

STAMP_VALUE="$LIBSIGNAL_VERSION-$OS_KIND-$ARCH_KIND"

# Exit early if the artifact is already present and stamped.
if [[ "${FORCE:-0}" != "1" && -f "$LIB_DIR/libsignal_ffi.a" && -f "$STAMP" ]]; then
  current="$(cat "$STAMP")"
  if [[ "$current" == "$STAMP_VALUE" ]]; then
    echo ">> libsignal_ffi.a already present for $STAMP_VALUE"
    exit 0
  fi
fi

ASSET_NAME="libsignal-ffi-${LIBSIGNAL_VERSION}-${OS_KIND}-${ARCH_KIND}.a"
RELEASE_TAG="libsignal-${LIBSIGNAL_VERSION}"
BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/${RELEASE_TAG}"
ASSET_URL="${BASE_URL}/${ASSET_NAME}"
SHA256_URL="${BASE_URL}/${ASSET_NAME}.sha256"

mkdir -p "$LIB_DIR"

TMP_A="$(mktemp)"
TMP_SHA="$(mktemp)"
trap 'rm -f "$TMP_A" "$TMP_SHA"' EXIT

echo ">> downloading ${ASSET_NAME}"
if ! curl -fsSL --retry 3 --retry-delay 2 -o "$TMP_A" "$ASSET_URL" 2>/dev/null; then
  echo ">> pre-built artifact not available at ${ASSET_URL}" >&2
  echo ">> (run 'task libsignal' to build from source, or 'go run tools/libsignal_setup.go')" >&2
  exit 1
fi

# Verify SHA256 if the checksum file is reachable.
if curl -fsSL --retry 3 -o "$TMP_SHA" "$SHA256_URL" 2>/dev/null; then
  expected="$(awk '{print $1}' "$TMP_SHA")"
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$TMP_A" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$TMP_A" | awk '{print $1}')"
  else
    echo ">> warning: no sha256sum/shasum found; skipping checksum verification" >&2
    actual="$expected"
  fi
  if [[ "$actual" != "$expected" ]]; then
    echo ">> SHA256 mismatch for ${ASSET_NAME}: expected $expected, got $actual" >&2
    exit 1
  fi
  echo ">> SHA256 verified: $actual"
else
  echo ">> warning: SHA256 file unavailable; skipping checksum verification" >&2
fi

cp "$TMP_A" "$LIB_DIR/libsignal_ffi.a"
echo "$STAMP_VALUE" > "$STAMP"
echo ">> installed ${ASSET_NAME}"
ls -lh "$LIB_DIR/libsignal_ffi.a"
