package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sync"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// Options configures [Open]. AccountStore and SignalStores are required;
// everything else matches [signal.OpenOptions] semantics.
type Options struct {
	AccountStore account.Store
	SignalStores store.SignalStores

	ChatURL    string
	APIBaseURL string
	UserAgent  string
	Logger     *slog.Logger
}

// ErrPass tells the dispatcher "I didn't really handle this event; try
// the next matching handler." Handlers that intentionally do nothing
// should return nil (which stops dispatch).
var ErrPass = errors.New("bot: pass to next handler")

// ErrorHandler is invoked for non-nil handler errors that aren't
// [ErrPass]. If nil, errors are logged at WARN.
type ErrorHandler func(ctx context.Context, ev signal.Event, err error)

// Client is the narrow surface Bot consumes from [signal.Client]. The
// concrete implementation is *signal.Client; tests substitute a stub.
type Client interface {
	Events() <-chan signal.Event
	Send(ctx context.Context, recipient, text string) (signal.Receipt, error)
	Close() error
}

// Bot wraps a [Client] with registered dispatchers.
type Bot struct {
	cli Client
	log *slog.Logger

	mu           sync.RWMutex
	textHandlers []textHandler
	closed       bool

	onError ErrorHandler
}

// Open loads the persisted account, connects the chat ws, and returns a
// Bot ready for handler registration.
func Open(ctx context.Context, opts Options) (*Bot, error) {
	if opts.AccountStore == nil {
		return nil, errors.New("bot.Open: AccountStore is required")
	}
	if opts.SignalStores == nil {
		return nil, errors.New("bot.Open: SignalStores is required")
	}
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	cli, err := signal.Open(ctx, signal.OpenOptions{
		AccountStore: opts.AccountStore,
		SignalStores: opts.SignalStores,
		ChatURL:      opts.ChatURL,
		APIBaseURL:   opts.APIBaseURL,
		UserAgent:    opts.UserAgent,
		Logger:       log,
	})
	if err != nil {
		return nil, fmt.Errorf("bot.Open: %w", err)
	}
	return wrap(cli, log), nil
}

// Wrap returns a Bot driving an existing [Client]. Useful for tests
// that already constructed a client (or want to use a stub).
func Wrap(cli Client) *Bot { return wrap(cli, slog.Default()) }

func wrap(cli Client, log *slog.Logger) *Bot {
	return &Bot{cli: cli, log: log}
}

// OnError sets the callback invoked when a handler returns a non-nil
// error (other than [ErrPass]). Default: log at WARN.
func (b *Bot) OnError(h ErrorHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onError = h
}

// Close shuts down the underlying [signal.Client]. Subsequent calls to
// [Bot.Run] return immediately. Idempotent.
func (b *Bot) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()
	return b.cli.Close()
}

// Run pumps inbound events through the dispatcher until ctx is
// cancelled, the underlying client closes, or [Bot.Close] is called.
//
// Returns nil on graceful shutdown; non-nil on transport failure.
func (b *Bot) Run(ctx context.Context) error {
	events := b.cli.Events()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			b.dispatch(ctx, ev)
		}
	}
}

// Underlying returns the underlying [Client] for cases the bot
// dispatcher doesn't cover (sending unsolicited messages, inspecting
// the account, etc.).
func (b *Bot) Underlying() Client { return b.cli }

// dispatch routes one event through the registered handlers.
// Currently we only dispatch [*signal.MessageEvent]; other event
// types are silently ignored at this layer — bots that want them can
// reach down via [Bot.Client] and consume the raw Events channel.
func (b *Bot) dispatch(ctx context.Context, ev signal.Event) {
	msgEv, ok := ev.(*signal.MessageEvent)
	if !ok {
		return
	}
	m := &Message{
		event: msgEv,
		bot:   b,
	}
	b.mu.RLock()
	handlers := append([]textHandler(nil), b.textHandlers...)
	onErr := b.onError
	b.mu.RUnlock()

	for _, h := range handlers {
		matches, matched := h.matcher.match(msgEv, m)
		if !matched {
			continue
		}
		err := h.run(ctx, m, matches)
		if errors.Is(err, ErrPass) {
			continue
		}
		if err != nil {
			if onErr != nil {
				onErr(ctx, ev, err)
			} else {
				b.log.Warn("bot handler error", "err", err)
			}
		}
		return
	}
}

// Match is the public matcher type returned by the OnText / OnRegex /
// OnCommand registration helpers. Callers attach a handler via Do.
type Match struct {
	bot *Bot
	m   matcher
}

// Do registers handler for the current matcher. Handlers receive
// (ctx, *Message, []string-of-captures-or-args).
func (h Match) Do(handler HandlerFunc) {
	h.bot.mu.Lock()
	defer h.bot.mu.Unlock()
	h.bot.textHandlers = append(h.bot.textHandlers, textHandler{
		matcher: h.m,
		run:     handler,
	})
}

// HandlerFunc is the signature every dispatched handler implements.
// args is empty for OnText / OnPrefix; the regex / command capture
// groups for OnRegex / OnCommand.
type HandlerFunc func(ctx context.Context, m *Message, args []string) error

// OnText registers an exact-body match (case-sensitive).
func (b *Bot) OnText(text string) Match {
	return Match{bot: b, m: matcher{kind: matchExact, text: text}}
}

// OnPrefix registers a case-insensitive prefix match. args is empty.
func (b *Bot) OnPrefix(prefix string) Match {
	return Match{bot: b, m: matcher{kind: matchPrefix, text: prefix}}
}

// OnRegex registers a regex match. The handler's args slice contains
// the regex's capture groups (with index 0 = full match).
func (b *Bot) OnRegex(re *regexp.Regexp) Match {
	return Match{bot: b, m: matcher{kind: matchRegex, re: re}}
}

// OnCommand registers a "/command [args ...]" handler. Matches messages
// whose body starts with "/<name>" optionally followed by whitespace +
// arguments. args is the whitespace-split arguments after the command
// name (with the leading slash stripped).
func (b *Bot) OnCommand(name string) Match {
	return Match{bot: b, m: matcher{kind: matchCommand, text: name}}
}

type textHandler struct {
	matcher matcher
	run     HandlerFunc
}
