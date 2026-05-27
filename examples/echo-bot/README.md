# echo-bot example

This folder contains a tiny, end-to-end example program that uses
`pkg/signal` directly to:

- Link a **secondary Signal device** (QR scan) into a local SQLite-backed store
- Connect to the authenticated chat websocket
- Echo-reply to inbound **1:1** text messages

Unlike `bin/signal-go link` (which currently persists only the account record),
this example uses `internal/store/sqlstore` so the on-disk store includes the
libsignal session/identity/prekey state required for receive + send.

## Build prerequisites

You must have built the pinned `libsignal_ffi.a` once:

```sh
task libsignal
```

## Link

```sh
go run ./examples/echo-bot link -store ./.signal-bot
```

The program prompts for a store passphrase and renders a terminal QR code.

## Run

```sh
go run ./examples/echo-bot run -store ./.signal-bot
```

Send a 1:1 message to the linked account from another Signal account/device;
the bot replies with `echo: <your message>`.

## Non-interactive passphrase

```sh
go run ./examples/echo-bot link -store ./.signal-bot -passphrase-file /run/secrets/store-passphrase
go run ./examples/echo-bot run  -store ./.signal-bot -passphrase-file /run/secrets/store-passphrase
```

## Notes / limitations

- The example **does not** reply in groups (it logs and skips group messages).
- If your network blocks Signal, the websocket connect will fail.
- Don’t use `-plaintext` for real accounts; it disables at-rest encryption.

