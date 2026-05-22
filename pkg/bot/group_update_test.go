package bot

import (
	"context"
	"sync"
	"testing"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

func TestOnGroupUpdateDispatches(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var fired sync.WaitGroup
	fired.Add(1)
	b.OnGroupUpdate(func(ctx context.Context, u *GroupUpdate) error {
		defer fired.Done()
		if u.GroupID() != "aa" {
			t.Fatalf("group id = %q", u.GroupID())
		}
		if u.Revision() != 2 {
			t.Fatalf("revision = %d", u.Revision())
		}
		return nil
	})

	cancel, wait := runBot(t, b)
	fc.events <- &signal.GroupUpdateEvent{
		Sender:   "alice",
		GroupID:  "aa",
		Revision: 2,
	}
	fired.Wait()
	cancel()
	_ = wait()
}
