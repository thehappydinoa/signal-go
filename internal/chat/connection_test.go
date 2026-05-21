package chat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/ws"
)

// fakeChatServer simulates Signal's authenticated chat websocket. It
// pushes pre-queued server-initiated requests on connect and accepts
// client responses.
type fakeChatServer struct {
	t         *testing.T
	srv       *httptest.Server
	url       string
	connCount atomic.Int32

	mu       sync.Mutex
	pending  []*wspb.WebSocketRequestMessage
	connHook func(r *http.Request) error
}

func newFakeChatServer(t *testing.T) *fakeChatServer {
	t.Helper()
	f := &fakeChatServer{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	f.url = "ws://" + strings.TrimPrefix(f.srv.URL, "http://")
	return f
}

func (f *fakeChatServer) Close() { f.srv.Close() }

func (f *fakeChatServer) PushRequest(verb, path string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uint64(len(f.pending) + 1)
	f.pending = append(f.pending, &wspb.WebSocketRequestMessage{
		Verb: &verb, Path: &path, Body: body, Id: &id,
	})
}

func (f *fakeChatServer) handle(w http.ResponseWriter, r *http.Request) {
	f.connCount.Add(1)

	if f.connHook != nil {
		if err := f.connHook(r); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
	}

	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		f.t.Logf("accept: %v", err)
		return
	}
	defer c.CloseNow()

	f.mu.Lock()
	reqs := f.pending
	f.pending = nil
	f.mu.Unlock()

	for _, req := range reqs {
		reqType := wspb.WebSocketMessage_REQUEST
		raw, _ := proto.Marshal(&wspb.WebSocketMessage{Type: &reqType, Request: req})
		_ = c.Write(r.Context(), websocket.MessageBinary, raw)
	}

	for {
		_, _, err := c.Read(r.Context())
		if err != nil {
			return
		}
	}
}

func (f *fakeChatServer) dialFunc(_ context.Context, _ string, dopts *ws.DialOptions) (*ws.Client, error) {
	ctx := context.Background()
	return ws.Dial(ctx, f.url, dopts)
}

func TestConnectRequiresFields(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"no ACI", Options{DeviceID: 1, Password: "p", Handler: func(context.Context, *InboundRequest) {}}, "ACI"},
		{"no DeviceID", Options{ACI: "a", Password: "p", Handler: func(context.Context, *InboundRequest) {}}, "DeviceID"},
		{"no Password", Options{ACI: "a", DeviceID: 1, Handler: func(context.Context, *InboundRequest) {}}, "Password"},
		{"no Handler", Options{ACI: "a", DeviceID: 1, Password: "p"}, "Handler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Connect(context.Background(), tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Connect() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestConnectAndReceiveEnvelope(t *testing.T) {
	fake := newFakeChatServer(t)
	defer fake.Close()

	envBody := []byte("test-envelope-body")
	fake.PushRequest("PUT", "/api/v1/message", envBody)

	gotCh := make(chan *InboundRequest, 1)
	handler := func(_ context.Context, req *InboundRequest) {
		gotCh <- req
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, Options{
		ACI:      "test-aci-uuid",
		DeviceID: 2,
		Password: "testpass",
		Handler:  handler,
		DialFunc: fake.dialFunc,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	select {
	case got := <-gotCh:
		if string(got.Body) != string(envBody) {
			t.Errorf("got body %q, want %q", got.Body, envBody)
		}
		if got.Verb != "PUT" || got.Path != "/api/v1/message" {
			t.Errorf("got verb=%q path=%q, want PUT /api/v1/message", got.Verb, got.Path)
		}
	case <-ctx.Done():
		t.Fatal("handler not invoked")
	}
}

func TestConnectSendsAuthHeader(t *testing.T) {
	var gotAuth atomic.Value
	fake := newFakeChatServer(t)
	fake.connHook = func(r *http.Request) error {
		gotAuth.Store(r.Header.Get("Authorization"))
		return nil
	}
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, Options{
		ACI:      "my-aci",
		DeviceID: 3,
		Password: "secret",
		Handler:  func(context.Context, *InboundRequest) {},
		DialFunc: fake.dialFunc,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	auth, _ := gotAuth.Load().(string)
	if auth == "" {
		t.Fatal("Authorization header not sent")
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("got auth %q, want Basic scheme", auth)
	}
}

func TestConnectionCloseIsIdempotent(t *testing.T) {
	fake := newFakeChatServer(t)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, Options{
		ACI:      "a",
		DeviceID: 1,
		Password: "p",
		Handler:  func(context.Context, *InboundRequest) {},
		DialFunc: fake.dialFunc,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestConnectionReconnectsOnFailure(t *testing.T) {
	var attempt atomic.Int32
	dialErr := errors.New("temporary dial failure")

	dialFunc := func(ctx context.Context, url string, opts *ws.DialOptions) (*ws.Client, error) {
		n := attempt.Add(1)
		if n == 1 {
			return ws.Dial(ctx, url, opts)
		}
		if n <= 3 {
			return nil, dialErr
		}
		return ws.Dial(ctx, url, opts)
	}

	fake := newFakeChatServer(t)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := Connect(ctx, Options{
		ACI:            "a",
		DeviceID:       1,
		Password:       "p",
		Handler:        func(context.Context, *InboundRequest) {},
		URL:            fake.url,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     50 * time.Millisecond,
		DialFunc:       dialFunc,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	// Force a disconnect to trigger reconnection attempts.
	conn.mu.Lock()
	client := conn.client
	conn.mu.Unlock()
	_ = client.Close()

	// Wait for reconnect to succeed (attempt 4+).
	deadline := time.After(5 * time.Second)
	for {
		if attempt.Load() >= 4 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for reconnect, attempts=%d", attempt.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestBackoffDelay(t *testing.T) {
	c := &Connection{
		opts: Options{
			InitialBackoff: 1 * time.Second,
			MaxBackoff:     60 * time.Second,
		},
	}

	tests := []struct {
		attempt int
		minD    time.Duration
		maxD    time.Duration
	}{
		{0, 1 * time.Second, 2 * time.Second},
		{1, 2 * time.Second, 3 * time.Second},
		{2, 4 * time.Second, 6 * time.Second},
		{3, 8 * time.Second, 11 * time.Second},
		{10, 60 * time.Second, 76 * time.Second},
	}

	for _, tt := range tests {
		d := c.backoffDelay(tt.attempt)
		if d < tt.minD || d > tt.maxD {
			t.Errorf("attempt %d: got %v, want [%v, %v]", tt.attempt, d, tt.minD, tt.maxD)
		}
	}
}

func TestConnectionQueueEmptyDelivered(t *testing.T) {
	fake := newFakeChatServer(t)
	defer fake.Close()

	fake.PushRequest("PUT", "/api/v1/queue/empty", nil)

	gotCh := make(chan *InboundRequest, 1)
	handler := func(_ context.Context, req *InboundRequest) {
		gotCh <- req
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := Connect(ctx, Options{
		ACI:      "a",
		DeviceID: 1,
		Password: "p",
		Handler:  handler,
		DialFunc: fake.dialFunc,
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer conn.Close()

	select {
	case got := <-gotCh:
		if got.Path != "/api/v1/queue/empty" {
			t.Errorf("got path %q, want /api/v1/queue/empty", got.Path)
		}
	case <-ctx.Done():
		t.Fatal("handler not invoked")
	}
}
