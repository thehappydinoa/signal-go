package memstore

import (
	"sync"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// GroupEndorsementStore is an in-memory [store.GroupEndorsementStore].
type GroupEndorsementStore struct {
	mu    sync.Mutex
	byHex map[string]*store.GroupEndorsementRecord
}

// NewGroupEndorsementStore returns an empty in-memory endorsement store.
func NewGroupEndorsementStore() *GroupEndorsementStore {
	return &GroupEndorsementStore{byHex: make(map[string]*store.GroupEndorsementRecord)}
}

// LoadGroupEndorsements implements [store.GroupEndorsementStore].
func (s *GroupEndorsementStore) LoadGroupEndorsements(masterKeyHex string) (*store.GroupEndorsementRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byHex[masterKeyHex]
	if !ok || rec == nil {
		return nil, store.ErrGroupEndorsementNotFound
	}
	return cloneGroupEndorsementRecord(rec), nil
}

// StoreGroupEndorsements implements [store.GroupEndorsementStore].
func (s *GroupEndorsementStore) StoreGroupEndorsements(masterKeyHex string, rec *store.GroupEndorsementRecord) error {
	if rec == nil {
		return store.ErrGroupEndorsementNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byHex[masterKeyHex] = cloneGroupEndorsementRecord(rec)
	return nil
}

// DeleteGroupEndorsements implements [store.GroupEndorsementStore].
func (s *GroupEndorsementStore) DeleteGroupEndorsements(masterKeyHex string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byHex, masterKeyHex)
	return nil
}

func cloneGroupEndorsementRecord(rec *store.GroupEndorsementRecord) *store.GroupEndorsementRecord {
	endorsements := make(map[string][]byte, len(rec.Endorsements))
	for k, v := range rec.Endorsements {
		endorsements[k] = append([]byte(nil), v...)
	}
	return &store.GroupEndorsementRecord{
		Expiration:   rec.Expiration,
		Response:     append([]byte(nil), rec.Response...),
		Endorsements: endorsements,
	}
}
