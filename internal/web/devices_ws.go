package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/thehappydinoa/signal-go/internal/debugsession"
	"github.com/thehappydinoa/signal-go/internal/ws"
)

// DefaultServiceWebSocketURL is Signal's unauthenticated chat websocket.
// PUT /v1/devices/link must use this endpoint (not the provisioning socket).
const DefaultServiceWebSocketURL = "wss://chat.signal.org/v1/websocket/"

// ServiceWebSocketURL derives the service websocket URL from a REST API base.
func ServiceWebSocketURL(apiBase string) string {
	if apiBase == "" {
		return DefaultServiceWebSocketURL
	}
	u, err := url.Parse(apiBase)
	if err != nil || u.Host == "" {
		return DefaultServiceWebSocketURL
	}
	scheme := "wss"
	if u.Scheme == "http" || u.Scheme == "ws" {
		scheme = "ws"
	}
	return scheme + "://" + u.Host + "/v1/websocket/"
}

// LinkDeviceWebSocket issues PUT /v1/devices/link on the unauthenticated
// service websocket. Signal's production server rejects REST with HTTP 498
// ("use websockets") and returns 404 if the request is sent on the
// provisioning websocket (/v1/websocket/provisioning/).
func LinkDeviceWebSocket(ctx context.Context, serviceWSURL, userAgent, number, password string, req LinkDeviceRequest) (*LinkDeviceResponse, error) {
	if serviceWSURL == "" {
		serviceWSURL = DefaultServiceWebSocketURL
	}
	if number == "" {
		return nil, errors.New("web.LinkDeviceWebSocket: missing phone number")
	}
	if password == "" {
		return nil, errors.New("web.LinkDeviceWebSocket: missing password")
	}
	if req.VerificationCode == "" {
		return nil, errors.New("web.LinkDeviceWebSocket: missing verification code in request")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("web.LinkDeviceWebSocket: marshal body: %w", err)
	}
	dialHdr := http.Header{}
	if userAgent != "" {
		dialHdr.Set("User-Agent", userAgent)
		dialHdr.Set("X-Signal-Agent", userAgent)
	}
	// #region agent log
	debugsession.Log("H7", "web/devices_ws.go:LinkDeviceWebSocket", "dialing service websocket", map[string]any{
		"serviceWSURL": serviceWSURL, "runId": "post-fix",
	})
	// #endregion
	conn, err := ws.Dial(ctx, serviceWSURL, &ws.DialOptions{Header: dialHdr})
	if err != nil {
		return nil, fmt.Errorf("web.LinkDeviceWebSocket: dial: %w", err)
	}
	defer conn.Close()

	reqHdr := http.Header{}
	reqHdr.Set("Content-Type", "application/json")
	if h := (Credentials{Username: number, Password: password}).Header(); h != "" {
		reqHdr.Set("Authorization", h)
	}
	resp, err := conn.Send(ctx, http.MethodPut, "/v1/devices/link", ws.HeaderPairs(reqHdr), body)
	if err != nil {
		return nil, fmt.Errorf("web.LinkDeviceWebSocket: %w", err)
	}
	status := resp.GetStatus()
	if status < 200 || status >= 300 {
		// #region agent log
		bodyPreview := resp.GetBody()
		if len(bodyPreview) > 128 {
			bodyPreview = bodyPreview[:128]
		}
		debugsession.Log("H6", "web/devices_ws.go:LinkDeviceWebSocket", "link response error", map[string]any{
			"status": status, "message": resp.GetMessage(), "bodyLen": len(resp.GetBody()),
			"bodyPreview": string(bodyPreview), "runId": "post-fix",
		})
		// #endregion
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
	// #region agent log
	debugsession.Log("H7", "web/devices_ws.go:LinkDeviceWebSocket", "link succeeded", map[string]any{
		"deviceId": out.DeviceID, "runId": "post-fix",
	})
	// #endregion
	return &out, nil
}
