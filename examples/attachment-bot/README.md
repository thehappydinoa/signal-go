# attachment-bot example

This example shows utility-style scripts for attachment and message helpers.

## What it demonstrates

- `ReplyAttachment` for outbound files
- `MarkRead` receipts
- Typing indicators (`TypingStarted` / `TypingStopped`)
- Inspecting inbound attachment metadata

## Run

```sh
go run ./examples/attachment-bot -store ./.signal-bot
```

Try:

```text
/help
/txt hello world
/read
/typing
```
