package bot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sync"
	"time"

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
	// ClientProfile selects a realistic User-Agent preset when UserAgent is
	// empty. Default: [signal.UserAgentSignalGo].
	ClientProfile    signal.UserAgentProfile
	UserAgentOptions signal.UserAgentOptions
	UserAgent        string
	Logger           *slog.Logger

	// ConvoStore persists per-conversation state surfaced via
	// [Bot.Convo] and [Message.Convo]. Defaults to an in-memory
	// store; bots that need state to survive process restarts can
	// supply a backing implementation.
	ConvoStore ConvoStore

	// AutoSyncGroupUpdates passes through to [signal.OpenOptions] so
	// inbound group change notifications trigger background log sync.
	AutoSyncGroupUpdates bool

	// AutoSyncStorage passes through to [signal.OpenOptions] so linked
	// devices requesting storage-manifest sync trigger background pull.
	AutoSyncStorage bool

	// AutoTypingIndicators sends TypingStarted before helper replies and
	// TypingStopped after send/abort in [Message.Reply] and
	// [Message.ReplyAttachment]. Default: false.
	AutoTypingIndicators bool

	// SendDelay waits before helper replies in [Message.Reply] and
	// [Message.ReplyAttachment]. Useful for more human-like pacing.
	// Default: 0 (disabled).
	SendDelay time.Duration
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
	SendGroup(ctx context.Context, masterKey []byte, text string) (signal.Receipt, error)
	SendGroupReaction(ctx context.Context, masterKey []byte, emoji, targetAuthor string, targetTimestamp time.Time, remove bool) (signal.Receipt, error)
	SendGroupTyping(ctx context.Context, masterKey []byte, action signal.TypingAction) (signal.Receipt, error)
	SendReceipt(ctx context.Context, recipient string, kind signal.ReceiptType, timestamps []time.Time) (signal.Receipt, error)
	SendTyping(ctx context.Context, recipient string, action signal.TypingAction) (signal.Receipt, error)
	SendReaction(ctx context.Context, recipient, emoji, targetAuthor string, targetTimestamp time.Time, remove bool) (signal.Receipt, error)
	SendAttachment(ctx context.Context, recipient string, r io.Reader, opts signal.SendAttachmentOptions) (signal.Receipt, error)
	SendGroupAttachment(ctx context.Context, masterKey []byte, r io.Reader, opts signal.SendAttachmentOptions) (signal.Receipt, error)
	Close() error
}

// MiddlewareFunc wraps a [HandlerFunc]. The outer function may inspect or
// modify the message, add context values, or short-circuit the call.
// The canonical pattern mirrors [net/http.Handler]:
//
//	func LogMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
//	    return func(ctx context.Context, m *bot.Message, args []string) error {
//	        slog.Info("message", "sender", m.Sender(), "body", m.Body())
//	        return next(ctx, m, args)
//	    }
//	}
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// Bot wraps a [Client] with registered dispatchers.
type Bot struct {
	cli   Client
	log   *slog.Logger
	convo *Conversations

	autoTypingIndicators bool
	sendDelay            time.Duration

	mu                  sync.RWMutex
	middleware          []MiddlewareFunc
	textHandlers        []textHandler
	reactionHandlers    []reactionHandler
	editHandlers        []editHandler
	groupUpdateHandlers []groupUpdateHandler
	closed              bool

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
		AccountStore:         opts.AccountStore,
		SignalStores:         opts.SignalStores,
		ChatURL:              opts.ChatURL,
		APIBaseURL:           opts.APIBaseURL,
		ClientProfile:        opts.ClientProfile,
		UserAgentOptions:     opts.UserAgentOptions,
		UserAgent:            opts.UserAgent,
		Logger:               log,
		AutoSyncGroupUpdates: opts.AutoSyncGroupUpdates,
		AutoSyncStorage:      opts.AutoSyncStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("bot.Open: %w", err)
	}
	return wrap(cli, log, opts.ConvoStore, opts.AutoTypingIndicators, opts.SendDelay), nil
}

// Wrap returns a Bot driving an existing [Client]. Useful for tests
// that already constructed a client (or want to use a stub). The
// returned Bot uses an in-memory [ConvoStore]; supply a custom one via
// [WrapWithOptions] if persistence is needed.
func Wrap(cli Client) *Bot { return wrap(cli, slog.Default(), nil, false, 0) }

// WrapOptions tweaks the [Bot] returned by [WrapWithOptions]. All
// fields are optional.
type WrapOptions struct {
	Logger               *slog.Logger
	ConvoStore           ConvoStore
	AutoTypingIndicators bool
	SendDelay            time.Duration
}

// WrapWithOptions returns a Bot driving an existing [Client] with
// non-default Logger / ConvoStore overrides. Useful for tests that
// want to inject a persistent ConvoStore.
func WrapWithOptions(cli Client, opts WrapOptions) *Bot {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}
	return wrap(cli, log, opts.ConvoStore, opts.AutoTypingIndicators, opts.SendDelay)
}

func wrap(cli Client, log *slog.Logger, cs ConvoStore, autoTypingIndicators bool, sendDelay time.Duration) *Bot {
	if cs == nil {
		cs = NewMemoryConvoStore()
	}
	return &Bot{
		cli:                  cli,
		log:                  log,
		convo:                &Conversations{store: cs},
		autoTypingIndicators: autoTypingIndicators,
		sendDelay:            sendDelay,
	}
}

// Use registers a global middleware that wraps every handler. Middleware
// is applied outermost-first: the first Use call produces the outermost
// wrapper in the call chain.
//
// Global middleware runs before any per-handler middleware registered via
// [Match.Use].
func (b *Bot) Use(mw MiddlewareFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.middleware = append(b.middleware, mw)
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

// Convo returns the bot-wide [Conversations] handle. Use
// [Conversations.For] to obtain a per-key [Convo], or call
// [Message.Convo] inside a handler to get one already scoped to the
// inbound message.
func (b *Bot) Convo() *Conversations { return b.convo }

// dispatch routes one event through the registered handlers. Bot
// dispatches [*signal.MessageEvent], [*signal.ReactionEvent],
// [*signal.EditMessageEvent], and [*signal.GroupUpdateEvent]; other
// event types are silently ignored at this layer — bots that want them
// can reach down via [Bot.Underlying] and consume the raw Events channel.
func (b *Bot) dispatch(ctx context.Context, ev signal.Event) {
	switch e := ev.(type) {
	case *signal.MessageEvent:
		b.dispatchMessage(ctx, e)
	case *signal.ReactionEvent:
		b.dispatchReaction(ctx, e)
	case *signal.EditMessageEvent:
		b.dispatchEdit(ctx, e)
	case *signal.GroupUpdateEvent:
		b.dispatchGroupUpdate(ctx, e)
	}
}

func (b *Bot) dispatchGroupUpdate(ctx context.Context, ev *signal.GroupUpdateEvent) {
	u := &GroupUpdate{event: ev, bot: b}
	b.mu.RLock()
	handlers := append([]groupUpdateHandler(nil), b.groupUpdateHandlers...)
	onErr := b.onError
	b.mu.RUnlock()

	for _, h := range handlers {
		err := h.run(ctx, u)
		if errors.Is(err, ErrPass) {
			continue
		}
		if err != nil {
			b.handleErr(ctx, ev, err, onErr)
		}
		return
	}
}

func (b *Bot) dispatchMessage(ctx context.Context, ev *signal.MessageEvent) {
	m := &Message{
		event: ev,
		bot:   b,
	}
	b.mu.RLock()
	handlers := append([]textHandler(nil), b.textHandlers...)
	globalMW := append([]MiddlewareFunc(nil), b.middleware...)
	onErr := b.onError
	b.mu.RUnlock()

	for _, h := range handlers {
		matches, matched := h.matcher.match(ev, m)
		if !matched {
			continue
		}
		run := applyMiddleware(h.run, h.middleware) // per-handler (inner)
		run = applyMiddleware(run, globalMW)        // global (outer)
		err := run(ctx, m, matches)
		if errors.Is(err, ErrPass) {
			continue
		}
		if err != nil {
			b.handleErr(ctx, ev, err, onErr)
		}
		return
	}
}

func (b *Bot) dispatchReaction(ctx context.Context, ev *signal.ReactionEvent) {
	r := &Reaction{event: ev, bot: b}
	b.mu.RLock()
	handlers := append([]reactionHandler(nil), b.reactionHandlers...)
	onErr := b.onError
	b.mu.RUnlock()

	for _, h := range handlers {
		if !h.matcher.match(ev) {
			continue
		}
		err := h.run(ctx, r)
		if errors.Is(err, ErrPass) {
			continue
		}
		if err != nil {
			b.handleErr(ctx, ev, err, onErr)
		}
		return
	}
}

func (b *Bot) dispatchEdit(ctx context.Context, ev *signal.EditMessageEvent) {
	e := &Edit{event: ev, bot: b}
	b.mu.RLock()
	handlers := append([]editHandler(nil), b.editHandlers...)
	onErr := b.onError
	b.mu.RUnlock()

	for _, h := range handlers {
		if !h.matcher.match(ev) {
			continue
		}
		err := h.run(ctx, e)
		if errors.Is(err, ErrPass) {
			continue
		}
		if err != nil {
			b.handleErr(ctx, ev, err, onErr)
		}
		return
	}
}

func (b *Bot) handleErr(ctx context.Context, ev signal.Event, err error, onErr ErrorHandler) {
	if onErr != nil {
		onErr(ctx, ev, err)
	} else {
		b.log.Warn("bot handler error", "err", err)
	}
}

// applyMiddleware wraps h in the provided middleware chain. mws[0] is the
// outermost wrapper; mws[len-1] wraps h directly.
func applyMiddleware(h HandlerFunc, mws []MiddlewareFunc) HandlerFunc {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Match is the public matcher type returned by the OnText / OnRegex /
// OnCommand registration helpers. Callers attach a handler via Do, optionally
// narrowing scope with DM / Group / From and adding middleware via Use.
type Match struct {
	bot *Bot
	m   matcher
	mw  []MiddlewareFunc
}

// DM narrows the match to direct (non-group) messages only.
func (h Match) DM() Match {
	h.m.dmOnly = true
	return h
}

// Group narrows the match to group-thread messages only.
func (h Match) Group() Match {
	h.m.groupOnly = true
	return h
}

// From narrows the match to messages whose sender ACI equals aci.
func (h Match) From(aci string) Match {
	h.m.fromACI = aci
	return h
}

// Stage narrows the match to conversations whose current stage equals
// the given value (see [Convo.SetStage]). The empty string disables
// the filter; pass it to [Match.AnyStage] to match conversations with
// any non-empty stage.
func (h Match) Stage(stage string) Match {
	h.m.stage = stage
	h.m.stageAny = false
	return h
}

// AnyStage narrows the match to conversations that have a non-empty
// stage. Useful for "wildcard" stage handlers, e.g. a /cancel command
// that fires only when the user is mid-flow.
func (h Match) AnyStage() Match {
	h.m.stage = ""
	h.m.stageAny = true
	return h
}

// Use attaches per-handler middleware. Middleware registered here runs
// after any global middleware from [Bot.Use] but before the handler.
// Multiple calls chain in registration order (first = outermost).
func (h Match) Use(mw MiddlewareFunc) Match {
	dst := make([]MiddlewareFunc, len(h.mw)+1)
	copy(dst, h.mw)
	dst[len(h.mw)] = mw
	h.mw = dst
	return h
}

// InGroups narrows the match to group messages whose group ID is one of
// the provided hex-encoded master keys. DM messages (empty GroupID) are
// unaffected: they still match. Pair with [Match.Group] to restrict to
// group-only traffic.
//
// Security: DM messages always pass this filter — InGroups alone does NOT
// prevent a direct-message sender from triggering this handler. If your
// handler must never run for DMs (e.g. admin commands, group-scoped secrets),
// chain .Group().InGroups(...) instead of .InGroups(...) alone.
func (h Match) InGroups(groupIDs ...string) Match {
	ids := make(map[string]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		ids[id] = struct{}{}
	}
	h.m.allowedGroupIDs = ids
	return h
}

// Do registers handler for the current matcher. Handlers receive
// (ctx, *Message, []string-of-captures-or-args).
func (h Match) Do(handler HandlerFunc) {
	h.bot.mu.Lock()
	defer h.bot.mu.Unlock()
	h.bot.textHandlers = append(h.bot.textHandlers, textHandler{
		matcher:    h.m,
		run:        handler,
		middleware: h.mw,
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

// OnAnyText registers a handler that matches any message body (including
// empty). Useful with [Match.Stage] for wizard steps.
func (b *Bot) OnAnyText() Match {
	return Match{bot: b, m: matcher{kind: matchAnyText}}
}

type textHandler struct {
	matcher    matcher
	run        HandlerFunc
	middleware []MiddlewareFunc
}

// ReactionHandlerFunc is invoked for each inbound reaction that
// matches a registered [Bot.OnReaction] / [Bot.OnAnyReaction] handler.
type ReactionHandlerFunc func(ctx context.Context, r *Reaction) error

// EditHandlerFunc is invoked for each inbound edit that matches a
// registered [Bot.OnEdit] handler.
type EditHandlerFunc func(ctx context.Context, e *Edit) error

type reactionMatcher struct {
	emoji           string // empty = match any emoji
	includeRm       bool   // include "remove" reactions even when emoji is set
	dmOnly          bool
	groupOnly       bool
	fromACI         string
	allowedGroupIDs map[string]struct{}
}

func (m reactionMatcher) match(ev *signal.ReactionEvent) bool {
	if m.dmOnly && ev.GroupID != "" {
		return false
	}
	if m.groupOnly && ev.GroupID == "" {
		return false
	}
	if m.fromACI != "" && ev.Sender != m.fromACI {
		return false
	}
	if m.allowedGroupIDs != nil && ev.GroupID != "" {
		if _, ok := m.allowedGroupIDs[ev.GroupID]; !ok {
			return false
		}
	}
	if !m.includeRm && ev.Remove {
		return false
	}
	if m.emoji != "" && ev.Emoji != m.emoji {
		return false
	}
	return true
}

type reactionHandler struct {
	matcher reactionMatcher
	run     ReactionHandlerFunc
}

type editMatcher struct {
	dmOnly          bool
	groupOnly       bool
	fromACI         string
	allowedGroupIDs map[string]struct{}
}

func (m editMatcher) match(ev *signal.EditMessageEvent) bool {
	if m.dmOnly && ev.GroupID != "" {
		return false
	}
	if m.groupOnly && ev.GroupID == "" {
		return false
	}
	if m.fromACI != "" && ev.Sender != m.fromACI {
		return false
	}
	if m.allowedGroupIDs != nil && ev.GroupID != "" {
		if _, ok := m.allowedGroupIDs[ev.GroupID]; !ok {
			return false
		}
	}
	return true
}

type editHandler struct {
	matcher editMatcher
	run     EditHandlerFunc
}

// ReactionMatch is the public matcher returned by [Bot.OnReaction] and
// [Bot.OnAnyReaction]. Narrow scope with DM/Group/From; finalize with Do.
type ReactionMatch struct {
	bot *Bot
	m   reactionMatcher
}

// DM narrows the match to direct (non-group) reactions.
func (h ReactionMatch) DM() ReactionMatch { h.m.dmOnly = true; return h }

// Group narrows the match to group-thread reactions.
func (h ReactionMatch) Group() ReactionMatch { h.m.groupOnly = true; return h }

// From narrows the match to reactions whose sender ACI equals aci.
func (h ReactionMatch) From(aci string) ReactionMatch { h.m.fromACI = aci; return h }

// InGroups narrows the match to reactions whose group ID is one of the
// provided hex-encoded master keys. DM reactions (empty GroupID) always pass
// this filter. Chain [ReactionMatch.Group] before InGroups to block DM
// reactions entirely.
func (h ReactionMatch) InGroups(groupIDs ...string) ReactionMatch {
	ids := make(map[string]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		ids[id] = struct{}{}
	}
	h.m.allowedGroupIDs = ids
	return h
}

// IncludeRemovals reverses the default behavior of skipping "remove"
// reactions: with this set, a removal of the matching emoji (or any
// emoji, for OnAnyReaction) also fires the handler. The handler can
// distinguish via [Reaction.IsRemoval].
func (h ReactionMatch) IncludeRemovals() ReactionMatch { h.m.includeRm = true; return h }

// Do registers handler for the current matcher.
func (h ReactionMatch) Do(handler ReactionHandlerFunc) {
	h.bot.mu.Lock()
	defer h.bot.mu.Unlock()
	h.bot.reactionHandlers = append(h.bot.reactionHandlers, reactionHandler{
		matcher: h.m,
		run:     handler,
	})
}

// EditMatch is the public matcher returned by [Bot.OnEdit].
type EditMatch struct {
	bot *Bot
	m   editMatcher
}

// DM narrows the match to direct (non-group) edits.
func (h EditMatch) DM() EditMatch { h.m.dmOnly = true; return h }

// Group narrows the match to group-thread edits.
func (h EditMatch) Group() EditMatch { h.m.groupOnly = true; return h }

// From narrows the match to edits whose sender ACI equals aci.
func (h EditMatch) From(aci string) EditMatch { h.m.fromACI = aci; return h }

// InGroups narrows the match to edits whose group ID is one of the
// provided hex-encoded master keys. DM edits (empty GroupID) always pass this
// filter. Chain [EditMatch.Group] before InGroups to block DM edits entirely.
func (h EditMatch) InGroups(groupIDs ...string) EditMatch {
	ids := make(map[string]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		ids[id] = struct{}{}
	}
	h.m.allowedGroupIDs = ids
	return h
}

// Do registers handler for the current matcher.
func (h EditMatch) Do(handler EditHandlerFunc) {
	h.bot.mu.Lock()
	defer h.bot.mu.Unlock()
	h.bot.editHandlers = append(h.bot.editHandlers, editHandler{
		matcher: h.m,
		run:     handler,
	})
}

// OnReaction registers a handler for reactions whose emoji matches the
// argument exactly. Removals (Reaction.Remove == true) are skipped by
// default; chain [ReactionMatch.IncludeRemovals] to receive them.
func (b *Bot) OnReaction(emoji string) ReactionMatch {
	return ReactionMatch{bot: b, m: reactionMatcher{emoji: emoji}}
}

// OnAnyReaction registers a handler for any inbound reaction
// (regardless of emoji). Removals are skipped by default — chain
// [ReactionMatch.IncludeRemovals] to receive them.
func (b *Bot) OnAnyReaction() ReactionMatch {
	return ReactionMatch{bot: b}
}

// OnEdit registers a handler for inbound message edits. Use
// chained scope helpers (DM/Group/From) to narrow.
func (b *Bot) OnEdit() EditMatch {
	return EditMatch{bot: b}
}

// OnGroupUpdate registers a handler for inbound Groups v2 change
// notifications (membership or metadata updates from peers).
func (b *Bot) OnGroupUpdate(handler GroupUpdateHandlerFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.groupUpdateHandlers = append(b.groupUpdateHandlers, groupUpdateHandler{run: handler})
}
