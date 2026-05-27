# echo-bot example

This folder contains a tiny, end-to-end example program that uses
`pkg/signal` directly to:

- Link a **secondary Signal device** (QR scan) into a local SQLite-backed store
- Connect to the authenticated chat websocket
- Echo-reply to inbound **1:1** text messages

Uses `internal/store/sqlstore` (`signal.db` + optional `kdf.json`) so the store
includes libsignal session/identity/prekey state required for receive + send.
The `signal-go link` CLI uses the same sqlstore layout.

## Build prerequisites

You must have built the pinned `libsignal_ffi.a` once:

```sh
task libsignal
```

## Link

Link from the **bot account’s phone** (the number you want people to message):

```sh
go run ./examples/echo-bot link -store ./.signal-bot
```

The program prompts for a store passphrase and renders a terminal QR code.

## Run

```sh
go run ./examples/echo-bot run -store ./.signal-bot
```

From your **primary** (or any other) Signal account, send a 1:1 message to the
**bot account’s phone number**. The bot replies with `echo: <your message>`.

## Non-interactive passphrase

```sh
go run ./examples/echo-bot link -store ./.signal-bot -passphrase-file /run/secrets/store-passphrase
go run ./examples/echo-bot run  -store ./.signal-bot -passphrase-file /run/secrets/store-passphrase
```

## Troubleshooting: bot logs “replied” but primary sees nothing

The REST API can return success while the peer’s phone still does not show the
message. Common causes:

1. **Sealed sender vs basic auth** — By default the bot tries sealed sender when
   it has your profile key from the inbound message. If your profile restricts
   unidentified access, delivery can fail silently. The bot now calls
   `FetchProfile` and falls back to basic auth when needed. You can force basic
   auth:

   ```sh
   go run ./examples/echo-bot run -store ./.signal-bot -basic-auth
   ```

2. **Wrong store / wrong account** — The store must be linked with
   `echo-bot link` (sqlstore). If `signal.db` is missing but `account.enc`
   exists, you linked with the CLI instead.

3. **Message requests** — On the primary phone, check **Message requests** and
   the chat with the **bot’s number** (not “Note to Self”).

4. **Verify outbound send from the same store** — From the bot store, run the
   e2e send test to your primary ACI:

   ```sh
   export SIGNAL_E2E_STORE_DIR=./.signal-bot
   export SIGNAL_E2E_PASSPHRASE_FILE=./.signal-bot/passphrase
   export SIGNAL_E2E_PEER_ACI='<your-primary-aci>'
   SIGNAL_GO_E2E=1 go test -tags=e2e -run TestE2E_Send ./pkg/signal/...
   ```

   If that message does not appear on the primary either, the issue is store /
   linking / network — not the echo loop.

## Notes / limitations

- The example **does not** reply in groups (it logs and skips group messages).
- If your network blocks Signal, the websocket connect will fail.
- Don’t use `-plaintext` for real accounts; it disables at-rest encryption.

## Profiling (long-running soak)

`-memprofile` and `-cpuprofile` are **echo-bot flags**, not `go run` flags. Put them
after `run`:

```sh
go run ./examples/echo-bot run \
  -store ./.signal-e2e \
  -passphrase-file ./.signal-e2e/passphrase \
  -memprofile=mem.prof \
  -cpuprofile=cpu.prof
```

Exercise the bot (send a few 1:1 messages), then press Ctrl+C. You should see
`profiles written` in the log.

```sh
go tool pprof -http=:8080 mem.prof   # heap
go tool pprof -http=:8081 cpu.prof   # CPU
```

More detail: [docs/guides/profiling.md](../../docs/guides/profiling.md).
