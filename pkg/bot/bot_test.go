package bot

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// fakeClient is a stand-in for *signal.Client. It exposes a buffered
// Events channel the test can push events into, and records every Send
// call.
type fakeClient struct {
	events chan signal.Event

	mu    sync.Mutex
	sends []sentMessage
}

type sentMessage struct {
	to, text string
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

func (f *fakeClient) Close() error { close(f.events); return nil }

func (f *fakeClient) Sends() []sentMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]sentMessage, len(f.sends))
	copy(out, f.sends)
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

func TestReplyInGroupReturnsError(t *testing.T) {
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
	ev := msgEv("alice", "hello")
	ev.GroupID = "deadbeef-group-master-key"
	fc.events <- ev
	fired.Wait()
	cancel()
	_ = wait()

	if !errors.Is(replyErr, ErrReplyNotSupportedInGroup) {
		t.Errorf("replyErr = %v", replyErr)
	}
	if len(fc.Sends()) != 0 {
		t.Errorf("group reply should not send")
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

// groupMsgEv is like msgEv but includes a group ID.
func groupMsgEv(sender, groupID, body string) *signal.MessageEvent {
	return &signal.MessageEvent{
		Sender:    sender,
		GroupID:   groupID,
		Body:      body,
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
	fc.events <- groupMsgEv("alice", "group-id-abc", "hi")
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
	fc.events <- groupMsgEv("alice", "group-id-abc", "hi")
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
