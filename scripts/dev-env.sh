#!/usr/bin/env bash
# Local developer environment for signal-go (cgo + libsignal).
# Safe to source on Linux/macOS (mostly no-op except CGO_ENABLED).
#
#   source scripts/dev-env.sh
# Task loads .env via dotenv; this script is the imperative equivalent.

_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
_root="$(cd "$_script_dir/.." && pwd)"

# MSYS2 MinGW on PATH for cgo (Windows only). Mirrors release.yml PATH setup.
_mingw_bin_dir() {
  case "$(uname -s 2>/dev/null || true)" in
    MINGW* | MSYS* | CYGWIN*) ;;
    *) return 1 ;;
  esac
  local -a roots=()
  if [[ -n "${MSYSTEM:-}" && -d /mingw64/bin ]]; then
    roots+=(/mingw64)
  fi
  local root
  for root in /c/msys64 /msys64 "${ProgramFiles:-/c/Program Files}/msys64"; do
    [[ -d "$root/mingw64/bin" ]] && roots+=("$root/mingw64")
  done
  local r
  for r in "${roots[@]}"; do
    if [[ -x "$r/bin/gcc.exe" ]]; then
      echo "$r/bin"
      return 0
    fi
  done
  return 1
}
if _mingw_bin="$(_mingw_bin_dir 2>/dev/null)"; then
  if ! command -v gcc.exe >/dev/null 2>&1 && ! command -v gcc >/dev/null 2>&1; then
    export PATH="$_mingw_bin:${PATH:-}"
  fi
  export CC="${CC:-$_mingw_bin/gcc.exe}"
  export CXX="${CXX:-$_mingw_bin/g++.exe}"
  if [[ -z "${CGO_LDFLAGS:-}" ]]; then
    export CGO_LDFLAGS="-lstdc++"
  fi
fi

export CGO_ENABLED="${CGO_ENABLED:-1}"

# Rust/cargo, gcc, and dlltool on Windows consult TEMP/TMP (Win32 APIs), not
# only TMPDIR. Point them at a writable directory (release runners use the
# default user Temp; C:\WINDOWS breaks without elevation).
case "$(uname -s 2>/dev/null || true)" in
  MINGW* | MSYS* | CYGWIN*)
    if [[ -z "${TMP:-}" || "${TMP}" == *WINDOWS* ]]; then
      if [[ -n "${LOCALAPPDATA:-}" ]]; then
        _win_tmp="${LOCALAPPDATA}/Temp"
      else
        _win_tmp="${USERPROFILE:-$HOME}/AppData/Local/Temp"
      fi
      export TMP="${_win_tmp//\\//}"
      export TEMP="$TMP"
    fi
    mkdir -p "$_root/.build/tmp" 2>/dev/null || true
    export TMPDIR="${TMPDIR:-$_root/.build/tmp}"
    ;;
esac

# prost-build (spqr) and libsignal-net-grpc need well-known protos on the include path.
if [[ -z "${PROTOC:-}" ]] && command -v protoc >/dev/null 2>&1; then
  export PROTOC="$(command -v protoc)"
fi
if [[ -z "${PROTOC_INCLUDE:-}" && -n "${PROTOC:-}" ]]; then
  _protoc_dir="$(dirname "$PROTOC")"
  for candidate in \
    "$_protoc_dir/../include" \
    "$_protoc_dir/../../include" \
    "/c/msys64/mingw64/include" \
    "/usr/local/include" \
    "/usr/include"; do
    if [[ -f "$candidate/google/protobuf/duration.proto" ]]; then
      export PROTOC_INCLUDE="${candidate//\\//}"
      break
    fi
  done
  # WinGet protobuf layout: .../Packages/Google.Protobuf_.../include
  if [[ -z "${PROTOC_INCLUDE:-}" && -d "${LOCALAPPDATA:-}/Microsoft/WinGet/Packages" ]]; then
    _winget_inc="$(find "${LOCALAPPDATA}/Microsoft/WinGet/Packages" -path '*/include/google/protobuf/duration.proto' 2>/dev/null | head -1)"
    if [[ -n "$_winget_inc" ]]; then
      # .../include/google/protobuf/duration.proto -> .../include
      export PROTOC_INCLUDE="$(dirname "$(dirname "$(dirname "$_winget_inc")")")"
      export PROTOC_INCLUDE="${PROTOC_INCLUDE//\\//}"
    fi
  fi
fi

# On Windows, match scripts/build-libsignal.sh unless overridden.
case "$(uname -s 2>/dev/null || true)" in
  MINGW* | MSYS* | CYGWIN*)
    : "${CC:=gcc}"
    : "${CXX:=g++}"
    export CC CXX
    ;;
esac
