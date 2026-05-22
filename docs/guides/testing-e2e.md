# End-to-end testing (real Signal)

These tests talk to production `chat.signal.org` with a **real linked
device** store. They never run in ordinary CI or `go test ./...`; use
the `e2e` build tag and `SIGNAL_GO_E2E=1`.

See also [Testing](./testing.md) and [ADR 0006](../adr/0006-testing-strategy.md).

## Prerequisites

- Network egress to `chat.signal.org` (TLS uses the pinned Signal root;
  see [ADR 0034](../adr/0034-signal-tls-root-pinning.md)).
- `libsignal_ffi.a` built (`task libsignal`).
- A **linked** device store under a dedicated directory (recommended:
  `internal/store/sqlstore`, not the CLI’s `fsstore`-only link path).
- A **peer** for 1:1 tests: usually your phone’s ACI (primary account).
- For group tests: a **64-character hex** group master key for a group
  your linked device is already in.

## One-time setup (sqlstore + link)

Link from library code or the CLI, but persist sessions in SQLite so
`Open` can decrypt traffic:

```go
db, _ := sqlstore.OpenWithPassphrase("./.signal-e2e", passphrase)
_, err := signal.Link(ctx, signal.LinkOptions{
    Store:        db,
    SignalStores: db.SignalStores(),
    OnURL:        printURL, // QR / URL for the phone
})
```

Or link with `signal-go` into a directory, then migrate / use a fresh
sqlstore directory and link again with the snippet above. The e2e
harness expects `SIGNAL_E2E_STORE_DIR` to contain `signal.db`.

## Running the suite

```sh
export SIGNAL_E2E_STORE_DIR=./.signal-e2e
export SIGNAL_E2E_PASSPHRASE='your-store-passphrase'
# or: export SIGNAL_E2E_PASSPHRASE_FILE=/path/to/secret

export SIGNAL_E2E_PEER_ACI='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'

SIGNAL_GO_E2E=1 task test:e2e
```

`task test:e2e` sets `SIGNAL_GO_E2E=1` and runs:

`go test -race -count=1 -tags=e2e -timeout=10m ./...`

## Tests and environment variables

| Test | Required env | Notes |
|------|----------------|-------|
| `TestE2E_Open` | `SIGNAL_E2E_STORE_DIR` + passphrase | Smoke: load account, connect websocket |
| `TestE2E_Send` | `SIGNAL_E2E_PEER_ACI` | Sends a unique plaintext to the peer |
| `TestE2E_Recv` | `SIGNAL_E2E_PEER_ACI` and/or `SIGNAL_E2E_RECV_CONTAINS` | **You** send a message from the peer first; test waits on `Events()` |
| `TestE2E_GroupManagement` | `SIGNAL_E2E_GROUP_MASTER_KEY` (64 hex chars) | `FetchGroup` + `SyncGroup`; optional `SIGNAL_E2E_GROUP_SEND=1` to post a message |
| `TestE2E_Link` | `SIGNAL_E2E_LINK=1` + empty store dir | Interactive link (logs URL); skips if `signal.db` exists |

Common optional variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `SIGNAL_E2E_TIMEOUT` | `3m` | Per-test context timeout (send/open/group) |
| `SIGNAL_E2E_RECV_TIMEOUT` | `5m` | How long `TestE2E_Recv` waits |
| `SIGNAL_E2E_PLAINTEXT` | off | Use `sqlstore.Open` (test-only plaintext DB) |
| `SIGNAL_E2E_SEND_PREFIX` | — | Prefix on outbound send body |
| `SIGNAL_E2E_EXPECT_BODY` | — | Alias for `SIGNAL_E2E_RECV_CONTAINS` |

## Suggested manual flow

1. **Open** — confirms store + websocket.
2. **Send** — run with `SIGNAL_E2E_PEER_ACI` set to your phone; confirm
   the message appears on the primary device.
3. **Recv** — from the phone, reply with a known substring; run with
   `SIGNAL_E2E_RECV_CONTAINS=that-substring` (and optionally
   `SIGNAL_E2E_PEER_ACI` to ignore other senders).
4. **Group** — copy the group master key (64 hex) from an inbound group
   message’s `GroupID` or your backup import; run
   `TestE2E_GroupManagement`. Set `SIGNAL_E2E_GROUP_SEND=1` only if you
   want an extra test message in that chat.

## Security

- Use a throwaway linked device and store directory.
- Do not commit passphrases or store paths with real credentials.
- E2e tests log message snippets at `t.Log` level only.
