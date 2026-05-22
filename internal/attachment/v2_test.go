package attachment

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptV2RoundTrip(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("attachment v2 payload")
	enc, err := EncryptV2(plain, key, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptV2(enc.Ciphertext, key, enc.Digest, enc.IncrementalMAC, enc.ChunkSize, len(plain))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q", got)
	}
}

func TestLogPadSize(t *testing.T) {
	if got := LogPadSize(100); got < 541 {
		t.Fatalf("LogPadSize(100) = %d", got)
	}
}

func TestCiphertextLengthV2Positive(t *testing.T) {
	if CiphertextLengthV2(10) <= 0 {
		t.Fatal("expected positive length")
	}
}
