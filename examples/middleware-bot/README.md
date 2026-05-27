# middleware-bot example

This example shows middleware-based bot scripts.

## What it demonstrates

- Global middleware with `Bot.Use`
- Per-handler middleware with `Match.Use`
- Panic recovery middleware
- Structured request logging middleware
- Sender rate-limiting middleware
- Protected admin command middleware (`BOT_ADMIN_ACI`)

## Run

```sh
go run ./examples/middleware-bot -store ./.signal-bot
```

Optional admin sender lock:

```sh
BOT_ADMIN_ACI=<aci-uuid> go run ./examples/middleware-bot -store ./.signal-bot
```

Try:

```text
/help
/whoami
/burst
/admin status
```
