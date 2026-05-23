//go:build live

package provisioning

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/web/useragent"
	"github.com/thehappydinoa/signal-go/internal/ws"
)

// TestLiveProvisioningDial probes chat.signal.org (manual: go test -tags=live -run TestLiveProvisioningDial -timeout=30s).
func TestLiveProvisioningDial(t *testing.T) {
	agents := []struct {
		name string
		ua   string
	}{
		{"signal-go", useragent.Resolve(useragent.SignalGo, "", useragent.Options{})},
		{"desktop-windows", useragent.DesktopWindows.Format(useragent.Options{})},
	}
	for _, tc := range agents {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, err := ws.Dial(ctx, DefaultProvisioningURL, &ws.DialOptions{
				Header: newSignalHeaders(tc.ua),
			})
			if err != nil {
				t.Fatalf("ws.Dial(%q): %v", tc.ua, err)
			}
		})
	}
}

// TestLiveProvisioningDialHeaders documents headers accepted by the server.
func TestLiveProvisioningDialHeaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h := http.Header{}
	h.Set("X-Signal-Agent", "OWD")
	h.Set("User-Agent", useragent.DesktopWindows.Format(useragent.Options{}))
	_, err := ws.Dial(ctx, DefaultProvisioningURL, &ws.DialOptions{Header: h})
	if err != nil {
		t.Fatalf("desktop split headers: %v", err)
	}
}
