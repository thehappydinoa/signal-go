package fsstore

import (
	"errors"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/store"
)

func TestGroupEndorsementStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewGroupEndorsementStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	const hexID = "deadbeef"
	rec := &store.GroupEndorsementRecord{
		Expiration: time.Unix(1_800_000_000, 0),
		Response:   []byte{1, 2, 3},
		Endorsements: map[string][]byte{
			"aci-1": {9, 8, 7},
		},
	}
	if err := s.StoreGroupEndorsements(hexID, rec); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadGroupEndorsements(hexID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Expiration != rec.Expiration || string(got.Response) != string(rec.Response) {
		t.Fatalf("got %+v", got)
	}
	if string(got.Endorsements["aci-1"]) != string(rec.Endorsements["aci-1"]) {
		t.Fatalf("endorsements = %+v", got.Endorsements)
	}

	s2, err := NewGroupEndorsementStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := s2.LoadGroupEndorsements(hexID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.Expiration != rec.Expiration {
		t.Fatalf("reopen expiration = %v", got2.Expiration)
	}

	if err := s2.DeleteGroupEndorsements(hexID); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.LoadGroupEndorsements(hexID); !errors.Is(err, store.ErrGroupEndorsementNotFound) {
		t.Fatalf("err = %v", err)
	}
}
