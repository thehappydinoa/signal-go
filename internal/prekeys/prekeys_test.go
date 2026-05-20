package prekeys

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

func identity(t *testing.T) *libsignal.IdentityKeyPair {
	t.Helper()
	kp, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("GenerateIdentityKeyPair: %v", err)
	}
	return kp
}

func TestGenerateSignedPreKey(t *testing.T) {
	id := identity(t)
	spk, err := GenerateSignedPreKey(id.Private, 42)
	if err != nil {
		t.Fatalf("GenerateSignedPreKey: %v", err)
	}
	if spk.ID != 42 {
		t.Errorf("id = %d", spk.ID)
	}
	if len(spk.PublicKey) != 33 || len(spk.PrivateKey) != 32 || len(spk.Signature) != 64 {
		t.Errorf("lengths: pub=%d priv=%d sig=%d", len(spk.PublicKey), len(spk.PrivateKey), len(spk.Signature))
	}
	// Signature must verify under the identity public key.
	ok, err := libsignal.Verify(id.Public, spk.PublicKey, spk.Signature)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Error("signature did not verify")
	}
}

func TestGenerateLastResortKyberPreKey(t *testing.T) {
	id := identity(t)
	kpk, err := GenerateLastResortKyberPreKey(id.Private, 1)
	if err != nil {
		t.Fatalf("GenerateLastResortKyberPreKey: %v", err)
	}
	if kpk.ID != 1 {
		t.Errorf("id = %d", kpk.ID)
	}
	if len(kpk.PublicKey) < 1000 || len(kpk.SecretKey) < 1000 {
		t.Errorf("kyber key lens look wrong: pub=%d sec=%d", len(kpk.PublicKey), len(kpk.SecretKey))
	}
	if len(kpk.Signature) != 64 {
		t.Errorf("sig len = %d", len(kpk.Signature))
	}
	ok, err := libsignal.Verify(id.Public, kpk.PublicKey, kpk.Signature)
	if err != nil || !ok {
		t.Errorf("verify: ok=%v err=%v", ok, err)
	}
}

func TestGenerateOneTimePreKeys(t *testing.T) {
	keys, err := GenerateOneTimePreKeys(100, 5)
	if err != nil {
		t.Fatalf("GenerateOneTimePreKeys: %v", err)
	}
	if len(keys) != 5 {
		t.Fatalf("got %d keys, want 5", len(keys))
	}
	for i, k := range keys {
		if k.ID != 100+uint32(i) {
			t.Errorf("key %d id = %d, want %d", i, k.ID, 100+i)
		}
		if len(k.PublicKey) != 33 || len(k.PrivateKey) != 32 {
			t.Errorf("key %d lens: pub=%d priv=%d", i, len(k.PublicKey), len(k.PrivateKey))
		}
	}
}

func TestGenerateOneTimeKyberPreKeys(t *testing.T) {
	id := identity(t)
	keys, err := GenerateOneTimeKyberPreKeys(id.Private, 200, 3)
	if err != nil {
		t.Fatalf("GenerateOneTimeKyberPreKeys: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("got %d keys, want 3", len(keys))
	}
	for i, k := range keys {
		if k.ID != 200+uint32(i) {
			t.Errorf("key %d id = %d", i, k.ID)
		}
		ok, err := libsignal.Verify(id.Public, k.PublicKey, k.Signature)
		if err != nil || !ok {
			t.Errorf("key %d verify: ok=%v err=%v", i, ok, err)
		}
	}
}

func TestValidateID(t *testing.T) {
	if err := validateID(0); err == nil {
		t.Error("expected error for id 0")
	}
	if err := validateID(MaxID + 1); err == nil {
		t.Error("expected error for id > MaxID")
	}
	if err := validateID(1); err != nil {
		t.Errorf("unexpected error for id 1: %v", err)
	}
	if err := validateID(MaxID); err != nil {
		t.Errorf("unexpected error for MaxID: %v", err)
	}
}

func TestNewRegistrationID(t *testing.T) {
	seen := make(map[uint32]struct{}, 100)
	for i := 0; i < 100; i++ {
		id, err := NewRegistrationID()
		if err != nil {
			t.Fatalf("NewRegistrationID: %v", err)
		}
		if id == 0 || id > MaxID {
			t.Errorf("id %d out of range", id)
		}
		seen[id] = struct{}{}
	}
	// Birthday bound: 100 14-bit ids should overwhelmingly produce >90 unique.
	if len(seen) < 90 {
		t.Errorf("too many collisions: %d distinct out of 100", len(seen))
	}
}

func TestGenerateOneTimePreKeysRejectsBadCount(t *testing.T) {
	if _, err := GenerateOneTimePreKeys(1, 0); err == nil {
		t.Error("expected error count=0")
	}
	if _, err := GenerateOneTimePreKeys(1, -1); err == nil {
		t.Error("expected error count<0")
	}
}
