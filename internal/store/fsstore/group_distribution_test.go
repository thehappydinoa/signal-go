package fsstore

import (
	"errors"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/store"
)

func TestGroupDistributionStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewGroupDistributionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	const hexID = "abc123"
	const dist = "11111111-2222-3333-4444-555555555555"
	if err := s.StoreGroupDistributionID(hexID, dist); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadGroupDistributionID(hexID)
	if err != nil {
		t.Fatal(err)
	}
	if got != dist {
		t.Fatalf("got %q, want %q", got, dist)
	}

	s2, err := NewGroupDistributionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := s2.LoadGroupDistributionID(hexID)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != dist {
		t.Fatalf("reopen got %q", got2)
	}

	if _, err := s2.LoadGroupDistributionID("missing"); !errors.Is(err, store.ErrGroupDistributionNotFound) {
		t.Fatalf("err = %v", err)
	}
}
