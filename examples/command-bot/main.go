// Command command-bot demonstrates slash-command style bot scripts.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/thehappydinoa/signal-go/examples/internal/botexample"
	"github.com/thehappydinoa/signal-go/pkg/bot"
)

func main() {
	os.Exit(botexample.Run(os.Args[1:], ".signal-command-bot", setup))
}

func setup(b *bot.Bot) error {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	b.OnCommand("help").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, strings.TrimSpace(`Commands:
  /help              show this message
  /ping              health check
  /roll [max]        random number from 1..max (default 6)
  /time              current UTC time`))
	})

	b.OnCommand("ping").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, "pong")
	})

	b.OnCommand("time").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, time.Now().UTC().Format(time.RFC3339))
	})

	b.OnCommand("roll").Do(func(ctx context.Context, m *bot.Message, args []string) error {
		maxRoll := 6
		if len(args) > 0 {
			n, err := strconv.Atoi(args[0])
			if err != nil || n < 2 || n > 1000 {
				return m.Reply(ctx, "usage: /roll [max], where max is 2..1000")
			}
			maxRoll = n
		}
		value := 1 + rng.Intn(maxRoll)
		return m.Reply(ctx, fmt.Sprintf("rolled %d (1..%d)", value, maxRoll))
	})

	b.OnPrefix("/").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, "unknown command; try /help")
	})

	return nil
}
