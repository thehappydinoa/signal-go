package signal

import (
	"io"
	"log/slog"
	"testing"
	"time"

	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

// newDispatchClient wires a Client with just the receive-side fields so
// dispatch tests can inspect emitted events without spinning up the
// chat ws.
func newDispatchClient() *Client {
	return &Client{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		events: make(chan Event, 8),
	}
}

func TestDispatchReactionEmitsReactionEvent(t *testing.T) {
	c := newDispatchClient()

	emoji := "👍"
	target := uint64(time.Now().Add(-time.Minute).UnixMilli())
	author := "bob-aci"
	remove := false
	rTS := uint64(time.Now().UnixMilli())
	dm := &sspb.DataMessage{
		Timestamp: &rTS,
		Reaction: &sspb.DataMessage_Reaction{
			Emoji:               &emoji,
			Remove:              &remove,
			TargetAuthorAci:     &author,
			TargetSentTimestamp: &target,
		},
	}
	content := &sspb.Content{Content: &sspb.Content_DataMessage{DataMessage: dm}}

	c.dispatchContent("alice-aci", 1, time.Time{}, time.Time{}, content)

	select {
	case ev := <-c.events:
		re, ok := ev.(*ReactionEvent)
		if !ok {
			t.Fatalf("event type = %T, want *ReactionEvent", ev)
		}
		if re.Emoji != emoji {
			t.Errorf("Emoji = %q, want %q", re.Emoji, emoji)
		}
		if re.TargetAuthorACI != author {
			t.Errorf("TargetAuthorACI = %q, want %q", re.TargetAuthorACI, author)
		}
		if re.TargetTimestamp.UnixMilli() != int64(target) {
			t.Errorf("TargetTimestamp = %v, want ms=%d", re.TargetTimestamp, target)
		}
		if re.Remove {
			t.Error("Remove should be false")
		}
	default:
		t.Fatal("no event emitted")
	}
}

func TestDispatchReactionWithoutDataMessageBodyDoesNotEmitMessageEvent(t *testing.T) {
	c := newDispatchClient()
	emoji := "❤️"
	target := uint64(time.Now().UnixMilli())
	author := "bob-aci"
	remove := true
	dm := &sspb.DataMessage{
		Reaction: &sspb.DataMessage_Reaction{
			Emoji:               &emoji,
			Remove:              &remove,
			TargetAuthorAci:     &author,
			TargetSentTimestamp: &target,
		},
	}
	content := &sspb.Content{Content: &sspb.Content_DataMessage{DataMessage: dm}}

	c.dispatchContent("alice-aci", 1, time.Time{}, time.Time{}, content)

	select {
	case ev := <-c.events:
		if _, ok := ev.(*MessageEvent); ok {
			t.Errorf("got *MessageEvent, want only *ReactionEvent for reaction-only DataMessage")
		}
	default:
		t.Fatal("no event emitted")
	}
}

func TestDispatchGroupUpdateEmitsGroupUpdateEvent(t *testing.T) {
	c := newDispatchClient()
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	rev := uint32(12)
	change := []byte{1, 2, 3}
	ts := uint64(time.Now().UnixMilli())
	dm := &sspb.DataMessage{
		Timestamp: &ts,
		GroupV2: &sspb.GroupContextV2{
			MasterKey:   masterKey,
			Revision:    &rev,
			GroupChange: change,
		},
	}
	content := &sspb.Content{Content: &sspb.Content_DataMessage{DataMessage: dm}}

	c.dispatchContent("alice-aci", 1, time.Time{}, time.Time{}, content)

	select {
	case ev := <-c.events:
		gu, ok := ev.(*GroupUpdateEvent)
		if !ok {
			t.Fatalf("event type = %T, want *GroupUpdateEvent", ev)
		}
		if gu.Revision != rev {
			t.Errorf("Revision = %d, want %d", gu.Revision, rev)
		}
		if string(gu.GroupChange) != string(change) {
			t.Errorf("GroupChange = %v", gu.GroupChange)
		}
	default:
		t.Fatal("no event emitted")
	}
}

func TestDispatchGroupUpdateWithoutBodySkipsMessageEvent(t *testing.T) {
	c := newDispatchClient()
	rev := uint32(3)
	dm := &sspb.DataMessage{
		GroupV2: &sspb.GroupContextV2{
			MasterKey:   make([]byte, 32),
			Revision:    &rev,
			GroupChange: []byte{9},
		},
	}
	content := &sspb.Content{Content: &sspb.Content_DataMessage{DataMessage: dm}}

	c.dispatchContent("alice-aci", 1, time.Time{}, time.Time{}, content)

	select {
	case ev := <-c.events:
		if _, ok := ev.(*MessageEvent); ok {
			t.Fatal("unexpected MessageEvent for group-update-only payload")
		}
	default:
		t.Fatal("no event emitted")
	}
}

func TestDispatchGroupUpdateWithBodyEmitsBoth(t *testing.T) {
	c := newDispatchClient()
	body := "update + text"
	rev := uint32(5)
	dm := &sspb.DataMessage{
		Body: &body,
		GroupV2: &sspb.GroupContextV2{
			MasterKey:   make([]byte, 32),
			Revision:    &rev,
			GroupChange: []byte{1},
		},
	}
	content := &sspb.Content{Content: &sspb.Content_DataMessage{DataMessage: dm}}

	c.dispatchContent("alice-aci", 1, time.Time{}, time.Time{}, content)

	first := <-c.events
	if _, ok := first.(*GroupUpdateEvent); !ok {
		t.Fatalf("first event = %T, want GroupUpdateEvent", first)
	}
	second := <-c.events
	if _, ok := second.(*MessageEvent); !ok {
		t.Fatalf("second event = %T, want MessageEvent", second)
	}
}

func TestStoreGroupRevision(t *testing.T) {
	c := newDispatchClient()
	c.storeGroupRevision("abc", 7)
	if got := c.cachedGroupRevision("abc"); got != 7 {
		t.Fatalf("revision = %d", got)
	}
	c.deleteGroupRevision("abc")
	if got := c.cachedGroupRevision("abc"); got != 0 {
		t.Fatalf("after delete revision = %d", got)
	}
}

func TestDispatchEditMessageEmitsEditEvent(t *testing.T) {
	c := newDispatchClient()

	body := "fixed typo"
	dmTS := uint64(time.Now().UnixMilli())
	dm := &sspb.DataMessage{
		Body:      &body,
		Timestamp: &dmTS,
	}
	target := uint64(time.Now().Add(-time.Minute).UnixMilli())
	em := &sspb.EditMessage{
		TargetSentTimestamp: &target,
		DataMessage:         dm,
	}
	content := &sspb.Content{Content: &sspb.Content_EditMessage{EditMessage: em}}

	c.dispatchContent("alice-aci", 1, time.Time{}, time.Time{}, content)

	select {
	case ev := <-c.events:
		ee, ok := ev.(*EditMessageEvent)
		if !ok {
			t.Fatalf("event type = %T, want *EditMessageEvent", ev)
		}
		if ee.NewBody != body {
			t.Errorf("NewBody = %q, want %q", ee.NewBody, body)
		}
		if ee.TargetTimestamp.UnixMilli() != int64(target) {
			t.Errorf("TargetTimestamp = %v, want ms=%d", ee.TargetTimestamp, target)
		}
	default:
		t.Fatal("no event emitted")
	}
}
