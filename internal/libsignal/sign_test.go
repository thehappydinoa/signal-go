package libsignal

import (
	"bytes"
	"testing"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	kp, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	msg := []byte("hello signal")
	sig, err := Sign(kp.Private, msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sig) != 64 {
		t.Errorf("signature len = %d, want 64", len(sig))
	}
	ok, err := Verify(kp.Public, msg, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Error("Verify returned false on valid signature")
	}
}

func TestSignVerifyRejectsTamperedMessage(t *testing.T) {
	kp, _ := GenerateIdentityKeyPair()
	msg := []byte("hello signal")
	sig, _ := Sign(kp.Private, msg)
	tampered := append(bytes.Clone(msg), '!')
	ok, err := Verify(kp.Public, tampered, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Error("Verify accepted tampered message")
	}
}

func TestSignVerifyRejectsWrongKey(t *testing.T) {
	kp1, _ := GenerateIdentityKeyPair()
	kp2, _ := GenerateIdentityKeyPair()
	msg := []byte("hello signal")
	sig, _ := Sign(kp1.Private, msg)
	ok, err := Verify(kp2.Public, msg, sig)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Error("Verify accepted signature from wrong key")
	}
}

func TestSignRejectsNilOrEmpty(t *testing.T) {
	if _, err := Sign(nil, []byte("x")); err == nil {
		t.Error("expected error nil priv")
	}
	kp, _ := GenerateIdentityKeyPair()
	if _, err := Sign(kp.Private, nil); err == nil {
		t.Error("expected error empty msg")
	}
}
