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
