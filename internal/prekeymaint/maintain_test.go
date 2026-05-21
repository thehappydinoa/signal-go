package prekeymaint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
	"github.com/thehappydinoa/signal-go/internal/web"
)

type memAccountStore struct {
	mu   sync.Mutex
	acct *account.Account
}

func (s *memAccountStore) LoadAccount() (*account.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.acct == nil {
		return nil, account.ErrNotLinked
	}
	return s.acct, nil
}

func (s *memAccountStore) SaveAccount(acct *account.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acct = acct
	return nil
}

func TestMaintainerMaybeTopUpUploadsWhenLow(t *testing.T) {
	var uploads int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/v2/keys" {
			uploads++
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	acct, stores := testMaintainFixtures(t)
	if err := stores.StorePreKey(99, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	acctStore := &memAccountStore{acct: acct}
	m := NewMaintainer(acctStore, stores, web.New(srv.URL, "test"))
	m.MinAvailable = 10
	m.TopUpBatch = 3

	if err := m.MaybeTopUp(context.Background(), acct); err != nil {
		t.Fatalf("MaybeTopUp: %v", err)
	}
	if uploads != 1 {
		t.Fatalf("uploads = %d, want 1", uploads)
	}
	if acct.ACIIdentity.NextPreKeyID != 2+3 {
		t.Fatalf("NextPreKeyID = %d, want %d", acct.ACIIdentity.NextPreKeyID, 5)
	}
	ec, kem, err := stores.CountAvailableOneTimePreKeys(acct.ACIIdentity.LastResortKyberPreKey.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ec != 4 || kem != 3 {
		t.Fatalf("ec=%d kem=%d, want 4 and 3", ec, kem)
	}
}

func TestMaintainerSkipsWhenSufficient(t *testing.T) {
	uploads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploads++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	acct, stores := testMaintainFixtures(t)
	for i := uint32(0); i < 15; i++ {
		id := 100 + i
		if err := stores.StorePreKey(id, []byte{byte(id)}); err != nil {
			t.Fatal(err)
		}
		if err := stores.StoreKyberPreKey(id, []byte{byte(id), 1}); err != nil {
			t.Fatal(err)
		}
	}

	m := NewMaintainer(&memAccountStore{acct: acct}, stores, web.New(srv.URL, "test"))
	if err := m.MaybeTopUp(context.Background(), acct); err != nil {
		t.Fatal(err)
	}
	if uploads != 0 {
		t.Fatalf("uploads = %d, want 0", uploads)
	}
}

func testMaintainFixtures(t *testing.T) (*account.Account, *memstore.SignalStores) {
	t.Helper()
	kp, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := kp.Public.Serialize()
	priv, _ := kp.Private.Serialize()
	regID, err := prekeys.NewRegistrationID()
	if err != nil {
		t.Fatal(err)
	}
	spk, err := prekeys.GenerateSignedPreKey(kp.Private, 1)
	if err != nil {
		t.Fatal(err)
	}
	kspk, err := prekeys.GenerateLastResortKyberPreKey(kp.Private, 1)
	if err != nil {
		t.Fatal(err)
	}
	ident := account.Identity{
		PublicKey:             pub,
		PrivateKey:            priv,
		RegistrationID:        regID,
		SignedPreKey:          *spk,
		LastResortKyberPreKey: *kspk,
		NextPreKeyID:          2,
		NextKyberPreKeyID:     2,
	}
	acct := &account.Account{
		ACI:         "00000000-0000-4000-8000-000000000001",
		DeviceID:    1,
		Password:    "pw",
		Number:      "+15551234567",
		ACIIdentity: ident,
		PNIIdentity: ident,
	}
	stores := memstore.NewSignalStores()
	stores.SetLocalIdentity(pub, priv, regID)
	return acct, stores
}
