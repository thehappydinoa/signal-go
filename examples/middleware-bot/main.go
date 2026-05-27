// Command middleware-bot demonstrates middleware composition patterns.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/thehappydinoa/signal-go/examples/internal/botexample"
	"github.com/thehappydinoa/signal-go/pkg/bot"
)

func main() {
	os.Exit(botexample.Run(os.Args[1:], ".signal-middleware-bot", setup))
}

func setup(b *bot.Bot) error {
	limiter := newSenderLimiter(1200 * time.Millisecond)

	b.Use(recoverMiddleware())
	b.Use(loggingMiddleware())
	b.Use(limiter.middleware())

	adminACI := strings.TrimSpace(os.Getenv("BOT_ADMIN_ACI"))

	b.OnCommand("help").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, strings.TrimSpace(`Commands:
  /help              list commands
  /whoami            show sender and thread details
  /burst             triggers the rate-limit middleware
  /admin status      protected command (requires BOT_ADMIN_ACI)`))
	})

	b.OnCommand("whoami").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		thread := "dm"
		if m.IsGroup() {
			thread = "group:" + m.GroupID()
		}
		return m.Reply(ctx, fmt.Sprintf("sender=%s thread=%s stage=%q", m.Sender(), thread, m.Convo().Stage()))
	})

	b.OnCommand("burst").Do(func(ctx context.Context, m *bot.Message, _ []string) error {
		return m.Reply(ctx, "send /burst repeatedly to see global rate limiting")
	})

	b.OnCommand("admin").
		Use(requireAdminMiddleware(adminACI)).
		Do(func(ctx context.Context, m *bot.Message, args []string) error {
			if len(args) == 0 || strings.ToLower(args[0]) != "status" {
				return m.Reply(ctx, "usage: /admin status")
			}
			return m.Reply(ctx, "admin status: ok")
		})

	return nil
}

func recoverMiddleware() bot.MiddlewareFunc {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, m *bot.Message, args []string) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()
			return next(ctx, m, args)
		}
	}
}

func loggingMiddleware() bot.MiddlewareFunc {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, m *bot.Message, args []string) error {
			slog.Info("incoming message",
				"sender", m.Sender(),
				"group", m.GroupID(),
				"body", truncate(m.Body(), 120),
			)
			return next(ctx, m, args)
		}
	}
}

type senderLimiter struct {
	mu           sync.Mutex
	lastBySender map[string]time.Time
	minGap       time.Duration
}

func newSenderLimiter(minGap time.Duration) *senderLimiter {
	return &senderLimiter{
		lastBySender: make(map[string]time.Time),
		minGap:       minGap,
	}
}

func (l *senderLimiter) middleware() bot.MiddlewareFunc {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, m *bot.Message, args []string) error {
			now := time.Now()
			l.mu.Lock()
			last := l.lastBySender[m.Sender()]
			if now.Sub(last) < l.minGap {
				l.mu.Unlock()
				return m.Reply(ctx, "rate limit: wait a second before sending another command")
			}
			l.lastBySender[m.Sender()] = now
			l.mu.Unlock()
			return next(ctx, m, args)
		}
	}
}

func requireAdminMiddleware(adminACI string) bot.MiddlewareFunc {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, m *bot.Message, args []string) error {
			if adminACI == "" {
				return m.Reply(ctx, "admin middleware not configured; set BOT_ADMIN_ACI")
			}
			if m.Sender() != adminACI {
				return m.Reply(ctx, "forbidden")
			}
			return next(ctx, m, args)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
