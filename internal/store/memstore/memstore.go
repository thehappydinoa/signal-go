// Package memstore is an in-memory [store.Store] used by tests.
package memstore

import (
	"sync"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store"
)

// Store keeps a single account in memory. The zero value is ready to use.
type Store struct {
	mu      sync.Mutex
	account *account.Account
}

// New returns an empty in-memory store.
func New() *Store { return &Store{} }

// LoadAccount implements [store.Store].
func (s *Store) LoadAccount() (*account.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == nil {
		return nil, store.ErrNotLinked
	}
	// Defensive copy so callers can mutate without affecting the store.
	cp := *s.account
	return &cp, nil
}

// SaveAccount implements [store.Store].
func (s *Store) SaveAccount(acct *account.Account) error {
	if err := acct.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *acct
	s.account = &cp
	return nil
}
