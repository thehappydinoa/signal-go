# Bot examples

These examples are intentionally small, script-style bot programs you can run and modify.

## Choose by script type

| Script type | Example |
|---|---|
| Command router (slash commands) | [`command-bot`](./command-bot/) |
| Multi-stage DM workflow | [`wizard-bot`](./wizard-bot/) |
| Group workflow (commands + reactions) | [`poll-bot`](./poll-bot/) |
| Middleware pipeline (global + per-handler) | [`middleware-bot`](./middleware-bot/) |
| Utility helpers (attachments/typing/receipts) | [`attachment-bot`](./attachment-bot/) |
| Minimal event loop with `pkg/signal` only | [`echo-bot`](./echo-bot/) |
| Group invite crawler (storage + live events) | [`group-crawler`](./group-crawler/) |

## Common run pattern

Most examples use the same flags:

- `-store` store directory
- `-passphrase-file` non-interactive passphrase source
- `-plaintext` test-only unencrypted store mode
- `-client`, `-user-agent` User-Agent selection
- `-auto-typing` send typing started/stopped around helper replies
- `-send-delay` delay helper replies (for example `1200ms`)

Run any example with a linked store:

```sh
go run ./examples/<name> -store ./.signal-bot
```

If needed, link first:

```sh
signal-go link -store ./.signal-bot
```
