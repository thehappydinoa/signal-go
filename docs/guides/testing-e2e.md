# End-to-end testing (real Signal)

These tests talk to production `chat.signal.org` with a **real linked
device** store. They never run in ordinary CI or `go test ./...`; use
the `e2e` build tag and `SIGNAL_GO_E2E=1`.

See also [Testing](./testing.md) and [ADR 0006](../adr/0006-testing-strategy.md).

## Prerequisites

- Network egress to `chat.signal.org` (TLS uses the pinned Signal root;
  see [ADR 0034](../adr/0034-signal-tls-root-pinning.md)).
- `libsignal_ffi.a` built and CLI built (`task libsignal`, `task build`).
- A **linked** device store under a dedicated directory (recommended:
  `internal/store/sqlstore`, not the CLI’s `fsstore`-only link path).
- A **peer** for 1:1 tests: usually your phone’s ACI (primary account).
- For group tests: a **64-character hex** group master key for a group
  your linked device is already in.

## Build (first time)

From the repo root:

```sh
cd signal-go
source scripts/dev-env.sh   # Windows: after copying .env.example → .env
task libsignal
task build
```

On Windows use **Git Bash** (or MSYS2) so `source scripts/dev-env.sh` picks up
MinGW, `CGO_ENABLED=1`, and writable `TMP`/`TEMP` from `.env`. See
[Getting started](./getting-started.md#windows-git-bash--msys2).

`task build` writes the CLI to **`bin/signal-go`** (`bin/signal-go.exe` on Windows).

## One-time setup: link a device

Use a dedicated directory (for example `./.signal-e2e`). This creates a
**throwaway linked device** — remove it from Signal → Settings → Linked
devices when you are done.

```sh
mkdir -p ./.signal-e2e
./bin/signal-go link \
  -store ./.signal-e2e \
  -name "signal-go e2e" \
  -client desktop-linux \
  -timeout 10m
```

The command prompts for a **store passphrase** (encrypts credentials at rest).
Remember it — you need the same passphrase for any later `link`/`open` against
that directory.

Scan the QR code (or open the printed URL) from the phone:
**Signal → Settings → Linked devices → +**.

`-client desktop-linux` sends a Desktop-style `User-Agent`. If linking fails
with **HTTP 499**, the preset version may be too old for Signal’s servers — see
[Troubleshooting](#expected-handshake-response-status-code-101-but-got-499) below,
or omit `-client` to use the default `signal-go` identity.

### CLI link vs e2e test store

`signal-go link -store` uses **fsstore** (JSON files under the directory).
That is enough to verify linking and to use the CLI.

The **`TestE2E_*` harness expects sqlstore** — a `signal.db` under
`SIGNAL_E2E_STORE_DIR`. To run `task test:e2e`, link into sqlstore instead:

**Option A — interactive link test** (logs the URL; no QR in terminal):

```sh
export SIGNAL_E2E_STORE_DIR=./.signal-e2e
export SIGNAL_E2E_PASSPHRASE='same-passphrase-as-above'

SIGNAL_GO_E2E=1 SIGNAL_E2E_LINK=1 \
  go test -tags=e2e -timeout=15m -run TestE2E_Link ./pkg/signal/...
```

**Option B — library link** (QR via your own `OnURL` handler):

```go
db, _ := sqlstore.OpenWithPassphrase("./.signal-e2e", passphrase)
_, err := signal.Link(ctx, signal.LinkOptions{
    Store:        db,
    SignalStores: db.SignalStores(),
    OnURL:        printURL, // QR / URL for the phone
})
```

If you already linked with the CLI into `./.signal-e2e`, use a **fresh directory**
for sqlstore (or delete the fsstore files) and link again with option A or B.

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

## Troubleshooting

### `expected handshake response status code 101 but got 499`

Signal returns **HTTP 499** when the client looks like an **expired upstream app**
(see Signal Desktop’s `AppExpired` handling). This often happens if you linked with
`-client desktop-windows` (or another desktop/android/ios preset) while the preset’s
**snapshot version** in [`internal/web/useragent`](../../internal/web/useragent/useragent.go)
is older than the minimum version Signal accepts.

**Fix (pick one):**

1. **Use the default client identity** — omit `-client` so `User-Agent` / `X-Signal-Agent`
   are `signal-go` (recommended for development and e2e).
2. **Override the version** — e.g.
   `signal-go link -store ./.signal-e2e -client desktop-windows -user-agent 'Signal-Desktop/8.10.0 Windows 10'`
3. **Bump the preset** in-tree if you maintain the snapshots (same file as above).

TLS to `chat.signal.org` must succeed first (pinned Signal root; see
[ADR 0034](../adr/0034-signal-tls-root-pinning.md)). A 499 means TLS worked but the
server rejected the upgrade.

### `register: ... ws.Client: closed during request`

Signal closes the provisioning websocket shortly after delivering the link
envelope. Registration uses a **separate** unauthenticated service websocket;
rebuild with a current signal-go if you still see this after pulling the fix.

While waiting for you to scan the QR, the client sends periodic WebSocket pings
to reduce idle timeouts.

### `register: web: HTTP 404 Not Found` on link

Registration was sent on the **provisioning** websocket, which does not route
`/v1/devices/link`. Rebuild — link must use `wss://chat.signal.org/v1/websocket/`.

### `register: web: HTTP 401 Unauthorized` on link

Provisioning succeeded (you saw the QR and got a provision envelope), but
`PUT /v1/devices/link` rejected the request. Signal expects HTTP Basic auth
with **username = the account phone number** (E.164 from the provision message)
and **password = the new device password** you generate locally. The signed
link token from the primary belongs only in the JSON `verificationCode` field
(`ProvisionMessage.provisioningCode`), not in the Basic username. Using the
token as the username fails (tokens contain `:`) and returns 401.

Rebuild with a current signal-go if you still see this after pulling the fix.

### `register: web: HTTP 498 : {"message":"use websockets"}`

Provisioning succeeded (you saw the QR) but registration still used HTTP.
Rebuild after updating signal-go — `PUT /v1/devices/link` must run on the
**service websocket** (`/v1/websocket/`), not the provisioning socket and not
`https://chat.signal.org/v1/devices/link` REST.

```sh
task build
./bin/signal-go link -store ./.signal-e2e ...
```

### E2e store must be sqlstore

See [CLI link vs e2e test store](#cli-link-vs-e2e-test-store) above. The fsstore
from `signal-go link -store` is not read by `TestE2E_*`; use `TestE2E_Link` or
library `sqlstore` link into the same (empty) directory.

## Security

- Use a throwaway linked device and store directory.
- Do not commit passphrases or store paths with real credentials.
- E2e tests log message snippets at `t.Log` level only.
