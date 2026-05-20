package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/store/fsstore"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
)

// validAccount returns an Account that passes Validate.
func validAccount(t *testing.T) *account.Account {
	t.Helper()
	mkIdentity := func() account.Identity {
		kp, err := libsignal.GenerateIdentityKeyPair()
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		pub, _ := kp.Public.Serialize()
		priv, _ := kp.Private.Serialize()
		regID, _ := prekeys.NewRegistrationID()
		spk, err := prekeys.GenerateSignedPreKey(kp.Private, 1)
		if err != nil {
			t.Fatalf("Signed: %v", err)
		}
		kspk, err := prekeys.GenerateLastResortKyberPreKey(kp.Private, 1)
		if err != nil {
			t.Fatalf("Kyber: %v", err)
		}
		return account.Identity{
			PublicKey:             pub,
			PrivateKey:            priv,
			RegistrationID:        regID,
			SignedPreKey:          *spk,
			LastResortKyberPreKey: *kspk,
			NextPreKeyID:          1,
			NextKyberPreKeyID:     1,
		}
	}
	return &account.Account{
		ACI:         "aci-1234",
		PNI:         "pni-5678",
		Number:      "+15551234567",
		DeviceID:    2,
		Password:    "deadbeefcafe",
		ProfileKey:  make([]byte, 32),
		ACIIdentity: mkIdentity(),
		PNIIdentity: mkIdentity(),
	}
}

// stores is the table of [store.Store] implementations under test.
func stores(t *testing.T) map[string]store.Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "fs")
	fs, err := fsstore.New(dir)
	if err != nil {
		t.Fatalf("fsstore.New: %v", err)
	}
	return map[string]store.Store{
		"memstore": memstore.New(),
		"fsstore":  fs,
	}
}

func TestStoreLoadAccountWhenEmpty(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			_, err := s.LoadAccount()
			if !errors.Is(err, store.ErrNotLinked) {
				t.Errorf("err = %v, want ErrNotLinked", err)
			}
		})
	}
}

func TestStoreSaveLoadRoundTrip(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			acct := validAccount(t)
			if err := s.SaveAccount(acct); err != nil {
				t.Fatalf("SaveAccount: %v", err)
			}
			loaded, err := s.LoadAccount()
			if err != nil {
				t.Fatalf("LoadAccount: %v", err)
			}
			if loaded.ACI != acct.ACI || loaded.DeviceID != acct.DeviceID || loaded.Password != acct.Password {
				t.Errorf("round-trip lost data")
			}
			if string(loaded.ACIIdentity.PublicKey) != string(acct.ACIIdentity.PublicKey) {
				t.Errorf("ACI identity not preserved")
			}
		})
	}
}

func TestStoreRejectsInvalidAccount(t *testing.T) {
	for name, s := range stores(t) {
		t.Run(name, func(t *testing.T) {
			bad := validAccount(t)
			bad.Password = ""
			if err := s.SaveAccount(bad); err == nil {
				t.Error("expected error on invalid account")
			}
		})
	}
}
