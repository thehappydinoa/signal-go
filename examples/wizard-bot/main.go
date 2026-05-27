// Command wizard-bot demonstrates multi-stage conversation scripts.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/thehappydinoa/signal-go/examples/internal/botexample"
	"github.com/thehappydinoa/signal-go/pkg/bot"
)

func main() {
	os.Exit(botexample.Run(os.Args[1:], ".signal-wizard-bot", setup))
}

func setup(b *bot.Bot) error {
	signup := b.Wizard("signup")
	usage := strings.TrimSpace(`Commands:
  /signup         start signup wizard
  /cancel         cancel active wizard
  /help           show this help

Wizard flow:
  1) send your name
  2) send your timezone (example: UTC, America/New_York)
  3) confirm with yes/no`)

	signup.Step("name", func(ctx context.Context, m *bot.Message, _ []string) error {
		name := strings.TrimSpace(m.Body())
		if name == "" {
			// Ignore keepalive/sync events while a stage is active.
			return bot.ErrPass
		}
		if strings.HasPrefix(name, "/") {
			return m.Reply(ctx, usage)
		}
		if name == "" {
			return m.Reply(ctx, "Please send your name.")
		}
		m.Convo().Set("name", name)
		signup.Advance(m, "timezone")
		return m.Reply(ctx, "What timezone are you in? (example: UTC, America/New_York)")
	})

	signup.Step("timezone", func(ctx context.Context, m *bot.Message, _ []string) error {
		tz := strings.TrimSpace(m.Body())
		if tz == "" {
			// Ignore keepalive/sync events while a stage is active.
			return bot.ErrPass
		}
		if strings.HasPrefix(tz, "/") {
			return m.Reply(ctx, usage)
		}
		if tz == "" {
			return m.Reply(ctx, "Please send a timezone.")
		}
		m.Convo().Set("timezone", tz)
		signup.Advance(m, "confirm")
		name, _ := m.Convo().Get("name")
		return m.Reply(ctx, fmt.Sprintf("Confirm signup? name=%q timezone=%q (reply yes/no)", name, tz))
	})

	signup.Step("confirm", func(ctx context.Context, m *bot.Message, _ []string) error {
		answer := strings.ToLower(strings.TrimSpace(m.Body()))
		if answer == "" {
			// Ignore keepalive/sync events while a stage is active.
			return bot.ErrPass
		}
		if strings.HasPrefix(answer, "/") {
			return m.Reply(ctx, usage)
		}
		if answer == "yes" || answer == "y" {
			name, _ := m.Convo().Get("name")
			tz, _ := m.Convo().Get("timezone")
			signup.Clear(m)
			m.Convo().Delete("name")
			m.Convo().Delete("timezone")
			return m.Reply(ctx, fmt.Sprintf("Signup complete. Welcome %s (%s).", name, tz))
		}
		if answer == "no" || answer == "n" {
			signup.Clear(m)
			m.Convo().Delete("name")
			m.Convo().Delete("timezone")
			return m.Reply(ctx, "Cancelled. Send /signup to start again.")
		}
		return m.Reply(ctx, "Please reply yes or no.")
	})
	signup.Register()

	b.OnCommand("signup").DM().Do(func(ctx context.Context, m *bot.Message, args []string) error {
		if len(args) > 0 {
			return m.Reply(ctx, "usage:\n"+usage)
		}
		if err := signup.Begin(ctx, m, "name"); err != nil {
			return err
		}
		return m.Reply(ctx, "Welcome. What is your name?")
	})

	b.OnCommand("help").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, usage)
	})

	b.OnCommand("cancel").AnyStage().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		signup.Clear(m)
		m.Convo().Delete("name")
		m.Convo().Delete("timezone")
		return m.Reply(ctx, "Current workflow cancelled.")
	})

	b.OnPrefix("/").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		trimmed := strings.TrimSpace(m.Body())
		if trimmed == "" {
			return bot.ErrPass
		}
		parts := strings.Fields(trimmed)
		if len(parts) == 0 {
			return bot.ErrPass
		}
		cmd := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
		switch cmd {
		case "signup", "cancel", "help":
			return bot.ErrPass
		default:
			return m.Reply(ctx, "unknown command\n"+usage)
		}
	})

	return nil
}
