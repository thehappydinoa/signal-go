// Package bot is a thin dispatch layer on top of [pkg/signal] that
// makes Signal bots roughly as ergonomic as Telegram bot or Slack Bolt.
//
// A Bot wraps a [signal.Client], routes inbound text / receipt / typing
// events through registered handlers, and exposes Reply / React helpers
// that send back through the same Client.
//
// Use:
//
//	b, err := bot.Open(ctx, bot.Options{AccountStore: …, SignalStores: …})
//	if err != nil { return err }
//	defer b.Close()
//
//	b.OnText("ping").Do(func(ctx context.Context, m *bot.Message) error {
//	    return m.Reply(ctx, "pong")
//	})
//	b.OnCommand("weather").Do(func(ctx context.Context, m *bot.Message, args []string) error {
//	    return m.Reply(ctx, "(stub)")
//	})
//	b.OnRegex(regexp.MustCompile(`(?i)hello`)).Do(func(ctx context.Context, m *bot.Message, _ []string) error {
//	    return m.Reply(ctx, "hi 👋")
//	})
//
//	if err := b.Run(ctx); err != nil { return err }
//
// Handlers are evaluated in registration order; the first match wins.
// Returning nil from a handler stops dispatch for that event; returning
// [ErrPass] continues to the next handler.
//
// # Humanized helper replies (optional)
//
// Set [Options.AutoTypingIndicators] and/or [Options.SendDelay] to make
// [Message.Reply] and [Message.ReplyAttachment] feel less instantaneous.
// With typing indicators enabled, helper replies send TypingStarted before
// work and TypingStopped after send/abort.
//
// # Conversation state
//
// Each conversation (sender ACI plus optional group ID) has a small
// per-key/value store accessed via [Message.Convo]. Stages provide a
// lightweight FSM: write the next step with [Convo.SetStage], gate
// the follow-up handlers with [Match.Stage] / [Match.AnyStage], and
// clear the flow with [Convo.ClearStage] or [Convo.Clear]. The default
// backend is in-memory; supply [Options.ConvoStore] to persist state
// across restarts.
//
// This package deliberately wraps the public [signal.Client] only — it
// does not reach into protocol internals. That lets bots target the
// same surface library consumers would, and keeps the dependency arrow
// pointing the right way.
package bot
