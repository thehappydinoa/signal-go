package fsstore

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// fastKDF returns deliberately-weak Argon2id parameters so tests run in
// milliseconds. Production uses defaultKDFParams.
func fastKDF(t *testing.T) kdfMeta {
	t.Helper()
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return kdfMeta{
		Version: 1,
		Salt:    base64.StdEncoding.EncodeToString(salt),
		Time:    1,
		Memory:  8 * 1024, // 8 MiB
		Threads: 2,
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	var key [keyLen]byte
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
		{"64 KiB", bytes.Repeat([]byte{0xCD}, 64*1024)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blob, err := seal(key, tc.pt)
			if err != nil {
				t.Fatalf("seal: %v", err)
			}
			if blob[0] != formatVersion {
				t.Errorf("version byte = 0x%02x, want 0x%02x", blob[0], formatVersion)
			}
			pt, err := open(key, blob)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if !bytes.Equal(pt, tc.pt) {
				t.Errorf("round-trip mismatch")
			}
		})
	}
}

func TestSealNonceIsRandomised(t *testing.T) {
	var key [keyLen]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	pt := []byte("identical plaintext, twice")
	a, _ := seal(key, pt)
	b, _ := seal(key, pt)
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same plaintext produced identical blobs (nonce reuse)")
	}
	// Nonce occupies bytes [1:13].
	if bytes.Equal(a[1:1+nonceLen], b[1:1+nonceLen]) {
		t.Errorf("nonces are identical")
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	var key1, key2 [keyLen]byte
	if _, err := rand.Read(key1[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := rand.Read(key2[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	blob, err := seal(key1, []byte("secret"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	_, err = open(key2, blob)
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("got %v, want wrapping ErrWrongPassphrase", err)
	}
}

func TestOpenRejectsTamper(t *testing.T) {
	var key [keyLen]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	blob, _ := seal(key, []byte("hello"))

	cases := []struct {
		name string
		mut  func(b []byte) []byte
	}{
		{"flip version", func(b []byte) []byte { c := bytes.Clone(b); c[0] ^= 0xFF; return c }},
		{"truncate", func(b []byte) []byte { return b[:len(b)-1] }},
		{"flip nonce", func(b []byte) []byte { c := bytes.Clone(b); c[5] ^= 0x01; return c }},
		{"flip ciphertext", func(b []byte) []byte { c := bytes.Clone(b); c[len(c)-5] ^= 0x01; return c }},
		{"flip tag", func(b []byte) []byte { c := bytes.Clone(b); c[len(c)-1] ^= 0x01; return c }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tampered := tc.mut(blob)
			if _, err := open(key, tampered); err == nil {
				t.Errorf("expected error on tampered blob")
			}
		})
	}
}

func TestOpenRejectsShortBlob(t *testing.T) {
	var key [keyLen]byte
	for _, n := range []int{0, 1, nonceLen + 1} {
		if _, err := open(key, bytes.Repeat([]byte{0x01}, n)); err == nil {
			t.Errorf("expected error for %d-byte blob", n)
		}
	}
}

func TestKDFMetaValidate(t *testing.T) {
	cases := []struct {
		name  string
		meta  kdfMeta
		wantE string
	}{
		{"good", fastKDF(t), ""},
		{"bad version", kdfMeta{Version: 2, Salt: base64.StdEncoding.EncodeToString(make([]byte, 16)), Time: 1, Memory: 1024, Threads: 1}, "version"},
		{"bad salt b64", kdfMeta{Version: 1, Salt: "!!!", Time: 1, Memory: 1024, Threads: 1}, "base64"},
		{"short salt", kdfMeta{Version: 1, Salt: base64.StdEncoding.EncodeToString(make([]byte, 4)), Time: 1, Memory: 1024, Threads: 1}, "salt too short"},
		{"zero time", kdfMeta{Version: 1, Salt: base64.StdEncoding.EncodeToString(make([]byte, 16)), Time: 0, Memory: 1024, Threads: 1}, "cannot be zero"},
		{"huge memory", kdfMeta{Version: 1, Salt: base64.StdEncoding.EncodeToString(make([]byte, 16)), Time: 1, Memory: 1 << 30, Threads: 1}, "safety caps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.meta.validate()
			if tc.wantE == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantE) {
				t.Errorf("err = %v, want one containing %q", err, tc.wantE)
			}
		})
	}
}

func TestKDFDeriveDeterministic(t *testing.T) {
	meta := fastKDF(t)
	k1, err := meta.derive("passphrase")
	if err != nil {
		t.Fatalf("derive 1: %v", err)
	}
	k2, err := meta.derive("passphrase")
	if err != nil {
		t.Fatalf("derive 2: %v", err)
	}
	if !bytes.Equal(k1[:], k2[:]) {
		t.Error("same passphrase + meta should yield same key")
	}
	k3, err := meta.derive("different")
	if err != nil {
		t.Fatalf("derive 3: %v", err)
	}
	if bytes.Equal(k1[:], k3[:]) {
		t.Error("different passphrase should yield different key")
	}
}

func TestKDFMetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := fastKDF(t)
	if err := writeKDFMeta(dir, want); err != nil {
		t.Fatalf("writeKDFMeta: %v", err)
	}
	AssertFileMode0600(t, filepath.Join(dir, kdfFile))
	got, err := readKDFMeta(dir)
	if err != nil {
		t.Fatalf("readKDFMeta: %v", err)
	}
	if got != want {
		t.Errorf("round-trip:\n got %+v\nwant %+v", got, want)
	}
}
