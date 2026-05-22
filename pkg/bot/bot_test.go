package bot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// fakeClient is a stand-in for *signal.Client. It exposes a buffered
// Events channel the test can push events into, and records every Send /
// SendReceipt / SendTyping / SendReaction call.
type fakeClient struct {
	events chan signal.Event

	mu             sync.Mutex
	sends          []sentMessage
	groupSends     []sentGroupMessage
	groupReactions []sentGroupReaction
	groupTypings   []sentGroupTyping
	receipts       []sentReceipt
	typings        []sentTyping
	reactions      []sentReaction
}

type sentMessage struct {
	to, text string
}

type sentGroupMessage struct {
	groupIDHex, text string
}

type sentGroupReaction struct {
	groupIDHex, emoji, targetAuthor string
	targetTS                        time.Time
	remove                          bool
}

type sentGroupTyping struct {
	groupIDHex string
	action     signal.TypingAction
}

type sentReceipt struct {
	to         string
	kind       signal.ReceiptType
	timestamps []time.Time
}

type sentTyping struct {
	to     string
	action signal.TypingAction
}

type sentReaction struct {
	to, emoji, targetAuthor string
	targetTS                time.Time
	remove                  bool
}

func newFakeClient() *fakeClient {
	return &fakeClient{events: make(chan signal.Event, 8)}
}

func (f *fakeClient) Events() <-chan signal.Event { return f.events }

func (f *fakeClient) Send(_ context.Context, to, text string) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sentMessage{to: to, text: text})
	return signal.Receipt{Timestamp: time.Now(), RecipientACI: to}, nil
}

func (f *fakeClient) SendGroup(_ context.Context, masterKey []byte, text string) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groupSends = append(f.groupSends, sentGroupMessage{groupIDHex: fmt.Sprintf("%x", masterKey), text: text})
	return signal.Receipt{Timestamp: time.Now()}, nil
}

func (f *fakeClient) SendGroupReaction(_ context.Context, masterKey []byte, emoji, targetAuthor string, targetTS time.Time, remove bool) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groupReactions = append(f.groupReactions, sentGroupReaction{
		groupIDHex:   fmt.Sprintf("%x", masterKey),
		emoji:        emoji,
		targetAuthor: targetAuthor,
		targetTS:     targetTS,
		remove:       remove,
	})
	return signal.Receipt{Timestamp: time.Now()}, nil
}

func (f *fakeClient) SendGroupTyping(_ context.Context, masterKey []byte, action signal.TypingAction) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groupTypings = append(f.groupTypings, sentGroupTyping{groupIDHex: fmt.Sprintf("%x", masterKey), action: action})
	return signal.Receipt{Timestamp: time.Now()}, nil
}

func (f *fakeClient) SendReceipt(_ context.Context, to string, kind signal.ReceiptType, ts []time.Time) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tsCopy := append([]time.Time(nil), ts...)
	f.receipts = append(f.receipts, sentReceipt{to: to, kind: kind, timestamps: tsCopy})
	return signal.Receipt{Timestamp: time.Now(), RecipientACI: to}, nil
}

func (f *fakeClient) SendTyping(_ context.Context, to string, action signal.TypingAction) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typings = append(f.typings, sentTyping{to: to, action: action})
	return signal.Receipt{Timestamp: time.Now(), RecipientACI: to}, nil
}

func (f *fakeClient) SendReaction(_ context.Context, to, emoji, targetAuthor string, targetTS time.Time, remove bool) (signal.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reactions = append(f.reactions, sentReaction{to: to, emoji: emoji, targetAuthor: targetAuthor, targetTS: targetTS, remove: remove})
	return signal.Receipt{Timestamp: time.Now(), RecipientACI: to}, nil
}

func (f *fakeClient) Close() error { close(f.events); return nil }

func (f *fakeClient) Sends() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentMessage, len(f.sends))
	copy(out, f.sends)
	return out
}

func (f *fakeClient) Receipts() []sentReceipt {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentReceipt, len(f.receipts))
	copy(out, f.receipts)
	return out
}

func (f *fakeClient) Typings() []sentTyping {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentTyping, len(f.typings))
	copy(out, f.typings)
	return out
}

func (f *fakeClient) Reactions() []sentReaction {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentReaction, len(f.reactions))
	copy(out, f.reactions)
	return out
}

func (f *fakeClient) GroupReactions() []sentGroupReaction {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentGroupReaction, len(f.groupReactions))
	copy(out, f.groupReactions)
	return out
}

func (f *fakeClient) GroupTypings() []sentGroupTyping {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentGroupTyping, len(f.groupTypings))
	copy(out, f.groupTypings)
	return out
}

// runBot starts b.Run in a goroutine and returns a cancel func + the
// goroutine's exit error.
func runBot(t *testing.T, b *Bot) (cancel context.CancelFunc, wait func() error) {
	t.Helper()
	ctx, c := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- b.Run(ctx) }()
	return c, func() error {
		select {
		case err := <-errCh:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return after cancel")
			return nil
		}
	}
}

func msgEv(sender, body string) *signal.MessageEvent {
	return &signal.MessageEvent{
		Sender:    sender,
		Body:      body,
		Timestamp: time.Now(),
	}
}

func TestOnTextExactMatch(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var pinged sync.WaitGroup
	pinged.Add(1)
	b.OnText("ping").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer pinged.Done()
		return m.Reply(ctx, "pong")
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice-aci", "ping")
	pinged.Wait()
	cancel()
	_ = wait()

	sends := fc.Sends()
	if len(sends) != 1 || sends[0].to != "alice-aci" || sends[0].text != "pong" {
		t.Errorf("sends = %+v", sends)
	}
}

func TestOnTextDoesNotMatchOtherBody(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnText("ping").Do(func(ctx context.Context, m *Message, _ []string) error {
		return m.Reply(ctx, "pong")
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "pong")
	// Give the dispatcher a moment.
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	if got := fc.Sends(); len(got) != 0 {
		t.Errorf("expected no sends, got %+v", got)
	}
}

func TestOnPrefixCaseInsensitive(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	b.OnPrefix("hello").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer fired.Done()
		return m.Reply(ctx, "hi")
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "HELLO bot")
	fired.Wait()
	cancel()
	_ = wait()

	if got := fc.Sends(); len(got) != 1 || got[0].text != "hi" {
		t.Errorf("sends = %+v", got)
	}
}

func TestOnRegexCapturesAreSurfaced(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	var captured []string
	b.OnRegex(regexp.MustCompile(`(?i)^remind (.+)$`)).Do(func(_ context.Context, _ *Message, args []string) error {
		defer fired.Done()
		captured = args
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "remind buy milk")
	fired.Wait()
	cancel()
	_ = wait()

	if len(captured) != 2 || captured[1] != "buy milk" {
		t.Errorf("captured = %v", captured)
	}
}

func TestOnCommandParsesArgs(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	var args []string
	b.OnCommand("weather").Do(func(_ context.Context, _ *Message, a []string) error {
		defer fired.Done()
		args = a
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "/weather seattle imperial")
	fired.Wait()
	cancel()
	_ = wait()

	if len(args) != 2 || args[0] != "seattle" || args[1] != "imperial" {
		t.Errorf("args = %v", args)
	}
}

func TestOnCommandRejectsBodyWithoutSlash(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnCommand("weather").Do(func(_ context.Context, _ *Message, _ []string) error {
		t.Fatal("handler should not fire for non-slash body")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "weather seattle")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestOnCommandDistinguishesPrefixCollisions(t *testing.T) {
	// /foo should not match the body "/foobar".
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnCommand("foo").Do(func(_ context.Context, _ *Message, _ []string) error {
		t.Fatal("/foo should not match /foobar")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "/foobar")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestHandlerFirstMatchWins(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var firstFired, secondFired bool
	b.OnPrefix("hi").Do(func(_ context.Context, _ *Message, _ []string) error { firstFired = true; return nil })
	b.OnText("hi there").Do(func(_ context.Context, _ *Message, _ []string) error { secondFired = true; return nil })

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi there")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()

	if !firstFired || secondFired {
		t.Errorf("dispatch order broken: first=%v second=%v", firstFired, secondFired)
	}
}

func TestHandlerErrPassFallsThrough(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var firstFired, secondFired sync.WaitGroup
	firstFired.Add(1)
	secondFired.Add(1)
	b.OnPrefix("hi").Do(func(_ context.Context, _ *Message, _ []string) error {
		defer firstFired.Done()
		return ErrPass
	})
	b.OnText("hi there").Do(func(_ context.Context, _ *Message, _ []string) error {
		defer secondFired.Done()
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi there")
	firstFired.Wait()
	secondFired.Wait()
	cancel()
	_ = wait()
}

func TestOnErrorReceivesNonNilHandlerErrors(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	wantErr := errors.New("boom")
	var got error
	var fired sync.WaitGroup
	fired.Add(1)
	b.OnError(func(_ context.Context, _ signal.Event, e error) {
		defer fired.Done()
		got = e
	})
	b.OnText("kaboom").Do(func(_ context.Context, _ *Message, _ []string) error {
		return wantErr
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "kaboom")
	fired.Wait()
	cancel()
	_ = wait()

	if !errors.Is(got, wantErr) {
		t.Errorf("got = %v, want = %v", got, wantErr)
	}
}

func TestReplyInGroupUsesSendGroup(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	var replyErr error
	b.OnText("hello").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer fired.Done()
		replyErr = m.Reply(ctx, "hi")
		return nil
	})

	cancel, wait := runBot(t, b)
	groupID := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	ev := msgEv("alice", "hello")
	ev.GroupID = groupID
	fc.events <- ev
	fired.Wait()
	cancel()
	_ = wait()

	if replyErr != nil {
		t.Fatalf("replyErr = %v", replyErr)
	}
	if len(fc.groupSends) != 1 || fc.groupSends[0].text != "hi" {
		t.Errorf("groupSends = %+v", fc.groupSends)
	}
	if len(fc.Sends()) != 0 {
		t.Errorf("group reply should not use 1:1 Send")
	}
}

func TestRunReturnsOnEventsClose(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	errCh := make(chan error, 1)
	go func() { errCh <- b.Run(context.Background()) }()

	close(fc.events) // simulate the client shutting down
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Run = %v, want nil on graceful close", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after events channel closed")
	}
}

func TestOpenRequiresStores(t *testing.T) {
	_, err := Open(context.Background(), Options{})
	if err == nil {
		t.Error("expected error when AccountStore + SignalStores missing")
	}
}

// groupMsgEv is like msgEv but includes a group ID (sender/body fixed for tests).
func groupMsgEv(groupID string) *signal.MessageEvent {
	return &signal.MessageEvent{
		Sender:    "alice",
		GroupID:   groupID,
		Body:      "hi",
		Timestamp: time.Now(),
	}
}

// --- scope tests ---

func TestDMScopeFiltersGroupMessages(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnText("hi").DM().Do(func(_ context.Context, _ *Message, _ []string) error {
		t.Fatal("DM handler must not fire for group messages")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- groupMsgEv("group-id-abc")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestDMScopeAllowsDirectMessages(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var wg sync.WaitGroup
	wg.Add(1)
	b.OnText("hi").DM().Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi") // no GroupID → DM
	wg.Wait()
	cancel()
	_ = wait()
}

func TestGroupScopeFiltersDirectMessages(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnText("hi").Group().Do(func(_ context.Context, _ *Message, _ []string) error {
		t.Fatal("Group handler must not fire for DM messages")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestGroupScopeAllowsGroupMessages(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var wg sync.WaitGroup
	wg.Add(1)
	b.OnText("hi").Group().Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- groupMsgEv("group-id-abc")
	wg.Wait()
	cancel()
	_ = wait()
}

func TestFromScopeFiltersOtherSenders(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnText("hi").From("allowed-aci").Do(func(_ context.Context, _ *Message, _ []string) error {
		t.Fatal("From filter must not fire for other senders")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("other-aci", "hi")
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestFromScopeAllowsMatchingSender(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var wg sync.WaitGroup
	wg.Add(1)
	b.OnText("hi").From("alice-aci").Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice-aci", "hi")
	wg.Wait()
	cancel()
	_ = wait()
}

// --- middleware tests ---

func TestPerHandlerMiddlewareWrapsHandler(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var order []string
	var wg sync.WaitGroup
	wg.Add(1)

	mw := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, m *Message, args []string) error {
			order = append(order, "mw")
			return next(ctx, m, args)
		}
	}
	b.OnText("hi").Use(mw).Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		order = append(order, "handler")
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	wg.Wait()
	cancel()
	_ = wait()

	if len(order) != 2 || order[0] != "mw" || order[1] != "handler" {
		t.Errorf("order = %v, want [mw handler]", order)
	}
}

func TestGlobalMiddlewareWrapsAllHandlers(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var order []string
	var wg sync.WaitGroup
	wg.Add(1)

	b.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, m *Message, args []string) error {
			order = append(order, "global")
			return next(ctx, m, args)
		}
	})
	b.OnText("hi").Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		order = append(order, "handler")
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	wg.Wait()
	cancel()
	_ = wait()

	if len(order) != 2 || order[0] != "global" || order[1] != "handler" {
		t.Errorf("order = %v, want [global handler]", order)
	}
}

func TestGlobalMiddlewareOuterPerHandlerMiddlewareInner(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var order []string
	var wg sync.WaitGroup
	wg.Add(1)

	b.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, m *Message, args []string) error {
			order = append(order, "global")
			return next(ctx, m, args)
		}
	})
	b.OnText("hi").Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, m *Message, args []string) error {
			order = append(order, "per-handler")
			return next(ctx, m, args)
		}
	}).Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		order = append(order, "handler")
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	wg.Wait()
	cancel()
	_ = wait()

	if len(order) != 3 || order[0] != "global" || order[1] != "per-handler" || order[2] != "handler" {
		t.Errorf("order = %v, want [global per-handler handler]", order)
	}
}

// --- reaction / edit / receipt / typing helpers ---

// targetACI used by all reactionEv constructions in this file. Pulled
// out as a constant so unparam doesn't flag a string parameter that's
// always the same literal.
const targetACI = "bob"

func reactionEv(sender, emoji string, targetTS time.Time, remove bool) *signal.ReactionEvent {
	return &signal.ReactionEvent{
		Sender:          sender,
		Emoji:           emoji,
		Remove:          remove,
		TargetAuthorACI: targetACI,
		TargetTimestamp: targetTS,
		Timestamp:       time.Now(),
	}
}

func editEv(sender string, targetTS time.Time, body string) *signal.EditMessageEvent {
	return &signal.EditMessageEvent{
		Sender:          sender,
		TargetTimestamp: targetTS,
		NewBody:         body,
		Timestamp:       time.Now(),
	}
}

func TestOnReactionEmojiMatch(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	var got *Reaction
	b.OnReaction("👍").Do(func(_ context.Context, r *Reaction) error {
		defer fired.Done()
		got = r
		return nil
	})

	cancel, wait := runBot(t, b)
	target := time.Now().Add(-time.Minute)
	fc.events <- reactionEv("alice", "👍", target, false)
	fired.Wait()
	cancel()
	_ = wait()

	if got == nil || got.Emoji() != "👍" || got.Sender() != "alice" || !got.TargetTimestamp().Equal(target) {
		t.Errorf("got reaction = %+v", got)
	}
}

func TestOnReactionSkipsEmojiMismatch(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnReaction("👍").Do(func(_ context.Context, _ *Reaction) error {
		t.Fatal("handler must not fire for different emoji")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- reactionEv("alice", "❤️", time.Now(), false)
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestOnReactionSkipsRemovalsByDefault(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnReaction("👍").Do(func(_ context.Context, _ *Reaction) error {
		t.Fatal("handler must not fire for removals by default")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- reactionEv("alice", "👍", time.Now(), true)
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestOnReactionIncludeRemovalsFires(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	var got *Reaction
	b.OnReaction("👍").IncludeRemovals().Do(func(_ context.Context, r *Reaction) error {
		defer fired.Done()
		got = r
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- reactionEv("alice", "👍", time.Now(), true)
	fired.Wait()
	cancel()
	_ = wait()
	if got == nil || !got.IsRemoval() {
		t.Errorf("got = %+v, want IsRemoval()=true", got)
	}
}

func TestOnAnyReactionFiresForAnyEmoji(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	b.OnAnyReaction().Do(func(_ context.Context, _ *Reaction) error {
		defer fired.Done()
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- reactionEv("alice", "🎉", time.Now(), false)
	fired.Wait()
	cancel()
	_ = wait()
}

func TestOnEditFires(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired sync.WaitGroup
	fired.Add(1)
	var got *Edit
	b.OnEdit().Do(func(_ context.Context, e *Edit) error {
		defer fired.Done()
		got = e
		return nil
	})
	cancel, wait := runBot(t, b)
	target := time.Now().Add(-time.Minute)
	fc.events <- editEv("alice", target, "fixed typo")
	fired.Wait()
	cancel()
	_ = wait()
	if got == nil || got.NewBody() != "fixed typo" || !got.TargetTimestamp().Equal(target) {
		t.Errorf("got = %+v", got)
	}
}

func TestMessageReactSendsReaction(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var done sync.WaitGroup
	done.Add(1)
	b.OnText("hi").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer done.Done()
		return m.React(ctx, "👍")
	})
	cancel, wait := runBot(t, b)
	ev := msgEv("alice", "hi")
	fc.events <- ev
	done.Wait()
	cancel()
	_ = wait()

	got := fc.Reactions()
	if len(got) != 1 || got[0].emoji != "👍" || got[0].targetAuthor != "alice" || got[0].remove {
		t.Errorf("reactions = %+v", got)
	}
}

func TestMessageReactInGroupUsesSendGroupReaction(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var done sync.WaitGroup
	done.Add(1)
	var reactErr error
	groupID := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	b.OnText("hi").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer done.Done()
		reactErr = m.React(ctx, "👍")
		return nil
	})
	cancel, wait := runBot(t, b)
	ev := groupMsgEv(groupID)
	fc.events <- ev
	done.Wait()
	cancel()
	_ = wait()
	if reactErr != nil {
		t.Fatalf("reactErr = %v", reactErr)
	}
	got := fc.GroupReactions()
	if len(got) != 1 || got[0].emoji != "👍" || got[0].targetAuthor != "alice" || got[0].remove {
		t.Errorf("groupReactions = %+v", got)
	}
	if len(fc.Reactions()) != 0 {
		t.Error("group react should not use 1:1 SendReaction")
	}
}

func TestMessageTypingInGroupUsesSendGroupTyping(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var done sync.WaitGroup
	done.Add(1)
	groupID := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	b.OnText("hi").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer done.Done()
		return m.Typing(ctx, signal.TypingStarted)
	})
	cancel, wait := runBot(t, b)
	fc.events <- groupMsgEv(groupID)
	done.Wait()
	cancel()
	_ = wait()
	got := fc.GroupTypings()
	if len(got) != 1 || got[0].action != signal.TypingStarted {
		t.Errorf("groupTypings = %+v", got)
	}
}

func TestMessageMarkReadInGroupSendsReceiptToAuthor(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var done sync.WaitGroup
	done.Add(1)
	groupID := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	b.OnText("hi").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer done.Done()
		return m.MarkRead(ctx)
	})
	cancel, wait := runBot(t, b)
	fc.events <- groupMsgEv(groupID)
	done.Wait()
	cancel()
	_ = wait()
	got := fc.Receipts()
	if len(got) != 1 || got[0].kind != signal.ReceiptRead || got[0].to != "alice" {
		t.Errorf("receipts = %+v", got)
	}
}

func TestMessageMarkReadSendsReceipt(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var done sync.WaitGroup
	done.Add(1)
	b.OnText("hi").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer done.Done()
		return m.MarkRead(ctx)
	})
	cancel, wait := runBot(t, b)
	ev := msgEv("alice", "hi")
	fc.events <- ev
	done.Wait()
	cancel()
	_ = wait()
	got := fc.Receipts()
	if len(got) != 1 || got[0].kind != signal.ReceiptRead || got[0].to != "alice" {
		t.Errorf("receipts = %+v", got)
	}
}

func TestMessageTypingSendsTyping(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var done sync.WaitGroup
	done.Add(1)
	b.OnText("hi").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer done.Done()
		return m.Typing(ctx, signal.TypingStarted)
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	done.Wait()
	cancel()
	_ = wait()
	got := fc.Typings()
	if len(got) != 1 || got[0].action != signal.TypingStarted || got[0].to != "alice" {
		t.Errorf("typings = %+v", got)
	}
}

func TestReactionScopeFromFiltersOtherSenders(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	b.OnAnyReaction().From("allowed").Do(func(_ context.Context, _ *Reaction) error {
		t.Fatal("must not fire for other sender")
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- reactionEv("other", "👍", time.Now(), false)
	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = wait()
}

func TestMiddlewareErrPassFallsThrough(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var wg sync.WaitGroup
	wg.Add(1)

	// Middleware that returns ErrPass without calling next.
	b.OnText("hi").Use(func(_ HandlerFunc) HandlerFunc {
		return func(_ context.Context, _ *Message, _ []string) error {
			return ErrPass
		}
	}).Do(func(_ context.Context, _ *Message, _ []string) error {
		t.Fatal("this handler must not fire when middleware returns ErrPass")
		return nil
	})
	// Fallthrough handler.
	b.OnText("hi").Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	wg.Wait()
	cancel()
	_ = wait()
}
