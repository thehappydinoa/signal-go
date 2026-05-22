package memstore

import (
	"sync"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// BackupImportStore is an in-memory [store.BackupImportStore] for tests.
type BackupImportStore struct {
	mu       sync.Mutex
	contacts map[string]store.ImportedContact
	groups   map[string]store.ImportedGroup
}

// NewBackupImportStore returns an empty backup import store.
func NewBackupImportStore() *BackupImportStore {
	return &BackupImportStore{
		contacts: make(map[string]store.ImportedContact),
		groups:   make(map[string]store.ImportedGroup),
	}
}

func (s *BackupImportStore) SaveImportedContact(c store.ImportedContact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := c
	if cp.ProfileKey != nil {
		cp.ProfileKey = append([]byte(nil), cp.ProfileKey...)
	}
	s.contacts[c.ACI] = cp
	return nil
}

func (s *BackupImportStore) SaveImportedGroup(g store.ImportedGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := g
	if cp.MasterKey != nil {
		cp.MasterKey = append([]byte(nil), cp.MasterKey...)
	}
	key := string(g.MasterKey)
	s.groups[key] = cp
	return nil
}

func (s *BackupImportStore) LoadImportedContacts() ([]store.ImportedContact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.ImportedContact, 0, len(s.contacts))
	for _, c := range s.contacts {
		out = append(out, c)
	}
	return out, nil
}

func (s *BackupImportStore) LoadImportedGroups() ([]store.ImportedGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]store.ImportedGroup, 0, len(s.groups))
	for _, g := range s.groups {
		out = append(out, g)
	}
	return out, nil
}
