package memstore_test

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/store/memstore"
)

func TestProfileKeyStoreRoundTrip(t *testing.T) {
	s := memstore.NewProfileKeyStore()
	aci := "00000000-0000-4000-8000-000000000002"
	key := make([]byte, 32)
	key[0] = 0xab

	if err := s.StoreProfileKey(aci, key); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadProfileKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded[aci]) != 32 || loaded[aci][0] != 0xab {
		t.Fatalf("loaded = %v", loaded[aci])
	}
	if err := s.DeleteProfileKey(aci); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.LoadProfileKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(loaded))
	}
}
