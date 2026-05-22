package bot

import (
	"context"
	"sync"
	"testing"
)

func TestWizardMultiStepFlow(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	signup := b.Wizard("signup")
	signup.Step("await_email", func(ctx context.Context, m *Message, _ []string) error {
		m.Convo().Set("email", m.Body())
		signup.Advance(m, "await_age")
		return m.Reply(ctx, "age?")
	})
	signup.Step("await_age", func(ctx context.Context, m *Message, _ []string) error {
		m.Convo().Set("age", m.Body())
		signup.Clear(m)
		return m.Reply(ctx, "done")
	})
	signup.Register()

	var wg sync.WaitGroup
	wg.Add(1)
	b.OnCommand("signup").DM().Do(func(ctx context.Context, m *Message, _ []string) error {
		defer wg.Done()
		if err := signup.Begin(ctx, m, ""); err != nil {
			return err
		}
		return m.Reply(ctx, "email?")
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "/signup")
	wg.Wait()
	fc.events <- msgEv("alice", "a@b.com")
	fc.events <- msgEv("alice", "30")
	cancel()
	_ = wait()

	if got := fc.Sends(); len(got) != 3 {
		t.Fatalf("sends = %d, want 3", len(got))
	}
}
