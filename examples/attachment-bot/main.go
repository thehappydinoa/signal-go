// Command attachment-bot demonstrates attachment and utility helper scripts.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/thehappydinoa/signal-go/examples/internal/botexample"
	"github.com/thehappydinoa/signal-go/pkg/bot"
	"github.com/thehappydinoa/signal-go/pkg/signal"
)

func main() {
	os.Exit(botexample.Run(os.Args[1:], ".signal-attachment-bot", setup))
}

func setup(b *bot.Bot) error {
	b.OnCommand("help").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, strings.TrimSpace(`Commands:
  /txt <message>   reply with a text/plain attachment
  /read            send read receipt for your last message
  /typing          send typing started/stopped indicators`))
	})

	b.OnCommand("txt").Do(func(ctx context.Context, m *bot.Message, args []string) error {
		if len(args) == 0 {
			return m.Reply(ctx, "usage: /txt <message>")
		}
		payload := []byte(strings.Join(args, " ") + "\n")
		nameHint := fmt.Sprintf("note-%s.txt", m.Timestamp().UTC().Format("20060102-150405"))
		r := bytes.NewReader(append([]byte("# "+nameHint+"\n\n"), payload...))
		return m.ReplyAttachment(ctx, r, "text/plain")
	})

	b.OnCommand("read").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		if err := m.MarkRead(ctx); err != nil {
			return err
		}
		return m.Reply(ctx, "sent read receipt")
	})

	b.OnCommand("typing").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		if err := m.Typing(ctx, signal.TypingStarted); err != nil {
			return err
		}
		if err := m.Typing(ctx, signal.TypingStopped); err != nil {
			return err
		}
		return m.Reply(ctx, "typing indicator toggled")
	})

	b.OnAnyText().Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		if len(m.Attachments()) == 0 {
			return bot.ErrPass
		}
		first := m.Attachments()[0]
		return m.Reply(ctx, fmt.Sprintf("received %d attachment(s), first content-type=%s", len(m.Attachments()), first.ContentType))
	})

	return nil
}
