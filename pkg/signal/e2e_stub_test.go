package signal

import (
	"os"
	"testing"
)

// TestIntegrationE2EStub documents the manual e2e suite. When
// SIGNAL_GO_E2E=1 is set but -tags=e2e is missing, it skips toward
// `task test:e2e`. Ordinary `go test ./...` passes without running
// network tests.
func TestIntegrationE2EStub(t *testing.T) {
	if os.Getenv("SIGNAL_GO_E2E") != "1" {
		return
	}
	t.Skip("SIGNAL_GO_E2E=1: run the e2e suite with task test:e2e (go test -tags=e2e -timeout=10m ./...)")
}
