package storage

import (
	"bytes"
	"testing"
)

func TestDeriveStorageServiceKeyDeterministic(t *testing.T) {
	var master [32]byte
	for i := range master {
		master[i] = byte(i)
	}
	got := DeriveStorageServiceKey(master)
	got2 := DeriveStorageServiceKey(master)
	if got != got2 {
		t.Fatal("DeriveStorageServiceKey not deterministic")
	}
	if got == master {
		t.Fatal("storage service key should differ from master")
	}
}

func TestDeriveManifestKeyChangesWithVersion(t *testing.T) {
	var key [32]byte
	k0 := DeriveManifestKey(key, 0)
	k1 := DeriveManifestKey(key, 1)
	if k0 == k1 {
		t.Fatal("manifest keys should differ by version")
	}
}

func TestDeriveItemKeyLegacyAndRotated(t *testing.T) {
	var storageKey [32]byte
	storageKey[0] = 0xab
	storageID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	legacy, err := DeriveItemKey(storageKey, nil, storageID)
	if err != nil {
		t.Fatalf("legacy item key: %v", err)
	}
	legacy2, err := DeriveItemKey(storageKey, nil, storageID)
	if err != nil {
		t.Fatalf("legacy item key 2: %v", err)
	}
	if legacy != legacy2 {
		t.Fatal("legacy item key not deterministic")
	}

	recordIkm := bytes.Repeat([]byte{0xcd}, 32)
	rotated, err := DeriveItemKey(storageKey, recordIkm, storageID)
	if err != nil {
		t.Fatalf("rotated item key: %v", err)
	}
	if rotated == legacy {
		t.Fatal("rotated key should differ from legacy")
	}
}

func TestEncryptDecryptRecordRoundTrip(t *testing.T) {
	var key [32]byte
	key[31] = 0x01
	plain := []byte("storage record plaintext")
	iv := bytes.Repeat([]byte{0x11}, ivLen)
	ct, err := EncryptRecordForTest(key, plain, iv)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := DecryptRecord(key, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round-trip mismatch: %q vs %q", got, plain)
	}
}
