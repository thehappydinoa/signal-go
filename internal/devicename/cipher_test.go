package devicename

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	id, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	const want = "xXxCoolDeviceNamexXx"
	enc, err := Encrypt(want, id.Public)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := Decrypt(enc, id)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != want {
		t.Fatalf("plaintext = %q, want %q", got, want)
	}
}

func TestEncryptEmptyPlaintextRejected(t *testing.T) {
	id, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Encrypt("", id.Public); err == nil {
		t.Fatal("expected error for empty plaintext")
	}
}

func TestDecryptRejectsWrongIdentity(t *testing.T) {
	alice, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	bob, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt("secret label", alice.Public)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(enc, bob); err == nil {
		t.Fatal("expected error when decrypting with wrong identity")
	}
}
