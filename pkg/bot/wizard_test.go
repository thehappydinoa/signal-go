package bot

import (
	"context"
	"sync"
	"testing"
)

func TestWizardMultiStepFlow(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var replies sync.WaitGroup
	replies.Add(3)

	signup := b.Wizard("signup")
	signup.Step("await_email", func(ctx context.Context, m *Message, _ []string) error {
		defer replies.Done()
		m.Convo().Set("email", m.Body())
		signup.Advance(m, "await_age")
		return m.Reply(ctx, "age?")
	})
	signup.Step("await_age", func(ctx context.Context, m *Message, _ []string) error {
		defer replies.Done()
		m.Convo().Set("age", m.Body())
		signup.Clear(m)
		return m.Reply(ctx, "done")
	})
	signup.Register()

	b.OnCommand("signup").DM().Do(func(ctx context.Context, m *Message, _ []string) error {
		defer replies.Done()
		if err := signup.Begin(ctx, m, ""); err != nil {
			return err
		}
		return m.Reply(ctx, "email?")
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "/signup")
	fc.events <- msgEv("alice", "a@b.com")
	fc.events <- msgEv("alice", "30")
	replies.Wait()
	cancel()
	_ = wait()

	got := fc.Sends()
	if len(got) != 3 {
		t.Fatalf("sends = %d, want 3", len(got))
	}
	if got[0].text != "email?" || got[1].text != "age?" || got[2].text != "done" {
		t.Fatalf("sends = %+v", got)
	}
}
