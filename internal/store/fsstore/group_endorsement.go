package fsstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/thehappydinoa/signal-go/internal/store"
)

const groupEndorsementFile = "group_endorsements.json"

type groupEndorsementFileRecord struct {
	ExpirationUnix int64             `json:"expirationUnix"`
	Response       []byte            `json:"response"`
	Endorsements   map[string][]byte `json:"endorsements"`
}

// GroupEndorsementStore persists group send endorsement caches as JSON in
// the signal data directory.
type GroupEndorsementStore struct {
	dir string
	mu  sync.Mutex
}

// NewGroupEndorsementStore returns a filesystem-backed endorsement store
// under dir. The file is created on first write with mode 0600.
func NewGroupEndorsementStore(dir string) (*GroupEndorsementStore, error) {
	if dir == "" {
		return nil, errors.New("fsstore.NewGroupEndorsementStore: dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore.NewGroupEndorsementStore: mkdir: %w", err)
	}
	return &GroupEndorsementStore{dir: dir}, nil
}

// LoadGroupEndorsements implements [store.GroupEndorsementStore].
func (s *GroupEndorsementStore) LoadGroupEndorsements(masterKeyHex string) (*store.GroupEndorsementRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return nil, err
	}
	raw, ok := m[masterKeyHex]
	if !ok {
		return nil, store.ErrGroupEndorsementNotFound
	}
	return fileRecordToStore(raw)
}

// StoreGroupEndorsements implements [store.GroupEndorsementStore].
func (s *GroupEndorsementStore) StoreGroupEndorsements(masterKeyHex string, rec *store.GroupEndorsementRecord) error {
	if rec == nil {
		return errors.New("fsstore.StoreGroupEndorsements: nil record")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return err
	}
	m[masterKeyHex] = storeToFileRecord(rec)
	return s.writeLocked(m)
}

// DeleteGroupEndorsements implements [store.GroupEndorsementStore].
func (s *GroupEndorsementStore) DeleteGroupEndorsements(masterKeyHex string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, err := s.readLocked()
	if err != nil {
		return err
	}
	delete(m, masterKeyHex)
	return s.writeLocked(m)
}

func (s *GroupEndorsementStore) path() string {
	return filepath.Join(s.dir, groupEndorsementFile)
}

func (s *GroupEndorsementStore) readLocked() (map[string]groupEndorsementFileRecord, error) {
	raw, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]groupEndorsementFileRecord{}, nil
		}
		return nil, fmt.Errorf("fsstore: read group endorsements: %w", err)
	}
	var m map[string]groupEndorsementFileRecord
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("fsstore: decode group endorsements: %w", err)
	}
	if m == nil {
		m = map[string]groupEndorsementFileRecord{}
	}
	return m, nil
}

func (s *GroupEndorsementStore) writeLocked(m map[string]groupEndorsementFileRecord) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("fsstore: encode group endorsements: %w", err)
	}
	tmp := s.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("fsstore: write group endorsements temp: %w", err)
	}
	if err := os.Rename(tmp, s.path()); err != nil {
		return fmt.Errorf("fsstore: rename group endorsements: %w", err)
	}
	return nil
}

func storeToFileRecord(rec *store.GroupEndorsementRecord) groupEndorsementFileRecord {
	endorsements := make(map[string][]byte, len(rec.Endorsements))
	for k, v := range rec.Endorsements {
		endorsements[k] = append([]byte(nil), v...)
	}
	return groupEndorsementFileRecord{
		ExpirationUnix: rec.Expiration.Unix(),
		Response:       append([]byte(nil), rec.Response...),
		Endorsements:   endorsements,
	}
}

func fileRecordToStore(raw groupEndorsementFileRecord) (*store.GroupEndorsementRecord, error) {
	if len(raw.Response) == 0 || len(raw.Endorsements) == 0 {
		return nil, store.ErrGroupEndorsementNotFound
	}
	endorsements := make(map[string][]byte, len(raw.Endorsements))
	for k, v := range raw.Endorsements {
		endorsements[k] = append([]byte(nil), v...)
	}
	return &store.GroupEndorsementRecord{
		Expiration:   time.Unix(raw.ExpirationUnix, 0),
		Response:     append([]byte(nil), raw.Response...),
		Endorsements: endorsements,
	}, nil
}
