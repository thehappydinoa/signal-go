# poll-bot example

This example shows a group workflow script driven by commands plus reactions.

## What it demonstrates

- Group-only command handlers (`.Group()`)
- Reaction handlers (`OnReaction("👍")`, `OnReaction("👎")`)
- Shared, group-scoped state in `ConvoStore`
- Open/close workflow state for a running poll

## Run

```sh
go run ./examples/poll-bot -store ./.signal-bot
```

In a group:

```text
/poll open Lunch at 12?
/poll status
/poll close
```
