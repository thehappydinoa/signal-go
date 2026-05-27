package seal

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	var key [KeyLen]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	cases := []struct {
		name string
		pt   []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("hi")},
		{"json-like", []byte(`{"aci":"abc","password":"s3cret"}`)},
		{"1 KiB", bytes.Repeat([]byte{0xAB}, 1024)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := Seal(key, tc.pt)
			if err != nil {
				t.Fatalf("Seal: %v", err)
			}
			if blob[0] != formatVersion {
				t.Errorf("version byte = 0x%02x, want 0x%02x", blob[0], formatVersion)
			}
			pt, err := Open(key, blob)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			if !bytes.Equal(pt, tc.pt) {
				t.Errorf("round-trip mismatch")
			}
		})
	}
}

func TestSealNonceIsRandomised(t *testing.T) {
	var key [KeyLen]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	pt := []byte("identical plaintext, twice")
	a, _ := Seal(key, pt)
	b, _ := Seal(key, pt)
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same plaintext produced identical blobs")
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	var key1, key2 [KeyLen]byte
	_, _ = rand.Read(key1[:])
	_, _ = rand.Read(key2[:])
	blob, err := Seal(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	_, err = Open(key2, blob)
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("got %v, want ErrWrongPassphrase", err)
	}
}
