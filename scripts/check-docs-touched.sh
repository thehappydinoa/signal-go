#!/usr/bin/env bash
# check-docs-touched.sh — warn when API-visible files change without touching docs/.
#
# Usage:
#   check-docs-touched.sh <base-sha> [<head-sha>]
#
# Exits 0 always. Emits a GitHub Actions ::warning:: annotation (and a
# plain-text warning on non-CI terminals) when API-visible paths changed
# but docs/ was not touched.
set -euo pipefail

BASE="${1:-}"
HEAD="${2:-HEAD}"

if [ -z "$BASE" ]; then
  echo "Usage: $0 <base-sha> [<head-sha>]" >&2
  exit 1
fi

changed=$(git diff --name-only "$BASE" "$HEAD" 2>/dev/null || true)

if [ -z "$changed" ]; then
  echo "No changed files detected between $BASE and $HEAD."
  exit 0
fi

# API-visible paths: public-facing code, CLI, and internal/ (cgo boundary, stores).
api_files=$(echo "$changed" | grep -E '^(cmd/|pkg/|internal/)' || true)
# Docs paths: guides, diagrams, ADRs, top-level README.
doc_files=$(echo "$changed" | grep -E '^(docs/|README\.md)' || true)

if [ -z "$api_files" ]; then
  exit 0
fi

if [ -n "$doc_files" ]; then
  echo "Docs updated alongside code changes. Good."
  exit 0
fi

warn() {
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::warning::API-visible files changed but no docs/ files were touched. Check the table in CLAUDE.md for which docs to update."
  else
    echo "WARNING: API-visible files changed but no docs/ files were touched."
    echo "Check the table in CLAUDE.md for which docs to update."
  fi
}

warn
echo ""
echo "Changed API-visible files (not exhaustive — first 20):"
echo "$api_files" | head -20
echo ""
echo "Typical docs to check:"
echo "  cmd/signal-go/main.go changed?  → docs/guides/getting-started.md"
echo "  pkg/signal/* changed?           → docs/guides/getting-started.md, CHANGELOG.md"
echo "  internal/store/* changed?       → docs/diagrams/encrypted-store.md, ADR 0012"
echo "  internal/libsignal/* changed?   → docs/security.md (if threat model shifts)"
echo ""
echo "This is a warning, not a failure. Review before merging."
exit 0
