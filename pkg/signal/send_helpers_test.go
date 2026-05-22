package signal

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

func TestBuildReceiptContentShape(t *testing.T) {
	cases := []struct {
		name string
		kind ReceiptType
		want sspb.ReceiptMessage_Type
	}{
		{"delivery", ReceiptDelivery, sspb.ReceiptMessage_DELIVERY},
		{"read", ReceiptRead, sspb.ReceiptMessage_READ},
		{"viewed", ReceiptViewed, sspb.ReceiptMessage_VIEWED},
	}
	now := time.Now().Truncate(time.Millisecond)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := buildReceiptContent(tc.kind, []time.Time{now})
			if err != nil {
				t.Fatalf("buildReceiptContent: %v", err)
			}
			var c sspb.Content
			if err := proto.Unmarshal(b, &c); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			rm := c.GetReceiptMessage()
			if rm == nil {
				t.Fatalf("Content has no ReceiptMessage: %+v", &c)
			}
			if rm.GetType() != tc.want {
				t.Errorf("Type = %v, want %v", rm.GetType(), tc.want)
			}
			if got := rm.GetTimestamp(); len(got) != 1 || got[0] != uint64(now.UnixMilli()) {
				t.Errorf("Timestamp = %v, want [%d]", got, now.UnixMilli())
			}
		})
	}
}

func TestBuildReceiptContentRejectsZeroTimestamp(t *testing.T) {
	if _, err := buildReceiptContent(ReceiptRead, []time.Time{{}}); err == nil {
		t.Error("expected error for zero timestamp")
	}
}

func TestBuildTypingContentShape(t *testing.T) {
	ts := uint64(time.Now().UnixMilli())
	b, err := buildTypingContent(TypingStarted, ts, nil)
	if err != nil {
		t.Fatalf("buildTypingContent: %v", err)
	}
	var c sspb.Content
	if err := proto.Unmarshal(b, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tm := c.GetTypingMessage()
	if tm == nil {
		t.Fatalf("Content has no TypingMessage: %+v", &c)
	}
	if tm.GetAction() != sspb.TypingMessage_STARTED {
		t.Errorf("Action = %v, want STARTED", tm.GetAction())
	}
	if tm.GetTimestamp() != ts {
		t.Errorf("Timestamp = %d, want %d", tm.GetTimestamp(), ts)
	}
}

func TestBuildReactionContentShape(t *testing.T) {
	target := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	ts := uint64(time.Now().UnixMilli())
	b, err := buildReactionContent("👍", "bob-aci", target, false, ts)
	if err != nil {
		t.Fatalf("buildReactionContent: %v", err)
	}
	var c sspb.Content
	if err := proto.Unmarshal(b, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dm := c.GetDataMessage()
	if dm == nil {
		t.Fatalf("Content has no DataMessage: %+v", &c)
	}
	r := dm.GetReaction()
	if r == nil {
		t.Fatalf("DataMessage has no Reaction: %+v", dm)
	}
	if r.GetEmoji() != "👍" {
		t.Errorf("Emoji = %q, want 👍", r.GetEmoji())
	}
	if r.GetRemove() {
		t.Error("Remove should be false")
	}
	if r.GetTargetAuthorAci() != "bob-aci" {
		t.Errorf("TargetAuthorAci = %q", r.GetTargetAuthorAci())
	}
	if r.GetTargetSentTimestamp() != uint64(target.UnixMilli()) {
		t.Errorf("TargetSentTimestamp = %d", r.GetTargetSentTimestamp())
	}
}

func TestBuildGroupReactionContentIncludesGroupV2(t *testing.T) {
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	target := time.Now().Add(-time.Minute).Truncate(time.Millisecond)
	ts := uint64(time.Now().UnixMilli())
	const revision uint32 = 7

	b, err := buildGroupReactionContent("👍", "bob-aci", target, false, ts, masterKey, revision)
	if err != nil {
		t.Fatalf("buildGroupReactionContent: %v", err)
	}
	var c sspb.Content
	if err := proto.Unmarshal(b, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dm := c.GetDataMessage()
	if dm == nil || dm.GetReaction() == nil {
		t.Fatalf("missing reaction DataMessage: %+v", &c)
	}
	gv2 := dm.GetGroupV2()
	if gv2 == nil {
		t.Fatal("missing GroupV2")
	}
	if string(gv2.GetMasterKey()) != string(masterKey) {
		t.Errorf("MasterKey mismatch")
	}
	if gv2.GetRevision() != revision {
		t.Errorf("Revision = %d, want %d", gv2.GetRevision(), revision)
	}
}

func TestBuildGroupTypingContentIncludesGroupID(t *testing.T) {
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i + 2)
	}
	ts := uint64(time.Now().UnixMilli())
	b, err := buildGroupTypingContent(TypingStarted, ts, masterKey)
	if err != nil {
		t.Fatalf("buildGroupTypingContent: %v", err)
	}
	var c sspb.Content
	if err := proto.Unmarshal(b, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tm := c.GetTypingMessage()
	if tm == nil {
		t.Fatal("missing TypingMessage")
	}
	if len(tm.GetGroupId()) != 32 {
		t.Fatalf("GroupId length = %d, want 32", len(tm.GetGroupId()))
	}
}

func TestSendGroupReactionInputValidation(t *testing.T) {
	c := &Client{}
	mk := make([]byte, 32)
	now := time.Now()
	if _, err := c.SendGroupReaction(context.Background(), mk[:16], "👍", "bob", now, false); err == nil {
		t.Error("expected error for short master key")
	}
	if _, err := c.SendGroupReaction(context.Background(), mk, "👍", "", now, false); err == nil {
		t.Error("expected error for empty target author")
	}
}

func TestSendReceiptInputValidation(t *testing.T) {
	c := &Client{}
	if _, err := c.SendReceipt(context.Background(), "", ReceiptRead, []time.Time{time.Now()}); err == nil {
		t.Error("expected error empty recipient")
	}
	if _, err := c.SendReceipt(context.Background(), "bob", ReceiptRead, nil); err == nil {
		t.Error("expected error empty timestamps")
	}
}

func TestSendReactionInputValidation(t *testing.T) {
	c := &Client{}
	now := time.Now()
	if _, err := c.SendReaction(context.Background(), "", "👍", "bob", now, false); err == nil {
		t.Error("expected error empty recipient")
	}
	if _, err := c.SendReaction(context.Background(), "bob", "👍", "", now, false); err == nil {
		t.Error("expected error empty target author")
	}
	if _, err := c.SendReaction(context.Background(), "bob", "👍", "alice", time.Time{}, false); err == nil {
		t.Error("expected error zero target timestamp")
	}
	if _, err := c.SendReaction(context.Background(), "bob", "", "alice", now, false); err == nil {
		t.Error("expected error empty emoji when not removing")
	}
}

func TestSendTypingInputValidation(t *testing.T) {
	c := &Client{}
	if _, err := c.SendTyping(context.Background(), "", TypingStarted); err == nil {
		t.Error("expected error empty recipient")
	}
}
