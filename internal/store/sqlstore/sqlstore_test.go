package sqlstore_test

import (
	"errors"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/store/sqlstore"
)

func TestAccountStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := sqlstore.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = s.Close() }()

	acct := validAccount(t)
	if err := s.SaveAccount(acct); err != nil {
		t.Fatalf("SaveAccount: %v", err)
	}
	loaded, err := s.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	if loaded.ACI != acct.ACI || loaded.Password != acct.Password {
		t.Fatal("account round-trip mismatch")
	}
}

func TestSignalStoresRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ss := db.SignalStores()

	addr := store.Address{ServiceID: "aci-1", DeviceID: 1}
	if err := ss.SetLocalIdentity([]byte{1, 2, 3}, []byte{4, 5, 6}, 42); err != nil {
		t.Fatal(err)
	}

	if err := ss.StoreSession(addr, []byte("session")); err != nil {
		t.Fatal(err)
	}
	got, err := ss.LoadSession(addr)
	if err != nil || string(got) != "session" {
		t.Fatalf("session = %q, %v", got, err)
	}

	if err := ss.StorePreKey(7, []byte("prekey")); err != nil {
		t.Fatal(err)
	}
	pk, err := ss.LoadPreKey(7)
	if err != nil || string(pk) != "prekey" {
		t.Fatalf("prekey = %q, %v", pk, err)
	}

	ec, kem, err := ss.CountAvailableOneTimePreKeys(999)
	if err != nil || ec != 1 || kem != 0 {
		t.Fatalf("inventory ec=%d kem=%d err=%v", ec, kem, err)
	}
}

func TestGroupStoresRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	dist := db.GroupDistributionStore()
	if err := dist.StoreGroupDistributionID("abc", "dist-uuid"); err != nil {
		t.Fatal(err)
	}
	id, err := dist.LoadGroupDistributionID("abc")
	if err != nil || id != "dist-uuid" {
		t.Fatalf("dist = %q, %v", id, err)
	}

	end := db.GroupEndorsementStore()
	rec := &store.GroupEndorsementRecord{
		Response:     []byte{1},
		Endorsements: map[string][]byte{"member": {2}},
	}
	if err := end.StoreGroupEndorsements("abc", rec); err != nil {
		t.Fatal(err)
	}
	loaded, err := end.LoadGroupEndorsements("abc")
	if err != nil || len(loaded.Endorsements) != 1 {
		t.Fatalf("endorsements = %+v, %v", loaded, err)
	}
}

func validAccount(t *testing.T) *account.Account {
	t.Helper()
	// Minimal valid account for store round-trip tests.
	acct := &account.Account{
		ACI:        "aci-1234",
		PNI:        "pni-5678",
		Number:     "+15551234567",
		DeviceID:   2,
		Password:   "deadbeefcafe",
		ProfileKey: make([]byte, 32),
	}
	fillIdentity := func() account.Identity {
		return account.Identity{
			PublicKey:             make([]byte, 33),
			PrivateKey:            make([]byte, 32),
			RegistrationID:        1234,
			SignedPreKey:          account.Identity{}.SignedPreKey,
			LastResortKyberPreKey: account.Identity{}.LastResortKyberPreKey,
			NextPreKeyID:          1,
			NextKyberPreKeyID:     1,
		}
	}
	id := fillIdentity()
	id.SignedPreKey.PublicKey = make([]byte, 33)
	id.SignedPreKey.Signature = make([]byte, 64)
	id.LastResortKyberPreKey.PublicKey = make([]byte, 1568)
	id.LastResortKyberPreKey.Signature = make([]byte, 64)
	acct.ACIIdentity = id
	acct.PNIIdentity = id
	return acct
}

func TestLoadAccountWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := sqlstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, err = s.LoadAccount()
	if !errors.Is(err, account.ErrNotLinked) {
		t.Fatalf("got %v", err)
	}
}

var _ store.SignalStores = (*sqlstore.SignalStores)(nil)
