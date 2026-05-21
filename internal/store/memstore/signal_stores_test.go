package memstore_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
)

func addr(uuid string, dev uint32) store.Address {
	return store.Address{ServiceID: uuid, DeviceID: dev}
}

func TestSignalStoresLocalIdentity(t *testing.T) {
	s := memstore.NewSignalStores()
	if _, _, err := s.LocalIdentityKey(); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("err before SetLocalIdentity = %v, want ErrRecordNotFound", err)
	}
	pub := bytes.Repeat([]byte{0x05}, 33)
	priv := bytes.Repeat([]byte{0xAB}, 32)
	s.SetLocalIdentity(pub, priv, 1234)
	gotPub, gotPriv, err := s.LocalIdentityKey()
	if err != nil {
		t.Fatalf("LocalIdentityKey: %v", err)
	}
	if !bytes.Equal(gotPub, pub) || !bytes.Equal(gotPriv, priv) {
		t.Errorf("identity round-trip mismatch")
	}
	rid, _ := s.LocalRegistrationID()
	if rid != 1234 {
		t.Errorf("regID = %d", rid)
	}
}

func TestSignalStoresIdentityLifecycle(t *testing.T) {
	s := memstore.NewSignalStores()
	a := addr("aaaa-1111", 1)
	if _, err := s.LoadIdentity(a); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("initial Load err = %v", err)
	}
	pub1 := bytes.Repeat([]byte{0x05}, 33)
	pub2 := bytes.Repeat([]byte{0x06}, 33)

	// First save -> unchanged (trust-on-first-use).
	res, err := s.SaveIdentity(a, pub1)
	if err != nil || res != store.IdentityUnchanged {
		t.Errorf("first save: res=%v err=%v", res, err)
	}
	// Same key again -> still unchanged.
	res, _ = s.SaveIdentity(a, pub1)
	if res != store.IdentityUnchanged {
		t.Errorf("re-save same: %v", res)
	}
	// Different key -> replaced.
	res, _ = s.SaveIdentity(a, pub2)
	if res != store.IdentityReplaced {
		t.Errorf("replace: %v", res)
	}

	// IsTrustedIdentity: stored key trusted, others not.
	ok, _ := s.IsTrustedIdentity(a, pub2, store.DirectionSending)
	if !ok {
		t.Errorf("stored key should be trusted")
	}
	ok, _ = s.IsTrustedIdentity(a, pub1, store.DirectionSending)
	if ok {
		t.Errorf("replaced key should no longer be trusted")
	}
	// Unknown address -> trust (TOFU).
	ok, _ = s.IsTrustedIdentity(addr("never-seen", 1), pub1, store.DirectionSending)
	if !ok {
		t.Errorf("unseen address should be trusted (TOFU)")
	}
}

func TestSignalStoresSession(t *testing.T) {
	s := memstore.NewSignalStores()
	a := addr("bbbb", 2)
	if _, err := s.LoadSession(a); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("initial Load = %v", err)
	}
	rec := []byte("opaque-session-record")
	if err := s.StoreSession(a, rec); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := s.LoadSession(a)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !bytes.Equal(got, rec) {
		t.Errorf("session round-trip mismatch")
	}
	// Defensive copy: mutating the returned slice must not change the store.
	got[0] ^= 0xFF
	again, _ := s.LoadSession(a)
	if !bytes.Equal(again, rec) {
		t.Errorf("LoadSession returned aliased slice")
	}
}

func TestSignalStoresPreKey(t *testing.T) {
	s := memstore.NewSignalStores()
	if _, err := s.LoadPreKey(1); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("initial Load = %v", err)
	}
	rec := []byte("rec")
	if err := s.StorePreKey(1, rec); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := s.LoadPreKey(1)
	if err != nil || !bytes.Equal(got, rec) {
		t.Errorf("round-trip: got=%v err=%v", got, err)
	}
	if err := s.RemovePreKey(1); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.LoadPreKey(1); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("post-remove Load = %v", err)
	}
	// Remove is idempotent.
	if err := s.RemovePreKey(1); err != nil {
		t.Errorf("repeat Remove: %v", err)
	}
}

func TestSignalStoresSignedPreKey(t *testing.T) {
	s := memstore.NewSignalStores()
	if err := s.StoreSignedPreKey(7, []byte("spk")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, err := s.LoadSignedPreKey(7)
	if err != nil || string(got) != "spk" {
		t.Errorf("round-trip: %q err=%v", got, err)
	}
	if _, err := s.LoadSignedPreKey(99); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("missing id Load = %v", err)
	}
}

func TestSignalStoresCountAvailableOneTimePreKeys(t *testing.T) {
	s := memstore.NewSignalStores()
	const lastResort uint32 = 1
	if err := s.StorePreKey(10, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := s.StorePreKey(11, []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreKyberPreKey(lastResort, []byte("lr")); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreKyberPreKey(20, []byte("k1")); err != nil {
		t.Fatal(err)
	}
	if err := s.StoreKyberPreKey(21, []byte("k2")); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkKyberPreKeyUsed(20); err != nil {
		t.Fatal(err)
	}
	ec, kem, err := s.CountAvailableOneTimePreKeys(lastResort)
	if err != nil {
		t.Fatal(err)
	}
	if ec != 2 || kem != 1 {
		t.Fatalf("ec=%d kem=%d, want 2 and 1", ec, kem)
	}
}

func TestSignalStoresKyberPreKey(t *testing.T) {
	s := memstore.NewSignalStores()
	if err := s.StoreKyberPreKey(3, []byte("kpk")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, _ := s.LoadKyberPreKey(3)
	if string(got) != "kpk" {
		t.Errorf("round-trip: %q", got)
	}
	if s.KyberPreKeyUsed(3) {
		t.Errorf("freshly-stored prekey is marked used")
	}
	if err := s.MarkKyberPreKeyUsed(3); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	if !s.KyberPreKeyUsed(3) {
		t.Errorf("MarkKyberPreKeyUsed did not stick")
	}
	// Record is retained after Mark (last-resort keys still need to be
	// reachable by their id); deletion is a higher-layer concern.
	got2, err := s.LoadKyberPreKey(3)
	if err != nil || string(got2) != "kpk" {
		t.Errorf("Load post-Mark: got=%q err=%v", got2, err)
	}
}

func TestSignalStoresSenderKey(t *testing.T) {
	s := memstore.NewSignalStores()
	sender := addr("cccc", 1)
	dist := "distribution-uuid-1"
	if _, err := s.LoadSenderKey(sender, dist); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("initial Load = %v", err)
	}
	if err := s.StoreSenderKey(sender, dist, []byte("sk")); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, _ := s.LoadSenderKey(sender, dist)
	if string(got) != "sk" {
		t.Errorf("round-trip: %q", got)
	}
	// Different (sender, dist) is independent.
	if _, err := s.LoadSenderKey(sender, "other"); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("other dist Load = %v", err)
	}
	if _, err := s.LoadSenderKey(addr("dddd", 1), dist); !errors.Is(err, store.ErrRecordNotFound) {
		t.Errorf("other sender Load = %v", err)
	}
}

func TestSignalStoresIsSignalStores(t *testing.T) {
	// Compile-time conformance: the in-memory impl satisfies all of the
	// sub-interfaces (and therefore the SignalStores aggregate).
	var _ store.SignalStores = memstore.NewSignalStores()
}
