package libsignal

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestAgreeSymmetric(t *testing.T) {
	// X25519 ECDH is symmetric: agree(a.priv, b.pub) == agree(b.priv, a.pub).
	a, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate a: %v", err)
	}
	b, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate b: %v", err)
	}
	s1, err := Agree(a.Private, b.Public)
	if err != nil {
		t.Fatalf("Agree a→b: %v", err)
	}
	s2, err := Agree(b.Private, a.Public)
	if err != nil {
		t.Fatalf("Agree b→a: %v", err)
	}
	if !bytes.Equal(s1, s2) {
		t.Errorf("shared secrets differ:\n  s1=%x\n  s2=%x", s1, s2)
	}
	if len(s1) != 32 {
		t.Errorf("shared secret len = %d, want 32", len(s1))
	}
}

func TestAgreeDifferentPeers(t *testing.T) {
	a, _ := GenerateIdentityKeyPair()
	b, _ := GenerateIdentityKeyPair()
	c, _ := GenerateIdentityKeyPair()
	s1, _ := Agree(a.Private, b.Public)
	s2, _ := Agree(a.Private, c.Public)
	if bytes.Equal(s1, s2) {
		t.Error("agreement with different peers produced identical secrets")
	}
}

func TestHKDFRFC5869VectorA1(t *testing.T) {
	// RFC 5869 Appendix A.1 (basic test case for SHA-256).
	ikm, _ := hex.DecodeString("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantHex := "3cb25f25faacd57a90434f64d0362f2a" +
		"2d2d0a90cf1a5a4c5db02d56ecc4c5bf" +
		"34007208d5b887185865"
	want, _ := hex.DecodeString(wantHex)
	got, err := HKDFSHA256(len(want), ikm, info, salt)
	if err != nil {
		t.Fatalf("HKDFSHA256: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HKDF mismatch:\n  got=%x\n want=%x", got, want)
	}
}

func TestHKDFRFC5869VectorA3(t *testing.T) {
	// RFC 5869 Appendix A.3 (zero salt, zero info).
	ikm, _ := hex.DecodeString("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	wantHex := "8da4e775a563c18f715f802a063c5a31" +
		"b8a11f5c5ee1879ec3454e5f3c738d2d" +
		"9d201395faa4b61a96c8"
	want, _ := hex.DecodeString(wantHex)
	got, err := HKDFSHA256(len(want), ikm, nil, nil)
	if err != nil {
		t.Fatalf("HKDFSHA256: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HKDF mismatch:\n  got=%x\n want=%x", got, want)
	}
}

func TestHKDFRejectsZeroLen(t *testing.T) {
	if _, err := HKDFSHA256(0, []byte("ikm"), nil, nil); err == nil {
		t.Error("expected error for outLen=0")
	}
}
