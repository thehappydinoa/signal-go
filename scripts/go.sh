#!/usr/bin/env bash
# Run go with repo dev env (MinGW on Windows, CGO_ENABLED, TEMP, PROTOC, …).
set -euo pipefail
# shellcheck source=dev-env.sh
source "$(dirname "$0")/dev-env.sh"
exec go "$@"
