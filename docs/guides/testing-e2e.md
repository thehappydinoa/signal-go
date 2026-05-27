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

### Create a passphrase file (skip the prompt)

To script linking and the e2e suite, write the passphrase to a file once and
point both the CLI and the test harness at it. The `/.signal-e2e/` directory is
gitignored, so a file there is never committed:

```sh
# One line, no leading/trailing spaces; a trailing newline is fine.
printf '%s\n' 'correct horse battery staple' > ./.signal-e2e/passphrase
chmod 600 ./.signal-e2e/passphrase
```

Use it from the CLI (replaces the interactive prompt) and from the harness:

```sh
./bin/signal-go link -store ./.signal-e2e -passphrase-file ./.signal-e2e/passphrase ...
export SIGNAL_E2E_PASSPHRASE_FILE="$PWD/.signal-e2e/passphrase"
```

The CLI (`-passphrase-file`) trims only trailing newlines/CR; the harness
(`SIGNAL_E2E_PASSPHRASE_FILE`) trims all surrounding whitespace. Keeping the
file to a single trimmed line makes the same file work for both. Never commit
it; keep it `chmod 600` and inside `/.signal-e2e/` (or outside the repo).

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

`task test:e2e` sets `SIGNAL_GO_E2E=1` and runs Open/Send/Recv/Group tests only
(`TestE2E_Link` and `TestE2E_GroupCreateAndChat` are excluded).
Interactive link uses `task test:e2e:link` (requires `SIGNAL_E2E_LINK=1`
and an empty store directory).

## Tests and environment variables

| Test | Required env | Notes |
|------|----------------|-------|
| `TestE2E_Open` | `SIGNAL_E2E_STORE_DIR` + passphrase | Smoke: load account, connect websocket |
| `TestE2E_Send` | `SIGNAL_E2E_PEER_ACI` | Sends a unique plaintext to the peer |
| `TestE2E_Recv` | `SIGNAL_E2E_PEER_ACI` and/or `SIGNAL_E2E_RECV_CONTAINS` | **You** send a message from the peer first; test waits on `Events()` |
| `TestE2E_GroupManagement` | `SIGNAL_E2E_GROUP_MASTER_KEY` (64 hex chars) | `FetchGroup` + `SyncGroup`; optional `SIGNAL_E2E_GROUP_SEND=1` to post a message |
| `TestE2E_GroupCreateAndChat` | `SIGNAL_E2E_GROUP_INTERACTIVE=1`, `SIGNAL_E2E_GROUP_INVITE_URL`, `SIGNAL_E2E_PEER_ACI` | Joins a peer-created invite-link group, verifies peer membership, sends bot message, then waits for peer reply containing a token |
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

## Interactive group create/join and bidirectional chat

Use this when you want an end-to-end check that a fresh group can be joined
and both sides can chat in it.

1. From the **peer account** (usually your phone), create a new Signal group.
2. In that group, enable an invite link and copy the full `https://signal.group/...` URL.
3. Export environment variables:

```sh
export SIGNAL_E2E_STORE_DIR=./.signal-e2e
export SIGNAL_E2E_PASSPHRASE='your-store-passphrase'
export SIGNAL_E2E_PEER_ACI='xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx'
export SIGNAL_E2E_GROUP_INVITE_URL='https://signal.group/#...'
```

4. Run the interactive task (uses `go test -v` so `t.Log` prompts are visible):

```sh
task test:e2e:group
```

5. Watch for the prompt that includes `manual step: ... reply ... containing ...`.
  From the peer account, send that reply in the group.
6. The test passes when it receives the peer's group message carrying the token.

This test intentionally requires operator interaction because creating a new
group on the peer side and producing an invite URL cannot be done from the
linked bot API in this repository today.

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

### `PreviewGroupJoin ... web.FetchGroupJoinInfo: web: HTTP 404 Not Found`

For `TestE2E_GroupCreateAndChat`, this almost always means the invite link in
`SIGNAL_E2E_GROUP_INVITE_URL` is no longer valid on Signal's side (revoked,
regenerated, disabled, or copied incorrectly).

Fix:

1. On the peer account, open the target group and re-enable invite link.
2. Copy a fresh full `https://signal.group/#...` URL.
3. Update `SIGNAL_E2E_GROUP_INVITE_URL` and rerun `task test:e2e:group`.

### `PreviewGroupJoin ... web.FetchGroupJoinInfo: web: HTTP 403 Forbidden`

For `TestE2E_GroupCreateAndChat`, this usually means the invite link exists
but is not currently usable by the bot account (for example after prior
join/leave/remove state).

Fix:

1. On the peer account, regenerate/copy a fresh full `https://signal.group/#...` URL.
2. In group settings, clear any pending request/ban state for the bot account.
3. Update `SIGNAL_E2E_GROUP_INVITE_URL` and rerun `task test:e2e:group`.

### How registration works (and where it can fail)

The provisioning websocket (`/v1/websocket/provisioning/`) is only a
rendezvous channel: it delivers the provisioning UUID and the encrypted
provision envelope, then Signal closes it. Registration is a **separate REST
call** — `PUT /v1/devices/link` over HTTPS — exactly as signal-cli /
libsignal-service-java do it. It is Basic-authenticated with the **e164
number** as the username and our freshly generated account password; the
provisioning code travels in the request body as `verificationCode`.

### `register: web: HTTP 403 Forbidden` on link

The server rejected `verificationCode` (validated as a device-linking token).
Usual causes: the QR was not approved in time (the code expired) or the
provisioning code did not round-trip from the decrypted envelope. Restart the
link flow and approve promptly.

### `register: web: HTTP 422 Unprocessable Entity` on link

The signed prekey / Kyber last-resort prekey signatures failed validation, or
a required device capability was missing. Check `buildLinkRequest` in
[`pkg/signal/link.go`](../../pkg/signal/link.go) and
[`DefaultCapabilities`](../../internal/web/devices.go).

### E2e store must be sqlstore

See [CLI link vs e2e test store](#cli-link-vs-e2e-test-store) above. The fsstore
from `signal-go link -store` is not read by `TestE2E_*`; use `TestE2E_Link` or
library `sqlstore` link into the same (empty) directory.

## Security

- Use a throwaway linked device and store directory.
- Do not commit passphrases or store paths with real credentials.
- E2e tests log message snippets at `t.Log` level only.
