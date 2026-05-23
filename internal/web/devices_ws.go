package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/thehappydinoa/signal-go/internal/ws"
)

// LinkDeviceWebSocket issues PUT /v1/devices/link on an open provisioning
// websocket. Signal's production server rejects the REST equivalent with
// HTTP 498 ("use websockets"); the request must use the same connection
// opened for the QR handshake.
func LinkDeviceWebSocket(ctx context.Context, conn *ws.Client, provisioningCode, password string, req LinkDeviceRequest) (*LinkDeviceResponse, error) {
	if conn == nil {
		return nil, errors.New("web.LinkDeviceWebSocket: nil websocket client")
	}
	if provisioningCode == "" {
		return nil, errors.New("web.LinkDeviceWebSocket: missing provisioning code")
	}
	if password == "" {
		return nil, errors.New("web.LinkDeviceWebSocket: missing password")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("web.LinkDeviceWebSocket: marshal body: %w", err)
	}
	hdr := http.Header{}
	hdr.Set("Content-Type", "application/json")
	if h := (Credentials{Username: provisioningCode, Password: password}).Header(); h != "" {
		hdr.Set("Authorization", h)
	}
	resp, err := conn.Send(ctx, http.MethodPut, "/v1/devices/link", ws.HeaderPairs(hdr), body)
	if err != nil {
		return nil, fmt.Errorf("web.LinkDeviceWebSocket: %w", err)
	}
	status := resp.GetStatus()
	if status < 200 || status >= 300 {
		return nil, &Error{
			StatusCode: int(status),
			Status:     fmt.Sprintf("%d %s", status, resp.GetMessage()),
			Body:       resp.GetBody(),
		}
	}
	var out LinkDeviceResponse
	if err := json.Unmarshal(resp.GetBody(), &out); err != nil {
		return nil, fmt.Errorf("web.LinkDeviceWebSocket: decode response: %w", err)
	}
	return &out, nil
}
