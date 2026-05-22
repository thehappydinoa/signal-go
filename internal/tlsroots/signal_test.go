package tlsroots

import (
	"crypto/x509"
	"testing"
)

func TestIsSignalHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"chat.signal.org", true},
		{"CHAT.SIGNAL.ORG.", true},
		{"cdn3.signal.org", true},
		{"signal.org", true},
		{"example.com", false},
		{"notsignal.org", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsSignalHost(tc.host); got != tc.want {
			t.Errorf("IsSignalHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestSignalRootCAsFingerprint(t *testing.T) {
	pool, err := SignalRootCAs()
	if err != nil {
		t.Fatalf("SignalRootCAs: %v", err)
	}
	if pool == nil {
		t.Fatal("pool is nil")
	}
	// Pool has no exported subject list; re-parse embedded DER.
	cert, err := x509.ParseCertificate(signalMessengerRootDER)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != "Signal Messenger, LLC" {
		t.Errorf("unexpected subject org: %v", cert.Subject.Organization)
	}
}

func TestHostname(t *testing.T) {
	if got := Hostname("chat.signal.org:443"); got != "chat.signal.org" {
		t.Errorf("Hostname = %q, want chat.signal.org", got)
	}
}
