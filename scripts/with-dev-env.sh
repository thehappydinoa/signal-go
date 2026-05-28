#!/usr/bin/env bash
# Run a command with local cgo/toolchain env (see scripts/dev-env.sh).
#
#   bash scripts/with-dev-env.sh go test ./...
#   bash scripts/with-dev-env.sh go build -o bin/signal-go ./cmd/signal-go
set -euo pipefail
_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/dev-env.sh disable=SC1091
source "$_root/scripts/dev-env.sh"
if [[ $# -eq 0 ]]; then
  echo "usage: with-dev-env.sh <command> [args...]" >&2
  exit 2
fi
exec "$@"
