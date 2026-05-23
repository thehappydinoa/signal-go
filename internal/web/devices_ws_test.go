package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
)

func TestServiceWebSocketURL(t *testing.T) {
	tests := []struct {
		apiBase string
		want    string
	}{
		{"", DefaultServiceWebSocketURL},
		{"https://chat.signal.org", "wss://chat.signal.org/v1/websocket/"},
		{"http://127.0.0.1:8080", "ws://127.0.0.1:8080/v1/websocket/"},
	}
	for _, tc := range tests {
		if got := ServiceWebSocketURL(tc.apiBase); got != tc.want {
			t.Errorf("ServiceWebSocketURL(%q) = %q, want %q", tc.apiBase, got, tc.want)
		}
	}
}

func TestLinkDeviceWebSocket(t *testing.T) {
	var gotReq LinkDeviceRequest
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		for {
			_, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			var msg wspb.WebSocketMessage
			if err := proto.Unmarshal(data, &msg); err != nil {
				t.Errorf("unmarshal: %v", err)
				return
			}
			if msg.GetType() != wspb.WebSocketMessage_REQUEST {
				continue
			}
			req := msg.GetRequest()
			if req.GetVerb() != http.MethodPut || req.GetPath() != "/v1/devices/link" {
				t.Fatalf("unexpected %s %s", req.GetVerb(), req.GetPath())
			}
			_ = json.Unmarshal(req.GetBody(), &gotReq)
			for _, h := range req.GetHeaders() {
				if strings.HasPrefix(h, "Authorization:") {
					gotAuth = strings.TrimSpace(strings.TrimPrefix(h, "Authorization:"))
				}
			}
			body, _ := json.Marshal(LinkDeviceResponse{UUID: "aci-uuid", DeviceID: 7, PNI: "pni-uuid"})
			status := uint32(200)
			respType := wspb.WebSocketMessage_RESPONSE
			id := req.GetId()
			out, _ := proto.Marshal(&wspb.WebSocketMessage{
				Type: &respType,
				Response: &wspb.WebSocketResponseMessage{
					Id: &id, Status: &status, Body: body,
				},
			})
			_ = c.Write(r.Context(), websocket.MessageBinary, out)
			return
		}
	}))
	defer srv.Close()

	wsURL := "ws://" + strings.TrimPrefix(srv.URL, "http://")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := LinkDeviceWebSocket(ctx, wsURL, "test-ua", "+15551234567", "passw0rd", LinkDeviceRequest{VerificationCode: "code-xyz"})
	if err != nil {
		t.Fatalf("LinkDeviceWebSocket: %v", err)
	}
	if got.UUID != "aci-uuid" || got.DeviceID != 7 {
		t.Fatalf("response = %+v", got)
	}
	if gotReq.VerificationCode != "code-xyz" {
		t.Fatalf("server saw verificationCode = %q", gotReq.VerificationCode)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("auth = %q", gotAuth)
	}
}
