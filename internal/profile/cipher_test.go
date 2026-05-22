package profile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"testing"
)

func TestCipherStringRoundTrip(t *testing.T) {
	profileKey := bytes.Repeat([]byte{0x42}, 32)
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	plain := "Alice\x00Smith"
	ct, err := EncryptStringForTest(profileKey, plain, nonce)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	c, err := NewCipher(profileKey)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	got, err := c.DecryptString(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Errorf("got %q, want %q", got, plain)
	}
	given, family, err := DecryptName(c, ct)
	if err != nil {
		t.Fatalf("DecryptName: %v", err)
	}
	if given != "Alice" || family != "Smith" {
		t.Errorf("name = %q / %q", given, family)
	}
}

func TestCipherRejectsBadVersion(t *testing.T) {
	c, err := NewCipher(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.DecryptString([]byte{99, 0, 0, 0})
	if err == nil {
		t.Error("expected error for unknown version")
	}
}

func TestVerifyUnidentifiedAccess(t *testing.T) {
	profileKey := []byte{
		0xb9, 0x50, 0x42, 0xa2, 0xc2, 0xd9, 0xe5, 0xb3, 0xbb, 0x09, 0x30, 0x0e, 0xe4,
		0x08, 0xa1, 0x72, 0xfa, 0xcd, 0x96, 0xe9, 0x1b, 0x50, 0x4e, 0x04, 0x3a, 0x5a,
		0x02, 0x3d, 0xc4, 0xcf, 0xf3, 0x59,
	}
	c, err := NewCipher(profileKey)
	if err != nil {
		t.Fatal(err)
	}
	uak := []byte{0x24, 0xfb, 0x96, 0xd4, 0xa5, 0xe3, 0x33, 0xe9, 0xd4, 0x45, 0x12, 0x05, 0xb9, 0xe2, 0xfa, 0xed}
	sum := sha256.Sum256(uak)
	ok, err := c.VerifyUnidentifiedAccess(profileKey, sum[:16])
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected verifier to match")
	}
}
