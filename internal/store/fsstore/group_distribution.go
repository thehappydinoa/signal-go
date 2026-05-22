package fsstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/thehappydinoa/signal-go/internal/store"
)

const groupDistFile = "group_dists.json"

// GroupDistributionStore persists group sender-key distribution UUIDs as
// JSON in the signal data directory.
type GroupDistributionStore struct {
	dir string
	mu  sync.Mutex
}

// NewGroupDistributionStore returns a filesystem-backed distribution store
// under dir. The file is created on first write with mode 0600.
func NewGroupDistributionStore(dir string) (*GroupDistributionStore, error) {
	if dir == "" {
		return nil, errors.New("fsstore.NewGroupDistributionStore: dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore.NewGroupDistributionStore: mkdir: %w", err)
	}
	return &GroupDistributionStore{dir: dir}, nil
}

// LoadGroupDistributionID implements [store.GroupDistributionStore].
func (s *GroupDistributionStore) LoadGroupDistributionID(masterKeyHex string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return "", err
	}
	id, ok := m[masterKeyHex]
	if !ok || id == "" {
		return "", store.ErrGroupDistributionNotFound
	}
	return id, nil
}

// StoreGroupDistributionID implements [store.GroupDistributionStore].
func (s *GroupDistributionStore) StoreGroupDistributionID(masterKeyHex, distributionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return err
	}
	m[masterKeyHex] = distributionID
	return s.writeLocked(m)
}

func (s *GroupDistributionStore) path() string {
	return filepath.Join(s.dir, groupDistFile)
}

func (s *GroupDistributionStore) readLocked() (map[string]string, error) {
	raw, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("fsstore: read group distributions: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("fsstore: decode group distributions: %w", err)
	}
	if m == nil {
		m = map[string]string{}
	}
	return m, nil
}

func (s *GroupDistributionStore) writeLocked(m map[string]string) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("fsstore: encode group distributions: %w", err)
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("fsstore: write group distributions temp: %w", err)
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		return fmt.Errorf("fsstore: rename group distributions: %w", err)
	}
	return nil
}
