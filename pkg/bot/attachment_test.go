package bot

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

func TestReplyAttachmentDM(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)

	var fired sync.WaitGroup
	fired.Add(1)
	b.OnText("pic").Do(func(ctx context.Context, m *Message, _ []string) error {
		defer fired.Done()
		return m.ReplyAttachment(ctx, strings.NewReader("img-bytes"), "image/png")
	})

	cancel, wait := runBot(t, b)
	fc.events <- msgEv("alice", "pic")
	fired.Wait()
	cancel()
	_ = wait()

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.attachments) != 1 || fc.attachments[0].to != "alice" {
		t.Fatalf("attachments = %+v", fc.attachments)
	}
	if fc.attachments[0].size != len("img-bytes") {
		t.Fatalf("size = %d", fc.attachments[0].size)
	}
}

func TestMessageAttachmentsMetadata(t *testing.T) {
	ev := &signal.MessageEvent{
		Attachments: []signal.AttachmentMeta{{CDNKey: "k", ContentType: "text/plain"}},
	}
	m := &Message{event: ev, bot: Wrap(newFakeClient())}
	if len(m.Attachments()) != 1 || m.Attachments()[0].CDNKey != "k" {
		t.Fatalf("meta = %+v", m.Attachments())
	}
}

func TestReplyAttachmentReaderConsumed(t *testing.T) {
	fc := newFakeClient()
	b := Wrap(fc)
	body := []byte{1, 2, 3}
	m := &Message{event: &signal.MessageEvent{Sender: "alice"}, bot: b}
	if err := m.ReplyAttachment(context.Background(), bytes.NewReader(body), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if fc.attachments[0].size != 3 {
		t.Fatal("attachment not sent")
	}
}

var _ io.Reader = strings.NewReader("")
