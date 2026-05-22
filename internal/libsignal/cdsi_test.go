package libsignal

import "testing"

func TestLookupRequestLifecycle(t *testing.T) {
	req, err := NewLookupRequest()
	if err != nil {
		t.Fatalf("NewLookupRequest: %v", err)
	}
	defer req.Close()

	if err := req.AddE164("+15551234567"); err != nil {
		t.Fatalf("AddE164: %v", err)
	}
	if err := req.AddPreviousE164("+15559876543"); err != nil {
		t.Fatalf("AddPreviousE164: %v", err)
	}
	if err := req.SetToken([]byte{1, 2, 3}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
}

func TestLookupRequestDoubleClose(t *testing.T) {
	req, err := NewLookupRequest()
	if err != nil {
		t.Fatalf("NewLookupRequest: %v", err)
	}
	req.Close()
	req.Close()
}

func TestCDSIResultE164String(t *testing.T) {
	r := CDSIResult{E164: 15551234567}
	if r.E164String() != "+15551234567" {
		t.Fatalf("got %q", r.E164String())
	}
}

func TestTokioAndConnectionManagerLifecycle(t *testing.T) {
	tokio, err := NewTokioAsyncContext()
	if err != nil {
		t.Fatalf("NewTokioAsyncContext: %v", err)
	}
	defer tokio.Close()

	cm, err := NewConnectionManager(NetworkEnvironmentProduction, "signal-go-test")
	if err != nil {
		t.Fatalf("NewConnectionManager: %v", err)
	}
	defer cm.Close()
}
