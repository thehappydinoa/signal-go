# ADR 0007 — Use Taskfile (not Make) for orchestration

- Status: Accepted
- Date: 2026-05-20

## Context

We need an orchestrator for build, codegen, test, lint, libsignal compile.
Options: `make`, [`go-task/task`](https://taskfile.dev), `mage`, raw shell.

## Decision

Use `task` (`Taskfile.yml`). Reasons:

- YAML is easier to read than recursive Make for a Go-focused workflow.
- First-class file-based caching with `sources:` and `generates:` removes
  the need for hand-rolled `.PHONY` and timestamp tricks.
- Cross-platform (works on Windows without Cygwin/MSYS), matters because
  Signal-Desktop runs on Windows and contributors may too.
- One-line install: `go install github.com/go-task/task/v3/cmd/task@latest`.

We will not require `task` for using the library — only for building from
source and developing. The Taskfile shells out to plain `go`, `protoc`, and
`cargo` so anyone can reproduce manually if needed.

## Consequences

- **Pro**: Contributors get a single discoverable entry point (`task
  --list`).
- **Pro**: CI calls the same tasks developers do — no drift.
- **Con**: One more tool to install. We bootstrap it from `go install` in
  the `setup` task so the cost is one command.
