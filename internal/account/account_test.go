package account

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func sampleAccount() *Account {
	return &Account{
		ACI:                "00000000-0000-4000-8000-000000000001",
		PNI:                "00000000-0000-4000-8000-000000000002",
		Number:             "+15551234567",
		DeviceID:           2,
		Password:           "super-secret-password",
		ProfileKey:         []byte("0123456789abcdef0123456789abcdef"),
		AccountEntropyPool: "very-secret-aep-string",
		ReadReceipts:       true,
		ACIIdentity: Identity{
			PublicKey:      bytes.Repeat([]byte{0x05}, 33),
			PrivateKey:     bytes.Repeat([]byte{0x06}, 32),
			RegistrationID: 4242,
		},
		PNIIdentity: Identity{
			PublicKey:      bytes.Repeat([]byte{0x07}, 33),
			PrivateKey:     bytes.Repeat([]byte{0x08}, 32),
			RegistrationID: 4343,
		},
	}
}

// TestAccountLogValueScrubsSecrets is the canonical Phase-8 check for
// the slog redaction. The test logs an Account through a TextHandler
// and asserts that no secret field appears in the output.
func TestAccountLogValueScrubsSecrets(t *testing.T) {
	a := sampleAccount()
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	logger.Info("linked", "account", a)
	got := buf.String()

	for _, secret := range []string{
		a.Password,
		a.AccountEntropyPool,
		string(a.ProfileKey),
		string(a.ACIIdentity.PrivateKey),
		string(a.PNIIdentity.PrivateKey),
	} {
		if strings.Contains(got, secret) {
			t.Errorf("log output contains secret %q\n---\n%s", secret, got)
		}
	}
	// Non-secret fields must still be visible.
	for _, want := range []string{a.ACI, a.PNI, "[REDACTED"} {
		if !strings.Contains(got, want) {
			t.Errorf("log output missing %q\n---\n%s", want, got)
		}
	}
	// Phone number country code is preserved.
	if !strings.Contains(got, "+155") {
		t.Errorf("log output missing country code in number: %s", got)
	}
}

func TestRedactBytes(t *testing.T) {
	if got := redactBytes(nil); got != "[empty]" {
		t.Errorf("redactBytes(nil) = %q", got)
	}
	if got := redactBytes([]byte{1, 2, 3}); got != "[REDACTED 3 bytes]" {
		t.Errorf("redactBytes([3]) = %q", got)
	}
}

func TestIdentityLogValueRedactsPrivateKey(t *testing.T) {
	id := &Identity{
		PublicKey:      []byte("public-bytes"),
		PrivateKey:     []byte("private-bytes-secret"),
		RegistrationID: 99,
	}
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("id", "identity", id)
	if strings.Contains(buf.String(), "private-bytes-secret") {
		t.Errorf("private key leaked: %s", buf.String())
	}
}

func TestNilAccountLogValueDoesNotPanic(t *testing.T) {
	var a *Account
	slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)).Info("nil acct", "account", a)
}
