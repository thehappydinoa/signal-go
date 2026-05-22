# AGENTS.md

## Cursor Cloud specific instructions

This is a Go + cgo project that statically links Rust's `libsignal_ffi.a`. The build
requires Go 1.25+, Rust/Cargo, gcc/g++, nasm, and protoc as system deps.

### Key commands

| Action | Command |
|--------|---------|
| Build all | `task build` or `go build -trimpath ./...` |
| Unit tests | `task test` or `go test -race -count=1 ./...` |
| Lint | `task lint` or `golangci-lint run ./...` |
| Vet | `go vet ./...` |
| Component tests | `task test:component` |
| Build CLI | `go build -trimpath -o bin/signal-go ./cmd/signal-go` |

All commands require `CGO_ENABLED=1` (the default) and `libsignal_ffi.a` to be built.

### Critical build dependency: libsignal_ffi.a

The static library at `internal/libsignal/lib/libsignal_ffi.a` must exist before any
Go compilation involving cgo packages works. Build it once with:

```sh
LIBSIGNAL_VERSION=v0.94.1 bash scripts/build-libsignal.sh
```

Or via task: `task libsignal`. The script is idempotent (skips if already built for
the pinned version). First build takes ~3-5 minutes (Rust release build). The result
is cached in `.build/libsignal/` and `internal/libsignal/lib/`.

On cloud VMs you may need `export CC=gcc CXX=g++` if clang cannot link `-lstdc++`.

### Environment notes

- `GOPATH/bin` must be on `PATH` for `task`, `golangci-lint`, and `protoc-gen-go`.
- The Taskfile requires task v3.51+ (the repo's `Taskfile.yml` uses YAML-quoted
  strings for commands containing colons).
- This is a CLI/library project with no web server or database. The only network
  dependency is Signal's servers (`chat.signal.org`), used for e2e tests only.
- The `signal-go link` command requires an interactive passphrase prompt and a real
  Signal account to scan the QR code — it cannot be fully exercised without a phone.
  Use `--help` to verify the binary works.
