package fsstore_test

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	"github.com/thehappydinoa/signal-go/internal/store/fsstore"
)

func validAccount(t *testing.T) *account.Account {
	t.Helper()
	mk := func() account.Identity {
		kp, err := libsignal.GenerateIdentityKeyPair()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		pub, _ := kp.Public.Serialize()
		priv, _ := kp.Private.Serialize()
		regID, _ := prekeys.NewRegistrationID()
		spk, _ := prekeys.GenerateSignedPreKey(kp.Private, 1)
		kspk, _ := prekeys.GenerateLastResortKyberPreKey(kp.Private, 1)
		return account.Identity{
			PublicKey: pub, PrivateKey: priv, RegistrationID: regID,
			SignedPreKey: *spk, LastResortKyberPreKey: *kspk,
			NextPreKeyID: 1, NextKyberPreKeyID: 1,
		}
	}
	return &account.Account{
		ACI: "aci-x", PNI: "pni-y", Number: "+15551234567",
		DeviceID: 2, Password: "deadbeef",
		ProfileKey:  make([]byte, 32),
		ACIIdentity: mk(), PNIIdentity: mk(),
	}
}

func TestNewWithKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	s, err := fsstore.NewWithKey(dir, key)
	if err != nil {
		t.Fatalf("NewWithKey: %v", err)
	}
	if !s.IsEncrypted() {
		t.Error("IsEncrypted false on encrypted store")
	}
	want := validAccount(t)
	if err := s.SaveAccount(want); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}

	// On-disk file should be account.enc, NOT account.json.
	if _, err := os.Stat(filepath.Join(dir, "account.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("plaintext account.json should not exist: stat err = %v", err)
	}
	encPath := filepath.Join(dir, "account.enc")
	info, err := os.Stat(encPath)
	if err != nil {
		t.Fatalf("stat account.enc: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("account.enc mode = %v, want 0600", info.Mode().Perm())
	}
	// And the contents must not be the plaintext password.
	enc, _ := os.ReadFile(encPath)
	if containsBytes(enc, []byte("deadbeef")) {
		t.Error("encrypted file leaks the password in plaintext")
	}

	// Re-open with same key, load, compare.
	s2, err := fsstore.NewWithKey(dir, key)
	if err != nil {
		t.Fatalf("NewWithKey (reopen): %v", err)
	}
	got, err := s2.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	if got.ACI != want.ACI || got.Password != want.Password || got.DeviceID != want.DeviceID {
		t.Errorf("round-trip lost data")
	}

	// Wrong key fails with ErrWrongPassphrase.
	var bad [32]byte
	if _, err := rand.Read(bad[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	s3, err := fsstore.NewWithKey(dir, bad)
	if err != nil {
		t.Fatalf("NewWithKey (bad key): %v", err)
	}
	_, err = s3.LoadAccount()
	if !errors.Is(err, fsstore.ErrWrongPassphrase) {
		t.Errorf("LoadAccount with wrong key: err = %v, want ErrWrongPassphrase", err)
	}
}

func TestNewWithPassphraseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := fsstore.NewWithPassphrase(dir, "hunter2")
	if err != nil {
		t.Fatalf("NewWithPassphrase: %v", err)
	}
	want := validAccount(t)
	if err := s.SaveAccount(want); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}

	// kdf.json should exist with mode 0600.
	info, err := os.Stat(filepath.Join(dir, "kdf.json"))
	if err != nil {
		t.Fatalf("stat kdf.json: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("kdf.json mode = %v, want 0600", info.Mode().Perm())
	}

	// Re-open with the same passphrase — should reuse kdf.json.
	s2, err := fsstore.NewWithPassphrase(dir, "hunter2")
	if err != nil {
		t.Fatalf("NewWithPassphrase (reopen): %v", err)
	}
	got, err := s2.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	if got.ACI != want.ACI || got.Password != want.Password {
		t.Errorf("round-trip lost data")
	}

	// Wrong passphrase -> ErrWrongPassphrase.
	s3, err := fsstore.NewWithPassphrase(dir, "wrong-passphrase")
	if err != nil {
		t.Fatalf("NewWithPassphrase (wrong): %v", err)
	}
	_, err = s3.LoadAccount()
	if !errors.Is(err, fsstore.ErrWrongPassphrase) {
		t.Errorf("LoadAccount with wrong passphrase: err = %v", err)
	}
}

func TestNewWithPassphraseRejectsEmpty(t *testing.T) {
	if _, err := fsstore.NewWithPassphrase(t.TempDir(), ""); err == nil {
		t.Error("expected error on empty passphrase")
	}
}

func TestModeMixingErrors(t *testing.T) {
	dir := t.TempDir()
	// Create a plaintext store + account.
	plain, err := fsstore.New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := plain.SaveAccount(validAccount(t)); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	// Trying to open with a key now must fail with ErrDirPlaintext.
	var key [32]byte
	rand.Read(key[:])
	if _, err := fsstore.NewWithKey(dir, key); !errors.Is(err, fsstore.ErrDirPlaintext) {
		t.Errorf("NewWithKey on plaintext dir: err = %v, want ErrDirPlaintext", err)
	}
	if _, err := fsstore.NewWithPassphrase(dir, "x"); !errors.Is(err, fsstore.ErrDirPlaintext) {
		t.Errorf("NewWithPassphrase on plaintext dir: err = %v, want ErrDirPlaintext", err)
	}

	// And the reverse: create an encrypted store in a fresh dir, then
	// try plain New.
	encDir := t.TempDir()
	encS, err := fsstore.NewWithKey(encDir, key)
	if err != nil {
		t.Fatalf("NewWithKey: %v", err)
	}
	if err := encS.SaveAccount(validAccount(t)); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	if _, err := fsstore.New(encDir); !errors.Is(err, fsstore.ErrDirEncrypted) {
		t.Errorf("New on encrypted dir: err = %v, want ErrDirEncrypted", err)
	}
}

func TestNewRequiresDir(t *testing.T) {
	cases := []func() error{
		func() error { _, err := fsstore.New(""); return err },
		func() error { _, err := fsstore.NewWithKey("", [32]byte{}); return err },
		func() error { _, err := fsstore.NewWithPassphrase("", "p"); return err },
	}
	for i, fn := range cases {
		if err := fn(); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

// containsBytes reports whether haystack contains needle. Avoids
// importing bytes in the assertion path.
func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if equalBytes(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
