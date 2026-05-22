package signal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/attachment"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	"github.com/thehappydinoa/signal-go/internal/web"
)

func TestDownloadAttachmentRoundTrip(t *testing.T) {
	key, err := attachment.NewKey()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello attachment")
	enc, err := attachment.EncryptV2(plain, key, "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/attachments/k1" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(enc.Ciphertext)
	}))
	defer srv.Close()

	webc := web.New("", "test")
	webc.CDNHosts = map[uint32]string{3: srv.URL}
	cli := &Client{webc: webc}

	meta := AttachmentMeta{
		CDNKey:    "k1",
		CDNNumber: 3,
		Key:       key,
		Digest:    enc.Digest,
		Size:      uint32(len(plain)),
	}
	got, err := cli.DownloadAttachment(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q", got)
	}
}

func TestDispatchDataMessageAttachments(t *testing.T) {
	c := newDispatchClient()
	key := make([]byte, 64)
	digest := make([]byte, 32)
	cdnKey := "file-key"
	size := uint32(99)
	ct := "image/png"
	dm := &sspb.DataMessage{
		Attachments: []*sspb.AttachmentPointer{{
			AttachmentIdentifier: &sspb.AttachmentPointer_CdnKey{CdnKey: cdnKey},
			Key:                  key,
			Digest:               digest,
			Size:                 &size,
			ContentType:          &ct,
		}},
	}
	content := &sspb.Content{Content: &sspb.Content_DataMessage{DataMessage: dm}}
	c.dispatchContent("alice", 1, time.Time{}, time.Time{}, content)

	ev := <-c.events
	me, ok := ev.(*MessageEvent)
	if !ok {
		t.Fatalf("event = %T", ev)
	}
	if len(me.Attachments) != 1 {
		t.Fatalf("attachments = %d", len(me.Attachments))
	}
	if me.Attachments[0].CDNKey != cdnKey || me.Attachments[0].ContentType != ct {
		t.Fatalf("meta = %+v", me.Attachments[0])
	}
}
