package libsignal

import (
	"bytes"
	"testing"
)

func TestGeneratePrivateKey(t *testing.T) {
	k, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	if k == nil || k.raw.raw == nil {
		t.Fatal("got nil key/pointer")
	}
}

func TestPrivateKeySerializeRoundTrip(t *testing.T) {
	orig, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	bytesOut, err := orig.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if len(bytesOut) != 32 {
		t.Errorf("private key serialization is %d bytes, want 32", len(bytesOut))
	}
	again, err := DeserializePrivateKey(bytesOut)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	bytesAgain, err := again.Serialize()
	if err != nil {
		t.Fatalf("Serialize 2: %v", err)
	}
	if !bytes.Equal(bytesOut, bytesAgain) {
		t.Errorf("round-trip mismatch:\n got %x\nwant %x", bytesAgain, bytesOut)
	}
}

func TestPublicKeyDerivation(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	pubBytes, err := pub.Serialize()
	if err != nil {
		t.Fatalf("pub.Serialize: %v", err)
	}
	if len(pubBytes) != 33 {
		t.Errorf("public key serialization is %d bytes, want 33 (1 tag + 32 point)", len(pubBytes))
	}
	if pubBytes[0] != 0x05 {
		t.Errorf("first byte should be Curve25519 tag 0x05, got 0x%02x", pubBytes[0])
	}
	// Derivation is deterministic: same priv -> same pub.
	pub2, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey 2: %v", err)
	}
	pubBytes2, err := pub2.Serialize()
	if err != nil {
		t.Fatalf("pub2.Serialize: %v", err)
	}
	if !bytes.Equal(pubBytes, pubBytes2) {
		t.Error("repeated derivation produced different public keys")
	}
}

func TestGenerateIdentityKeyPair(t *testing.T) {
	kp, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("GenerateIdentityKeyPair: %v", err)
	}
	if kp.Private == nil || kp.Public == nil {
		t.Fatal("nil keys in pair")
	}
	priv, err := kp.Private.Serialize()
	if err != nil {
		t.Fatalf("priv.Serialize: %v", err)
	}
	if len(priv) != 32 {
		t.Errorf("priv len %d, want 32", len(priv))
	}
	pub, err := kp.Public.Serialize()
	if err != nil {
		t.Fatalf("pub.Serialize: %v", err)
	}
	if len(pub) != 33 {
		t.Errorf("pub len %d, want 33", len(pub))
	}
	derivedPub, err := kp.Private.PublicKey()
	if err != nil {
		t.Fatalf("derived: %v", err)
	}
	derivedBytes, err := derivedPub.Serialize()
	if err != nil {
		t.Fatalf("derivedPub.Serialize: %v", err)
	}
	if !bytes.Equal(pub, derivedBytes) {
		t.Error("Pair.Public does not match Pair.Private.PublicKey()")
	}
}

func TestDeserializePrivateKeyRejectsEmpty(t *testing.T) {
	if _, err := DeserializePrivateKey(nil); err == nil {
		t.Error("expected error on empty input")
	}
}

func TestDeserializePrivateKeyRejectsGarbage(t *testing.T) {
	// Wrong length triggers a libsignal-side validation error.
	if _, err := DeserializePrivateKey([]byte{0x01, 0x02}); err == nil {
		t.Error("expected error deserializing too-short key")
	}
}

func TestUniquenessAcrossManyKeys(t *testing.T) {
	// Smoke test for the RNG: 100 keys should all be distinct.
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		k, err := GeneratePrivateKey()
		if err != nil {
			t.Fatalf("Generate %d: %v", i, err)
		}
		s, err := k.Serialize()
		if err != nil {
			t.Fatalf("Serialize %d: %v", i, err)
		}
		if _, dup := seen[string(s)]; dup {
			t.Fatalf("duplicate key at iteration %d", i)
		}
		seen[string(s)] = struct{}{}
	}
}
