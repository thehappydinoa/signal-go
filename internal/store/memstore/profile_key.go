package memstore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// ProfileKeyStore is an in-memory [store.ProfileKeyStore].
type ProfileKeyStore struct {
	mu   sync.Mutex
	keys map[string][]byte
}

// NewProfileKeyStore returns an empty in-memory profile key store.
func NewProfileKeyStore() *ProfileKeyStore {
	return &ProfileKeyStore{keys: make(map[string][]byte)}
}

// LoadProfileKeys implements [store.ProfileKeyStore].
func (s *ProfileKeyStore) LoadProfileKeys() (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string][]byte, len(s.keys))
	for aci, key := range s.keys {
		out[aci] = append([]byte(nil), key...)
	}
	return out, nil
}

// StoreProfileKey implements [store.ProfileKeyStore].
func (s *ProfileKeyStore) StoreProfileKey(aci string, profileKey []byte) error {
	if aci == "" {
		return errors.New("memstore: profile key ACI required")
	}
	if len(profileKey) != libsignal.ProfileKeyLen {
		return fmt.Errorf("memstore: profile key length %d, want %d", len(profileKey), libsignal.ProfileKeyLen)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[aci] = append([]byte(nil), profileKey...)
	return nil
}

// DeleteProfileKey implements [store.ProfileKeyStore].
func (s *ProfileKeyStore) DeleteProfileKey(aci string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.keys, aci)
	return nil
}
