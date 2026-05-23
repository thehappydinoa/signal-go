package backup

import (
	"testing"

	"google.golang.org/protobuf/proto"

	backuppbg "github.com/thehappydinoa/signal-go/internal/proto/gen/backuppbg"
)

func TestImportFrameChatItemCallback(t *testing.T) {
	frame := &backuppbg.Frame{
		Item: &backuppbg.Frame_ChatItem{
			ChatItem: &backuppbg.ChatItem{
				AuthorId: 99,
			},
		},
	}
	wire, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}

	var stats ImportStats
	var got [][]byte
	target := ImportTarget{
		OnChatItem: func(b []byte) error {
			got = append(got, append([]byte(nil), b...))
			return nil
		},
	}
	if err := importFrame(wire, target, &stats); err != nil {
		t.Fatalf("importFrame: %v", err)
	}
	if stats.ChatItemsProcessed != 1 {
		t.Fatalf("ChatItemsProcessed = %d, want 1", stats.ChatItemsProcessed)
	}
	if len(got) != 1 {
		t.Fatalf("callback invocations = %d, want 1", len(got))
	}
	var round backuppbg.ChatItem
	if err := proto.Unmarshal(got[0], &round); err != nil {
		t.Fatalf("unmarshal callback payload: %v", err)
	}
	if round.GetAuthorId() != 99 {
		t.Fatalf("author id = %d", round.GetAuthorId())
	}
}

func TestImportFrameChatItemWithoutCallbackStillCounts(t *testing.T) {
	frame := &backuppbg.Frame{
		Item: &backuppbg.Frame_ChatItem{
			ChatItem: &backuppbg.ChatItem{},
		},
	}
	wire, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	var stats ImportStats
	if err := importFrame(wire, ImportTarget{}, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.ChatItemsProcessed != 1 {
		t.Fatalf("ChatItemsProcessed = %d, want 1", stats.ChatItemsProcessed)
	}
}
