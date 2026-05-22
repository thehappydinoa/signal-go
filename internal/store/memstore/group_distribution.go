package memstore

import (
	"sync"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// GroupDistributionStore is an in-memory [store.GroupDistributionStore].
type GroupDistributionStore struct {
	mu    sync.Mutex
	byHex map[string]string
}

// NewGroupDistributionStore returns an empty in-memory distribution store.
func NewGroupDistributionStore() *GroupDistributionStore {
	return &GroupDistributionStore{byHex: make(map[string]string)}
}

// LoadGroupDistributionID implements [store.GroupDistributionStore].
func (s *GroupDistributionStore) LoadGroupDistributionID(masterKeyHex string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byHex[masterKeyHex]
	if !ok || id == "" {
		return "", store.ErrGroupDistributionNotFound
	}
	return id, nil
}

// StoreGroupDistributionID implements [store.GroupDistributionStore].
func (s *GroupDistributionStore) StoreGroupDistributionID(masterKeyHex, distributionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byHex[masterKeyHex] = distributionID
	return nil
}
