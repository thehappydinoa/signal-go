package ws

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
)

func TestMergeTLSConfigEnforcesMinVersion(t *testing.T) {
	// nil base → fresh config with MinVersion locked to 1.2.
	got := mergeTLSConfig(nil)
	if got.MinVersion != MinTLSVersion {
		t.Errorf("nil base: MinVersion = 0x%x, want 0x%x", got.MinVersion, MinTLSVersion)
	}
	// Caller TLS 1.0 is silently raised; ServerName preserved.
	base := &tls.Config{ServerName: "chat.signal.org", MinVersion: tls.VersionTLS10}
	got = mergeTLSConfig(base)
	if got == base {
		t.Error("mergeTLSConfig must return a clone, not the caller's config")
	}
	if got.MinVersion != MinTLSVersion {
		t.Errorf("low MinVersion: got 0x%x, want 0x%x", got.MinVersion, MinTLSVersion)
	}
	if got.ServerName != "chat.signal.org" {
		t.Errorf("ServerName dropped: %q", got.ServerName)
	}
	// Caller TLS 1.3 stays as 1.3.
	base = &tls.Config{MinVersion: tls.VersionTLS13}
	got = mergeTLSConfig(base)
	if got.MinVersion != tls.VersionTLS13 {
		t.Errorf("high MinVersion: got 0x%x, want 0x%x", got.MinVersion, tls.VersionTLS13)
	}
}

// fakeSignalServer answers WebSocketMessage requests with deterministic
// responses. It mirrors what chat.signal.org does over the provisioning ws.
type fakeSignalServer struct {
	t               *testing.T
	respondTo       func(req *wspb.WebSocketRequestMessage) (status uint32, body []byte)
	serverInitReqs  []*wspb.WebSocketRequestMessage
	serverInitReqMu sync.Mutex
	srv             *httptest.Server
	url             string
}

func newFakeSignalServer(t *testing.T, respond func(req *wspb.WebSocketRequestMessage) (uint32, []byte)) *fakeSignalServer {
	t.Helper()
	f := &fakeSignalServer{t: t, respondTo: respond}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	f.url = "ws://" + strings.TrimPrefix(f.srv.URL, "http://")
	return f
}

func (f *fakeSignalServer) Close() { f.srv.Close() }

func (f *fakeSignalServer) handle(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		f.t.Errorf("accept: %v", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	// Push any pre-queued server-initiated requests.
	f.serverInitReqMu.Lock()
	pending := f.serverInitReqs
	f.serverInitReqs = nil
	f.serverInitReqMu.Unlock()
	for _, req := range pending {
		reqType := wspb.WebSocketMessage_REQUEST
		raw, _ := proto.Marshal(&wspb.WebSocketMessage{Type: &reqType, Request: req})
		_ = c.Write(r.Context(), websocket.MessageBinary, raw)
	}

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
		if msg.GetType() == wspb.WebSocketMessage_REQUEST {
			status, body := f.respondTo(msg.GetRequest())
			id := msg.GetRequest().GetId()
			respType := wspb.WebSocketMessage_RESPONSE
			out := &wspb.WebSocketMessage{
				Type: &respType,
				Response: &wspb.WebSocketResponseMessage{
					Id:     &id,
					Status: &status,
					Body:   body,
				},
			}
			raw, _ := proto.Marshal(out)
			_ = c.Write(r.Context(), websocket.MessageBinary, raw)
		}
	}
}

func (f *fakeSignalServer) PushRequest(verb, path string, body []byte) {
	f.serverInitReqMu.Lock()
	defer f.serverInitReqMu.Unlock()
	id := uint64(len(f.serverInitReqs) + 1)
	f.serverInitReqs = append(f.serverInitReqs, &wspb.WebSocketRequestMessage{
		Verb: &verb, Path: &path, Body: body, Id: &id,
	})
}

func TestClientSendReceive(t *testing.T) {
	fake := newFakeSignalServer(t, func(req *wspb.WebSocketRequestMessage) (uint32, []byte) {
		if req.GetPath() == "/v1/health" {
			return 200, []byte("ok")
		}
		return 404, nil
	})
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli, err := Dial(ctx, fake.url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer cli.Close()

	resp, err := cli.Send(ctx, "GET", "/v1/health", nil, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.GetStatus() != 200 || string(resp.GetBody()) != "ok" {
		t.Errorf("got status=%d body=%q, want 200 'ok'", resp.GetStatus(), resp.GetBody())
	}

	resp2, err := cli.Send(ctx, "GET", "/missing", nil, nil)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp2.GetStatus() != 404 {
		t.Errorf("got status=%d, want 404", resp2.GetStatus())
	}
}

func TestClientServerInitiatedRequestRoutesToHandler(t *testing.T) {
	fake := newFakeSignalServer(t, func(req *wspb.WebSocketRequestMessage) (uint32, []byte) {
		return 200, nil
	})
	fake.PushRequest("PUT", "/v1/message", []byte("from-server"))
	defer fake.Close()

	gotBody := make(chan []byte, 1)
	handler := RequestHandlerFunc(func(_ context.Context, req *wspb.WebSocketRequestMessage) (uint32, string, []byte, error) {
		gotBody <- req.GetBody()
		return 200, "OK", nil, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli, err := Dial(ctx, fake.url, &DialOptions{Handler: handler})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer cli.Close()

	select {
	case got := <-gotBody:
		if string(got) != "from-server" {
			t.Errorf("got %q, want %q", got, "from-server")
		}
	case <-ctx.Done():
		t.Fatal("handler not invoked")
	}
}

func TestClientSendErrorsAfterClose(t *testing.T) {
	fake := newFakeSignalServer(t, func(_ *wspb.WebSocketRequestMessage) (uint32, []byte) { return 200, nil })
	defer fake.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli, err := Dial(ctx, fake.url, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	cli.Close()
	if _, err := cli.Send(ctx, "GET", "/", nil, nil); err == nil {
		t.Fatal("expected error sending on closed client")
	}
}
