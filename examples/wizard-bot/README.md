# wizard-bot example

This example shows a multi-stage conversation flow (wizard).

## What it demonstrates

- `Bot.Wizard` with named steps
- Stage-gated message handling
- Per-conversation state via `Message.Convo()`
- Cancellation via `/cancel` while any stage is active

## Run

```sh
go run ./examples/wizard-bot -store ./.signal-bot
```

Then DM the bot:

```text
/signup
```

Commands:

```text
/signup
/cancel
/help
```

If you send an unknown slash command (or `/signup` with extra args), the bot
replies with a usage/help block.
