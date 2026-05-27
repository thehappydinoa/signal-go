package botexample

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	osSignal "os/signal"
	"syscall"

	"github.com/thehappydinoa/signal-go/internal/cliargs"
	"github.com/thehappydinoa/signal-go/pkg/bot"
	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// SetupFunc registers handlers on a ready bot.
type SetupFunc func(b *bot.Bot) error

// Run opens a linked account store, configures a bot, registers handlers,
// and blocks until interrupted.
func Run(args []string, defaultStore string, setup SetupFunc) int {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, defaultStore)
	clientPreset, userAgent := cliargs.ClientBind(fs)
	autoTyping := fs.Bool("auto-typing", false, "send typing started/stopped around bot replies")
	sendDelay := fs.Duration("send-delay", 0, "delay before bot replies (e.g. 1200ms)")
	_ = fs.Parse(args)

	client, err := cliargs.ClientFromFlags(clientPreset, userAgent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	store, err := cliargs.StoreFromFlags(storeDir, passphraseFile, plaintext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	db, err := store.OpenSQLStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := signalContext(context.Background())
	defer cancel()

	b, err := bot.Open(ctx, bot.Options{
		AccountStore:         db,
		SignalStores:         db.SignalStores(),
		ClientProfile:        client.Profile,
		UserAgent:            client.UserAgent,
		AutoSyncGroupUpdates: true,
		AutoSyncStorage:      true,
		AutoTypingIndicators: *autoTyping,
		SendDelay:            *sendDelay,
		Logger:               slog.Default(),
	})
	if err != nil {
		if errors.Is(err, signal.ErrNotLinked) {
			fmt.Fprintln(os.Stderr, "store is not linked yet; run: signal-go link -store <path>")
			return 1
		}
		fmt.Fprintf(os.Stderr, "open bot: %v\n", err)
		return 1
	}
	defer func() { _ = b.Close() }()

	b.Use(bot.RateLimitRetryMiddleware(bot.RateLimitRetryOptions{MaxRetries: 1}))

	if err := setup(b); err != nil {
		fmt.Fprintf(os.Stderr, "setup bot: %v\n", err)
		return 1
	}

	slog.Info("bot connected", "store", store.Dir)
	if err := b.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "run bot: %v\n", err)
		return 1
	}
	return 0
}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 1)
	osSignal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		osSignal.Stop(sigCh)
		close(sigCh)
	}()
	return ctx, cancel
}
