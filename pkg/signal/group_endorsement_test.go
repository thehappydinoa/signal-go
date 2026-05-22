package signal

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
)

func TestGroupSendEndorsementCacheLoadsFromStore(t *testing.T) {
	masterHex := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	master, err := hex.DecodeString(masterHex)
	if err != nil {
		t.Fatal(err)
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}
	endorseStore := memstore.NewGroupEndorsementStore()
	exp := time.Now().Add(time.Hour)
	rec := &store.GroupEndorsementRecord{
		Expiration: exp,
		Response:   []byte{1, 2, 3},
		Endorsements: map[string][]byte{
			"00000000-0000-4000-8000-000000000001": {9, 8, 7},
		},
	}
	if err := endorseStore.StoreGroupEndorsements(masterHex, rec); err != nil {
		t.Fatal(err)
	}

	c := &Client{
		groupEndorseStore: endorseStore,
		groupSecretParams: map[string][libsignal.GroupSecretParamsLen]byte{
			masterHex: secretParams,
		},
	}
	cache, err := c.groupSendEndorsementCache(masterHex)
	if err != nil {
		t.Fatal(err)
	}
	if !cache.expiration.Equal(exp) {
		t.Fatalf("expiration = %v", cache.expiration)
	}
	if string(cache.endorsements["00000000-0000-4000-8000-000000000001"]) != string(rec.Endorsements["00000000-0000-4000-8000-000000000001"]) {
		t.Fatalf("endorsements = %+v", cache.endorsements)
	}
}

func TestDeleteGroupSendEndorsements(t *testing.T) {
	masterHex := "aa"
	endorseStore := memstore.NewGroupEndorsementStore()
	rec := &store.GroupEndorsementRecord{
		Expiration:   time.Now().Add(time.Hour),
		Response:     []byte{1},
		Endorsements: map[string][]byte{"a": {1}},
	}
	if err := endorseStore.StoreGroupEndorsements(masterHex, rec); err != nil {
		t.Fatal(err)
	}
	c := &Client{
		groupEndorseStore: endorseStore,
		groupEndorsements: map[string]*groupSendEndorsementCache{
			masterHex: {response: []byte{1}},
		},
	}
	c.deleteGroupSendEndorsements(masterHex)
	if _, err := endorseStore.LoadGroupEndorsements(masterHex); err == nil {
		t.Fatal("expected not found after delete")
	}
	c.groupEndorseMu.Lock()
	_, ok := c.groupEndorsements[masterHex]
	c.groupEndorseMu.Unlock()
	if ok {
		t.Fatal("memory cache should be cleared")
	}
}
