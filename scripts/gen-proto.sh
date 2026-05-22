#!/usr/bin/env bash
# Generate Go code from vendored Signal .proto files.
#
# Each proto package lands in its own Go package under internal/proto/gen/
# to avoid namespace collisions and keep the public surface narrow.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
PROTO_DIR="$ROOT/proto"
OUT_DIR="$ROOT/internal/proto/gen"

command -v protoc        >/dev/null || { echo "protoc not found"        >&2; exit 1; }
command -v protoc-gen-go >/dev/null || { echo "protoc-gen-go not found (run: task setup)" >&2; exit 1; }

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

# Map upstream filenames to logical Go package names. Each gets its own subdir
# so the generated Go file is self-contained.
declare -A PACKAGES=(
  [Provisioning.proto]=provisioningpb
  [WebSocketResources.proto]=websocketpb
  [SignalService.proto]=signalservicepb
  [Groups.proto]=groupspb
  [StorageService.proto]=storagepb
  [StickerResources.proto]=stickerpb
  [DeviceName.proto]=devicenamepb
  [Backup.proto]=backuppbg
)

# Build the protoc --go_opt=M<filename>=<importpath>;<pkg> argument set so each
# `.proto` is generated into its own Go import path.
M_ARGS=()
for proto in "${!PACKAGES[@]}"; do
  pkg="${PACKAGES[$proto]}"
  importpath="github.com/thehappydinoa/signal-go/internal/proto/gen/$pkg"
  M_ARGS+=("--go_opt=M$proto=$importpath")
done

for proto in "${!PACKAGES[@]}"; do
  pkg="${PACKAGES[$proto]}"
  out="$OUT_DIR/$pkg"
  mkdir -p "$out"
  echo ">> $proto -> $out"
  protoc \
    --proto_path="$PROTO_DIR" \
    --go_out="$out" \
    --go_opt=paths=source_relative \
    "${M_ARGS[@]}" \
    "$proto"
  # protoc-gen-go writes <Name>.pb.go in the same relative path as the input.
  # Flatten so each pkg dir holds exactly one .pb.go file.
  generated="$out/${proto%.proto}.pb.go"
  if [[ ! -f "$generated" ]]; then
    echo "expected $generated, got:" >&2
    find "$out" -name '*.pb.go' >&2
    exit 1
  fi
done

echo ">> done"
find "$OUT_DIR" -name '*.pb.go' -printf '  %p\n'
