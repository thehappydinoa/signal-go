package libsignal

import (
	"bytes"
	"testing"
)

func TestGenerateKyberKeyPair(t *testing.T) {
	kp, err := GenerateKyberKeyPair()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	pub, err := kp.Public()
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	secret, err := kp.Secret()
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	pubBytes, err := pub.Serialize()
	if err != nil {
		t.Fatalf("pub.Serialize: %v", err)
	}
	secretBytes, err := secret.Serialize()
	if err != nil {
		t.Fatalf("secret.Serialize: %v", err)
	}
	// Signal uses ML-KEM 1024: public key 1568 bytes, secret 3168 bytes,
	// per FIPS 203. We assert sane non-zero lengths rather than hard
	// constants so we don't break across libsignal upgrades that change
	// parameters.
	if len(pubBytes) < 1000 {
		t.Errorf("kyber pub key suspiciously short: %d bytes", len(pubBytes))
	}
	if len(secretBytes) < 1000 {
		t.Errorf("kyber secret key suspiciously short: %d bytes", len(secretBytes))
	}
}

func TestKyberPublicKeyRoundTrip(t *testing.T) {
	kp, _ := GenerateKyberKeyPair()
	pub, _ := kp.Public()
	a, _ := pub.Serialize()
	b, err := DeserializeKyberPublicKey(a)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	again, _ := b.Serialize()
	if !bytes.Equal(a, again) {
		t.Errorf("kyber pub round-trip mismatch")
	}
}

func TestKyberUniqueness(t *testing.T) {
	// 10 keys should all be distinct.
	seen := make(map[string]struct{}, 10)
	for i := 0; i < 10; i++ {
		kp, err := GenerateKyberKeyPair()
		if err != nil {
			t.Fatalf("Generate %d: %v", i, err)
		}
		pub, _ := kp.Public()
		s, _ := pub.Serialize()
		if _, dup := seen[string(s)]; dup {
			t.Fatalf("duplicate kyber pub at iteration %d", i)
		}
		seen[string(s)] = struct{}{}
	}
}
