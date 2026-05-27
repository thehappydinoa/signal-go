package signal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/chat"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
	"github.com/thehappydinoa/signal-go/internal/ws"
)

// chatFakeServer simulates Signal's authenticated chat websocket for
// Client tests.
type chatFakeServer struct {
	t   *testing.T
	srv *httptest.Server
	url string
}

func newChatFakeServer(t *testing.T, pushReqs ...*wspb.WebSocketRequestMessage) *chatFakeServer {
	t.Helper()
	f := &chatFakeServer{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Logf("accept: %v", err)
			return
		}
		defer func() { _ = c.CloseNow() }()

		for _, req := range pushReqs {
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
	}))
	f.url = "ws://" + strings.TrimPrefix(f.srv.URL, "http://")
	return f
}

func (f *chatFakeServer) Close() { f.srv.Close() }

func (f *chatFakeServer) dialFunc(_ context.Context, _ string, opts *ws.DialOptions) (*ws.Client, error) {
	return ws.Dial(context.Background(), f.url, opts)
}

func testSignedPreKey() prekeys.SignedPreKey {
	return prekeys.SignedPreKey{
		ID:         1,
		PublicKey:  make([]byte, 33),
		PrivateKey: make([]byte, 32),
		Signature:  make([]byte, 64),
	}
}

func testLastResortKyberPreKey() prekeys.LastResortKyberPreKey {
	return prekeys.LastResortKyberPreKey{
		ID:        1,
		PublicKey: make([]byte, 1568),
		SecretKey: make([]byte, 64),
		Signature: make([]byte, 64),
	}
}

// testOpenOptions builds [OpenOptions] for websocket fakes that push
// already-decoded Content bytes (not real ciphertext).
func testOpenOptions(acctStore account.Store, ss *memstore.SignalStores, dial chat.DialFunc) OpenOptions {
	return OpenOptions{
		AccountStore: acctStore,
		SignalStores: ss,
		Decryptor:    passthroughDecryptor{},
		DialFunc:     dial,
	}
}

func testAccount() *account.Account {
	return &account.Account{
		ACI:      "test-aci-uuid",
		PNI:      "test-pni-uuid",
		Number:   "+15551234567",
		DeviceID: 2,
		Password: "testpassword123",
		ACIIdentity: account.Identity{
			PublicKey:             make([]byte, 33),
			PrivateKey:            make([]byte, 32),
			RegistrationID:        1234,
			SignedPreKey:          testSignedPreKey(),
			LastResortKyberPreKey: testLastResortKyberPreKey(),
			NextPreKeyID:          2,
			NextKyberPreKeyID:     2,
		},
		PNIIdentity: account.Identity{
			PublicKey:             make([]byte, 33),
			PrivateKey:            make([]byte, 32),
			RegistrationID:        5678,
			SignedPreKey:          testSignedPreKey(),
			LastResortKyberPreKey: testLastResortKyberPreKey(),
			NextPreKeyID:          2,
			NextKyberPreKeyID:     2,
		},
	}
}

func makeEnvelopeRequest(t *testing.T, env *sspb.Envelope) *wspb.WebSocketRequestMessage {
	t.Helper()
	body, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	verb := "PUT"
	path := "/api/v1/message"
	id := uint64(1)
	return &wspb.WebSocketRequestMessage{
		Verb: &verb,
		Path: &path,
		Body: body,
		Id:   &id,
	}
}

func TestOpenRequiresFields(t *testing.T) {
	acctStore := memstore.New()
	ss := memstore.NewSignalStores()

	tests := []struct {
		name string
		opts OpenOptions
		want string
	}{
		{"no AccountStore", OpenOptions{SignalStores: ss}, "AccountStore"},
		{"no SignalStores", OpenOptions{AccountStore: acctStore}, "SignalStores"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(context.Background(), tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestOpenReturnsErrNotLinked(t *testing.T) {
	acctStore := memstore.New()
	ss := memstore.NewSignalStores()

	_, err := Open(context.Background(), OpenOptions{
		AccountStore: acctStore,
		SignalStores: ss,
	})
	if err == nil {
		t.Fatal("Open should fail when not linked")
	}
	if !strings.Contains(err.Error(), "not linked") {
		t.Errorf("got %v, want error about 'not linked'", err)
	}
}

func TestOpenAndReceiveDataMessage(t *testing.T) {
	acctStore := memstore.New()
	acct := testAccount()
	if err := acctStore.SaveAccount(acct); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	ss := memstore.NewSignalStores()

	body := "Hello from Signal!"
	ts := uint64(1700000000000)
	srvTS := uint64(1700000001000)
	envType := sspb.Envelope_DOUBLE_RATCHET
	senderACI := "sender-aci-uuid"
	senderDevice := uint32(1)

	contentBytes, _ := proto.Marshal(&sspb.Content{
		Content: &sspb.Content_DataMessage{
			DataMessage: &sspb.DataMessage{
				Body:      &body,
				Timestamp: &ts,
			},
		},
	})

	env := &sspb.Envelope{
		Type:            &envType,
		SourceServiceId: &senderACI,
		SourceDeviceId:  &senderDevice,
		Content:         contentBytes,
		ClientTimestamp: &ts,
		ServerTimestamp: &srvTS,
	}
	envReq := makeEnvelopeRequest(t, env)

	fake := newChatFakeServer(t, envReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		msg, ok := ev.(*MessageEvent)
		if !ok {
			t.Fatalf("got %T, want *MessageEvent", ev)
		}
		if msg.Body != body {
			t.Errorf("got body %q, want %q", msg.Body, body)
		}
		if msg.Sender != senderACI {
			t.Errorf("got sender %q, want %q", msg.Sender, senderACI)
		}
		if msg.SenderDevice != senderDevice {
			t.Errorf("got device %d, want %d", msg.SenderDevice, senderDevice)
		}
	case <-ctx.Done():
		t.Fatal("no event received")
	}
}

func TestOpenAndReceiveReceiptMessage(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	ts := uint64(1700000000000)
	rcptType := sspb.ReceiptMessage_READ
	envType := sspb.Envelope_DOUBLE_RATCHET
	sender := "sender-aci"
	dev := uint32(1)

	contentBytes, _ := proto.Marshal(&sspb.Content{
		Content: &sspb.Content_ReceiptMessage{
			ReceiptMessage: &sspb.ReceiptMessage{
				Type:      &rcptType,
				Timestamp: []uint64{ts},
			},
		},
	})

	env := &sspb.Envelope{
		Type:            &envType,
		SourceServiceId: &sender,
		SourceDeviceId:  &dev,
		Content:         contentBytes,
		ClientTimestamp: &ts,
	}
	envReq := makeEnvelopeRequest(t, env)

	fake := newChatFakeServer(t, envReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		rcpt, ok := ev.(*ReceiptEvent)
		if !ok {
			t.Fatalf("got %T, want *ReceiptEvent", ev)
		}
		if rcpt.Type != ReceiptRead {
			t.Errorf("got type %d, want ReceiptRead", rcpt.Type)
		}
		if len(rcpt.Timestamps) != 1 {
			t.Errorf("got %d timestamps, want 1", len(rcpt.Timestamps))
		}
	case <-ctx.Done():
		t.Fatal("no event received")
	}
}

func TestOpenAndReceiveTypingMessage(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	ts := uint64(1700000000000)
	action := sspb.TypingMessage_STARTED
	envType := sspb.Envelope_DOUBLE_RATCHET
	sender := "sender-aci"
	dev := uint32(1)

	contentBytes, _ := proto.Marshal(&sspb.Content{
		Content: &sspb.Content_TypingMessage{
			TypingMessage: &sspb.TypingMessage{
				Timestamp: &ts,
				Action:    &action,
			},
		},
	})

	env := &sspb.Envelope{
		Type:            &envType,
		SourceServiceId: &sender,
		SourceDeviceId:  &dev,
		Content:         contentBytes,
		ClientTimestamp: &ts,
	}
	envReq := makeEnvelopeRequest(t, env)

	fake := newChatFakeServer(t, envReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		typ, ok := ev.(*TypingEvent)
		if !ok {
			t.Fatalf("got %T, want *TypingEvent", ev)
		}
		if typ.Action != TypingStarted {
			t.Errorf("got action %d, want TypingStarted", typ.Action)
		}
	case <-ctx.Done():
		t.Fatal("no event received")
	}
}

func TestOpenAndReceiveSyncMessage(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	ts := uint64(1700000000000)
	envType := sspb.Envelope_DOUBLE_RATCHET
	sender := "my-aci"
	dev := uint32(1)
	body := "Sent from other device"
	destACI := "dest-aci"

	contentBytes, _ := proto.Marshal(&sspb.Content{
		Content: &sspb.Content_SyncMessage{
			SyncMessage: &sspb.SyncMessage{
				Content: &sspb.SyncMessage_Sent_{
					Sent: &sspb.SyncMessage_Sent{
						DestinationServiceId: &destACI,
						Message: &sspb.DataMessage{
							Body:      &body,
							Timestamp: &ts,
						},
					},
				},
			},
		},
	})

	env := &sspb.Envelope{
		Type:            &envType,
		SourceServiceId: &sender,
		SourceDeviceId:  &dev,
		Content:         contentBytes,
		ClientTimestamp: &ts,
	}
	envReq := makeEnvelopeRequest(t, env)

	fake := newChatFakeServer(t, envReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		sync, ok := ev.(*SyncMessageEvent)
		if !ok {
			t.Fatalf("got %T, want *SyncMessageEvent", ev)
		}
		if sync.SentBody != body {
			t.Errorf("got body %q, want %q", sync.SentBody, body)
		}
		if sync.SentTo != destACI {
			t.Errorf("got dest %q, want %q", sync.SentTo, destACI)
		}
	case <-ctx.Done():
		t.Fatal("no event received")
	}
}

func TestOpenServerDeliveryReceiptNoDecryptError(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	ts := uint64(1700000000000)
	envType := sspb.Envelope_SERVER_DELIVERY_RECEIPT
	sender := "sender-aci"
	dev := uint32(1)

	env := &sspb.Envelope{
		Type:            &envType,
		SourceServiceId: &sender,
		SourceDeviceId:  &dev,
		ClientTimestamp: &ts,
	}
	envReq := makeEnvelopeRequest(t, env)

	fake := newChatFakeServer(t, envReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		t.Fatalf("unexpected event %T (%v)", ev, ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestOpenDecryptionErrorEmitsEvent(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	ts := uint64(1700000000000)
	envType := sspb.Envelope_DOUBLE_RATCHET
	sender := "sender-aci"
	dev := uint32(1)

	env := &sspb.Envelope{
		Type:            &envType,
		SourceServiceId: &sender,
		SourceDeviceId:  &dev,
		Content:         []byte("this-is-not-valid-protobuf-content!@#$%"),
		ClientTimestamp: &ts,
	}
	envReq := makeEnvelopeRequest(t, env)

	fake := newChatFakeServer(t, envReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		decErr, ok := ev.(*DecryptionErrorEvent)
		if !ok {
			t.Fatalf("got %T, want *DecryptionErrorEvent", ev)
		}
		if decErr.Err == nil {
			t.Error("expected non-nil error")
		}
		if decErr.Sender != sender {
			t.Errorf("got sender %q, want %q", decErr.Sender, sender)
		}
	case <-ctx.Done():
		t.Fatal("no event received")
	}
}

func TestOpenQueueEmpty(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	verb := "PUT"
	path := "/api/v1/queue/empty"
	id := uint64(1)
	queueReq := &wspb.WebSocketRequestMessage{
		Verb: &verb,
		Path: &path,
		Id:   &id,
	}

	fake := newChatFakeServer(t, queueReq)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	select {
	case ev := <-client.Events():
		_, ok := ev.(*QueueEmptyEvent)
		if !ok {
			t.Fatalf("got %T, want *QueueEmptyEvent", ev)
		}
	case <-ctx.Done():
		t.Fatal("no event received")
	}
}

func TestClientCloseClosesEvents(t *testing.T) {
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(testAccount()); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()

	fake := newChatFakeServer(t)
	defer fake.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Open(ctx, testOpenOptions(acctStore, ss, fake.dialFunc))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	client.Close()

	_, ok := <-client.Events()
	if ok {
		t.Error("Events channel should be closed after Close")
	}
}

func TestMsToTime(t *testing.T) {
	tests := []struct {
		ms   uint64
		want time.Time
	}{
		{0, time.Time{}},
		{1700000000000, time.UnixMilli(1700000000000)},
		{1, time.UnixMilli(1)},
	}
	for _, tt := range tests {
		got := msToTime(tt.ms)
		if !got.Equal(tt.want) {
			t.Errorf("msToTime(%d) = %v, want %v", tt.ms, got, tt.want)
		}
	}
}
