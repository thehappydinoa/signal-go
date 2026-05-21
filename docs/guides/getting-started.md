# Getting started

`signal-go` is **pre-alpha**. You can pair it as a Signal secondary
device today, but receive/send aren't wired yet (Phases 3 & 4). This
guide walks the link flow.

## Prerequisites

- **Go 1.25+** (we use `crypto/hkdf` and other stdlib bits from recent releases)
- **A C toolchain** (gcc/clang on Linux/macOS, MSVC on Windows)
- **Rust** — only required *once* to build the pinned `libsignal_ffi.a`
- **`protoc`** if you need to regenerate the protobufs (we ship the
  generated code, so most contributors skip this)
- **A real Signal account** on your phone (to scan the QR)

## Build

```sh
git clone https://github.com/thehappydinoa/signal-go
cd signal-go

# One-time: build the pinned libsignal_ffi.a (~5–10 min on first run; cached after).
task libsignal

# Build the demo CLI.
task build
```

Don't have `task`? `go install github.com/go-task/task/v3/cmd/task@latest`
or read [`Taskfile.yml`](../../Taskfile.yml) and run the equivalent `go`
/ `bash` commands by hand.

## Pair as a secondary device

```sh
./bin/signal-go link -store ./.signal-data
```

You'll get an interactive passphrase prompt. The passphrase is used to
encrypt your account state (AES-256-GCM, with the key derived via
Argon2id) — see [the encrypted-store diagram](../diagrams/encrypted-store.md).

The tool then prints a `sgnl://linkdevice?...` URL. Two ways to use it:

1. **Open the QR**: paste the URL into your favourite terminal-based QR
   generator and scan it from your phone's *Signal → Settings → Linked
   devices → + (Add device)* menu.
2. **Type it manually**: not currently possible — Signal's mobile app
   doesn't expose a "paste URL" option. Use option 1.

After you approve on the phone, signal-go decrypts the provisioning
envelope, generates ACI + PNI prekeys, registers via
`PUT /v1/devices/link`, uploads one-time prekey batches, and persists
the account under `./.signal-data/`.

## Non-interactive

For systemd units, container deployments, CI, etc.:

```sh
# Read the passphrase from a file (trailing newline trimmed).
./bin/signal-go link -store /var/lib/mybot -passphrase-file /run/secrets/store-passphrase
```

Or supply your own 32-byte key by writing a small Go program against
`pkg/signal` and `internal/store/fsstore.NewWithKey`. The CLI doesn't
expose this directly to keep flag surface small.

## Verify the link

```sh
ls -l ./.signal-data
# -rw------- 1 you you 4096 May 20 17:00 account.enc
# -rw------- 1 you you  170 May 20 17:00 kdf.json
```

Open the Signal app → *Linked devices* and you should see "signal-go"
listed (or whatever you passed to `-name`).

## What's next

- **Receive** (Phase 3): pending. The [receive pipeline](../diagrams/receive-pipeline.md)
  describes the planned shape.
- **Send** (Phase 4): pending. The [send flow](../diagrams/send-flow.md)
  describes the planned shape.
- **Bot framework** (Phase 6): pending. See [ADR 0008](../adr/0008-bot-framework.md).

## Troubleshooting

- *"the requested URL returned error: 404"* during `task libsignal` —
  the upstream tag in `scripts/build-libsignal.sh` is wrong or Signal
  moved the repo. Check the [pinned version](../../scripts/build-libsignal.sh).
- *"wrong passphrase (or the store is corrupted)"* — make sure you
  typed the same passphrase you used at link time, or delete
  `./.signal-data/` and re-link.
- *Compilation errors mentioning `signal_*`* — re-run `task libsignal`.
  The header (`internal/libsignal/include/signal_ffi.h`) and static
  library (`internal/libsignal/lib/libsignal_ffi.a`) must come from the
  same upstream tag.
