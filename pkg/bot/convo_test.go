package bot

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryConvoStoreGetSetDelete(t *testing.T) {
	s := NewMemoryConvoStore()
	k := ConvoKey{Sender: "alice"}

	if _, ok := s.Get(k, "stage"); ok {
		t.Errorf("Get on empty store should not be ok")
	}

	s.Set(k, "stage", "awaiting_email")
	if v, ok := s.Get(k, "stage"); !ok || v != "awaiting_email" {
		t.Errorf("Get(stage) = %q,%v; want awaiting_email,true", v, ok)
	}

	s.Set(k, "stage", "awaiting_age")
	if v, _ := s.Get(k, "stage"); v != "awaiting_age" {
		t.Errorf("overwrite failed: stage = %q", v)
	}

	s.Set(k, "email", "a@b.com")
	all := s.All(k)
	if len(all) != 2 || all["stage"] != "awaiting_age" || all["email"] != "a@b.com" {
		t.Errorf("All = %v", all)
	}

	s.Delete(k, "email")
	if _, ok := s.Get(k, "email"); ok {
		t.Errorf("Delete didn't remove field")
	}

	s.Clear(k)
	if got := s.All(k); len(got) != 0 {
		t.Errorf("Clear didn't empty conversation: %v", got)
	}
}

func TestMemoryConvoStoreScopesByKey(t *testing.T) {
	s := NewMemoryConvoStore()
	a := ConvoKey{Sender: "alice"}
	bdm := ConvoKey{Sender: "bob"}
	bGroup := ConvoKey{Sender: "bob", GroupID: "g1"}

	s.Set(a, "stage", "x")
	s.Set(bdm, "stage", "y")
	s.Set(bGroup, "stage", "z")

	if v, _ := s.Get(a, "stage"); v != "x" {
		t.Errorf("alice stage = %q; want x", v)
	}
	if v, _ := s.Get(bdm, "stage"); v != "y" {
		t.Errorf("bob-dm stage = %q; want y", v)
	}
	if v, _ := s.Get(bGroup, "stage"); v != "z" {
		t.Errorf("bob-group stage = %q; want z", v)
	}
}

func TestMemoryConvoStoreAllReturnsCopy(t *testing.T) {
	s := NewMemoryConvoStore()
	k := ConvoKey{Sender: "alice"}
	s.Set(k, "stage", "one")
	got := s.All(k)
	got["stage"] = "two"
	if v, _ := s.Get(k, "stage"); v != "one" {
		t.Errorf("All() returned a live view: stage now %q", v)
	}
}

func TestMemoryConvoStoreConcurrent(t *testing.T) {
	s := NewMemoryConvoStore()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			k := ConvoKey{Sender: "alice"}
			for j := 0; j < 100; j++ {
				s.Set(k, "stage", "x")
				_, _ = s.Get(k, "stage")
				_ = s.All(k)
			}
		}()
	}
	wg.Wait()
}

func TestConvoStageHelpers(t *testing.T) {
	c := &Convo{store: NewMemoryConvoStore(), key: ConvoKey{Sender: "alice"}}
	if c.Stage() != "" {
		t.Errorf("empty store should give empty stage")
	}
	c.SetStage("await_email")
	if c.Stage() != "await_email" {
		t.Errorf("SetStage didn't persist; Stage() = %q", c.Stage())
	}
	c.SetStage("") // empty clears
	if c.Stage() != "" {
		t.Errorf("SetStage(\"\") should clear, got %q", c.Stage())
	}
	c.SetStage("a")
	c.ClearStage()
	if c.Stage() != "" {
		t.Errorf("ClearStage failed: %q", c.Stage())
	}
}

func TestBotConvoForReturnsScopedHandle(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	k := ConvoKey{Sender: "alice"}
	b.Convo().For(k).Set("stage", "x")
	if v, _ := b.Convo().For(k).Get("stage"); v != "x" {
		t.Errorf("Convo.For round-trip failed: %q", v)
	}
}

func TestMessageConvoIsScopedToSenderAndGroup(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var wg sync.WaitGroup
	wg.Add(1)
	var seenStage string
	b.OnText("hi").Do(func(_ context.Context, m *Message, _ []string) error {
		defer wg.Done()
		seenStage = m.Convo().Stage()
		m.Convo().SetStage("greeted")
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "hi")
	wg.Wait()
	cancel()
	_ = wait()

	if seenStage != "" {
		t.Errorf("first call should see empty stage, got %q", seenStage)
	}
	if v := b.Convo().For(ConvoKey{Sender: "alice"}).Stage(); v != "greeted" {
		t.Errorf("stage not persisted via Message.Convo: %q", v)
	}
}

func TestStageMatcherFiltersByStage(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var wg sync.WaitGroup
	wg.Add(1)
	var matchedStage string

	b.OnText("alice@example.com").Stage("await_email").Do(func(_ context.Context, m *Message, _ []string) error {
		defer wg.Done()
		matchedStage = m.Convo().Stage()
		m.Convo().SetStage("await_age")
		return nil
	})
	// Without the stage set, the next handler must NOT fire.
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "alice@example.com")
	// Give dispatcher time to finish.
	time.Sleep(50 * time.Millisecond)
	if matchedStage != "" {
		t.Errorf("handler fired before stage set, matchedStage=%q", matchedStage)
	}

	// Now set the stage and re-send.
	b.Convo().For(ConvoKey{Sender: "alice"}).SetStage("await_email")
	fc.events <- msgEv("alice", "alice@example.com")
	wg.Wait()
	cancel()
	_ = wait()
	if matchedStage != "await_email" {
		t.Errorf("matchedStage = %q, want await_email", matchedStage)
	}
	if got := b.Convo().For(ConvoKey{Sender: "alice"}).Stage(); got != "await_age" {
		t.Errorf("stage transition not stored: %q", got)
	}
}

func TestAnyStageMatcherIgnoresEmptyStage(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	var fired bool
	b.OnCommand("cancel").AnyStage().Do(func(_ context.Context, m *Message, _ []string) error {
		fired = true
		m.Convo().ClearStage()
		return nil
	})
	cancel, wait := runBot(t, b)
	// No stage set; AnyStage should NOT fire.
	fc.events <- msgEv("alice", "/cancel")
	time.Sleep(50 * time.Millisecond)
	if fired {
		t.Errorf("AnyStage handler fired when stage was empty")
	}

	// Set a stage, send again.
	b.Convo().For(ConvoKey{Sender: "alice"}).SetStage("await_email")
	var wg sync.WaitGroup
	wg.Add(1)
	b.OnCommand("done").AnyStage().Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		return nil
	})
	fc.events <- msgEv("alice", "/done")
	wg.Wait()
	cancel()
	_ = wait()
}

func TestStageMatcherWithDMScope(t *testing.T) {
	// Combining Stage with DM scope should still work.
	fc := newFakeClient()
	b := Wrap(fc)
	b.Convo().For(ConvoKey{Sender: "alice"}).SetStage("s1")
	var wg sync.WaitGroup
	wg.Add(1)
	b.OnText("ok").Stage("s1").DM().Do(func(_ context.Context, _ *Message, _ []string) error {
		defer wg.Done()
		return nil
	})
	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "ok")
	wg.Wait()
	cancel()
	_ = wait()
}

func TestWrapWithOptionsUsesProvidedConvoStore(t *testing.T) {
	fc := newFakeClient()
	custom := NewMemoryConvoStore()
	// Pre-populate via the Convo helper so the well-known stage field is used.
	(&Convo{store: custom, key: ConvoKey{Sender: "alice"}}).SetStage("preset")
	b := WrapWithOptions(fc, WrapOptions{ConvoStore: custom})
	if b.Convo().Store() != custom {
		t.Errorf("Bot did not use provided ConvoStore")
	}
	if v := b.Convo().For(ConvoKey{Sender: "alice"}).Stage(); v != "preset" {
		t.Errorf("preset stage not visible: %q", v)
	}
}
