package attachment

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	for i, plain := range [][]byte{
		[]byte("Peter Parker"),
		nil,
		{},
		bytes.Repeat([]byte{0xab}, 100),
	} {
		name := []string{"text", "nil", "empty", "block"}[i]
		t.Run(name, func(t *testing.T) {
			enc, err := Encrypt(plain, key)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			got, err := Decrypt(enc.Ciphertext, key, enc.Digest, int64(len(plain)))
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, plain) {
				t.Fatalf("got %q, want %q", got, plain)
			}
		})
	}
}

func TestCiphertextLength(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("hello")
	enc, err := Encrypt(plain, key)
	if err != nil {
		t.Fatal(err)
	}
	if got := CiphertextLength(int64(len(plain))); int64(len(enc.Ciphertext)) != got {
		t.Fatalf("CiphertextLength = %d, blob = %d", got, len(enc.Ciphertext))
	}
}

func TestDecryptRejectsBadKey(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt([]byte("Gwen Stacy"), key)
	if err != nil {
		t.Fatal(err)
	}
	badKey := make([]byte, CombinedKeySize)
	_, err = Decrypt(enc.Ciphertext, badKey, enc.Digest, 10)
	if err == nil {
		t.Fatal("expected error for bad key")
	}
}

func TestDecryptRejectsBadDigest(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt([]byte("Mary Jane Watson"), key)
	if err != nil {
		t.Fatal(err)
	}
	badDigest := make([]byte, 32)
	_, err = Decrypt(enc.Ciphertext, key, badDigest, 16)
	if err == nil {
		t.Fatal("expected error for bad digest")
	}
}

func TestDecryptRejectsBadMAC(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt([]byte("Uncle Ben"), key)
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), enc.Ciphertext...)
	tampered[len(tampered)-1] ^= 0xff
	_, err = Decrypt(tampered, key, enc.Digest, 9)
	if err == nil {
		t.Fatal("expected error for bad MAC")
	}
}

func TestStickerRoundTrip(t *testing.T) {
	packKey := make([]byte, 32)
	if _, err := rand.Read(packKey); err != nil {
		t.Fatal(err)
	}
	combined, err := ExpandStickerPackKey(packKey)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("Peter Parker")
	enc, err := Encrypt(plain, combined)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptSticker(enc.Ciphertext, packKey)
	if err != nil {
		t.Fatalf("DecryptSticker: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q, want %q", got, plain)
	}
}

func TestStickerRejectsBadKey(t *testing.T) {
	packKey := make([]byte, 32)
	if _, err := rand.Read(packKey); err != nil {
		t.Fatal(err)
	}
	combined, err := ExpandStickerPackKey(packKey)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := Encrypt([]byte("Gwen Stacy"), combined)
	if err != nil {
		t.Fatal(err)
	}
	badPackKey := make([]byte, 32)
	_, err = DecryptSticker(enc.Ciphertext, badPackKey)
	if err == nil {
		t.Fatal("expected error for bad sticker key")
	}
}

func TestNewKeyLength(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != CombinedKeySize {
		t.Fatalf("key len = %d", len(key))
	}
}
