# command-bot example

This example shows a command-router style bot script built on `pkg/bot`.

## What it demonstrates

- `OnCommand` for slash commands (`/help`, `/ping`, `/time`, `/roll`)
- `OnPrefix("/")` fallback for unknown commands
- Argument parsing and validation in handlers

## Run

```sh
go run ./examples/command-bot -store ./.signal-bot
```

If the store is not linked yet:

```sh
signal-go link -store ./.signal-bot
```
