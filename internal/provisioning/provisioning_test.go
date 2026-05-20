package provisioning

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
)

func TestBuildLinkURL(t *testing.T) {
	got, err := buildLinkURL("abc-123", []byte{0x05, 0xde, 0xad, 0xbe, 0xef}, []string{"cap1", "cap2"})
	if err != nil {
		t.Fatalf("buildLinkURL: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Scheme != "sgnl" || u.Host != "linkdevice" {
		t.Errorf("scheme/host = %q/%q, want sgnl/linkdevice", u.Scheme, u.Host)
	}
	q := u.Query()
	if q.Get("uuid") != "abc-123" {
		t.Errorf("uuid = %q", q.Get("uuid"))
	}
	pub, err := base64.URLEncoding.DecodeString(q.Get("pub_key"))
	if err != nil {
		t.Errorf("pub_key not base64url: %v", err)
	} else if string(pub) != string([]byte{0x05, 0xde, 0xad, 0xbe, 0xef}) {
		t.Errorf("pub_key round-trip mismatch: %x", pub)
	}
	if q.Get("capabilities") != "cap1,cap2" {
		t.Errorf("capabilities = %q", q.Get("capabilities"))
	}
}

func TestBuildLinkURLRejectsEmpty(t *testing.T) {
	if _, err := buildLinkURL("", []byte{1}, nil); err == nil {
		t.Error("expected error for empty address")
	}
	if _, err := buildLinkURL("x", nil, nil); err == nil {
		t.Error("expected error for empty public key")
	}
}

// fakeProvisioningServer impersonates chat.signal.org's provisioning ws
// endpoint. It sends ProvisioningAddress, waits for the test to push an
// envelope, and verifies both REQUESTs were acked.
type fakeProvisioningServer struct {
	t            *testing.T
	srv          *httptest.Server
	addressUUID  string
	envelopeChan chan *wspb.WebSocketRequestMessage
	gotAck       chan struct{}
}

func newFakeProvisioningServer(t *testing.T, addressUUID string) *fakeProvisioningServer {
	t.Helper()
	f := &fakeProvisioningServer{
		t:            t,
		addressUUID:  addressUUID,
		envelopeChan: make(chan *wspb.WebSocketRequestMessage, 1),
		gotAck:       make(chan struct{}, 2),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

// PushEnvelope queues an envelope to be sent after the address ack.
func (f *fakeProvisioningServer) PushEnvelope(env *provpb.ProvisionEnvelope) {
	body, _ := proto.Marshal(env)
	verb := "PUT"
	path := "/v1/message"
	id := uint64(2)
	f.envelopeChan <- &wspb.WebSocketRequestMessage{
		Verb: &verb, Path: &path, Body: body, Id: &id,
	}
}

func (f *fakeProvisioningServer) URL() string {
	return "ws://" + strings.TrimPrefix(f.srv.URL, "http://")
}

func (f *fakeProvisioningServer) Close() { f.srv.Close() }

func (f *fakeProvisioningServer) handle(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		f.t.Errorf("accept: %v", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Step 1: send ProvisioningAddress.
	addrBody, _ := proto.Marshal(&provpb.ProvisioningAddress{Address: &f.addressUUID})
	addrVerb, addrPath, addrID := "PUT", "/v1/address", uint64(1)
	reqType := wspb.WebSocketMessage_REQUEST
	addrMsg := &wspb.WebSocketMessage{
		Type: &reqType,
		Request: &wspb.WebSocketRequestMessage{
			Verb: &addrVerb, Path: &addrPath, Body: addrBody, Id: &addrID,
		},
	}
	raw, _ := proto.Marshal(addrMsg)
	if err := c.Write(r.Context(), websocket.MessageBinary, raw); err != nil {
		return
	}

	// Read until we've received both acks.
	for {
		_, data, err := c.Read(r.Context())
		if err != nil {
			return
		}
		var msg wspb.WebSocketMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			f.t.Errorf("unmarshal: %v", err)
			return
		}
		if msg.GetType() == wspb.WebSocketMessage_RESPONSE && msg.GetResponse().GetStatus() == 200 {
			f.gotAck <- struct{}{}
			// After the address ack, push the envelope as soon as it's available.
			if msg.GetResponse().GetId() == addrID {
				go func() {
					select {
					case env := <-f.envelopeChan:
						out, _ := proto.Marshal(&wspb.WebSocketMessage{Type: &reqType, Request: env})
						_ = c.Write(r.Context(), websocket.MessageBinary, out)
					case <-r.Context().Done():
					}
				}()
			}
		}
	}
}

func TestLinkHappyPath(t *testing.T) {
	const wantAddress = "deadbeef-aaaa-bbbb-cccc-eeeeffffffff"
	fake := newFakeProvisioningServer(t, wantAddress)
	defer fake.Close()

	// To round-trip the envelope through Link's internal decrypt we have to
	// know which secondary public key the primary side encrypts to. Inject
	// the secondary's ephemeral key so we can compute the matching envelope.
	secondary, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate secondary: %v", err)
	}
	primary, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate primary: %v", err)
	}
	aci := "11111111-2222-3333-4444-555555555555"
	num := "+15551234567"
	code := "ABCD-1234"
	wantMsg := &provpb.ProvisionMessage{
		Aci:              &aci,
		Number:           &num,
		ProvisioningCode: &code,
	}
	env := encryptForTest(t, primary.Private, primary.Public, secondary.Public, wantMsg)
	fake.PushEnvelope(env)

	gotURLCh := make(chan string, 1)
	var onURLOnce sync.Once
	opts := Options{
		URL:          fake.URL(),
		EphemeralKey: secondary,
		OnURL: func(linkURL string) error {
			onURLOnce.Do(func() { gotURLCh <- linkURL })
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := Link(ctx, opts)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if sess.EphemeralKey == nil {
		t.Fatal("nil ephemeral key in session")
	}
	if sess.Message.GetAci() != aci || sess.Message.GetNumber() != num || sess.Message.GetProvisioningCode() != code {
		t.Errorf("decoded message mismatch: %+v", sess.Message)
	}

	select {
	case linkURL := <-gotURLCh:
		u, err := url.Parse(linkURL)
		if err != nil {
			t.Fatalf("link URL parse: %v", err)
		}
		if u.Query().Get("uuid") != wantAddress {
			t.Errorf("URL uuid = %q, want %q", u.Query().Get("uuid"), wantAddress)
		}
		if u.Query().Get("pub_key") == "" {
			t.Error("URL has no pub_key")
		}
	default:
		t.Fatal("OnURL was not called")
	}
}

func TestLinkRequiresOnURL(t *testing.T) {
	_, err := Link(context.Background(), Options{})
	if err == nil || !strings.Contains(err.Error(), "OnURL") {
		t.Errorf("err = %v, want one mentioning OnURL", err)
	}
}

func TestLinkContextCancellation(t *testing.T) {
	// Server sends the address but never an envelope.
	fake := newFakeProvisioningServer(t, "addr")
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := Link(ctx, Options{
		URL:   fake.URL(),
		OnURL: func(string) error { return nil },
	})
	if err == nil {
		t.Fatal("expected ctx error")
	}
}

func TestLinkUsesInjectedKey(t *testing.T) {
	kp, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	wantPub, _ := kp.Public.Serialize()

	primary, _ := libsignal.GenerateIdentityKeyPair()
	num := "+1"
	env := encryptForTest(t, primary.Private, primary.Public, kp.Public, &provpb.ProvisionMessage{Number: &num})

	fake := newFakeProvisioningServer(t, "addr2")
	defer fake.Close()
	fake.PushEnvelope(env)

	var gotPub []byte
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = Link(ctx, Options{
		URL:          fake.URL(),
		EphemeralKey: kp,
		OnURL: func(linkURL string) error {
			u, perr := url.Parse(linkURL)
			if perr != nil {
				return perr
			}
			gotPub, _ = base64.URLEncoding.DecodeString(u.Query().Get("pub_key"))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if string(gotPub) != string(wantPub) {
		t.Errorf("URL pub_key %x, want %x", gotPub, wantPub)
	}
}

func bytesPattern(n int, b byte) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
