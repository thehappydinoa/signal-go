// Package memstore is an in-memory [account.Store] used by tests. It
// also provides an in-memory [store.SignalStores] for libsignal callback
// tests (see signal_stores.go in this package).
package memstore

import (
	"sync"

	"github.com/thehappydinoa/signal-go/internal/account"
)

// Store keeps a single account in memory. The zero value is ready to use.
type Store struct {
	mu      sync.Mutex
	account *account.Account
}

// New returns an empty in-memory store.
func New() *Store { return &Store{} }

// LoadAccount implements [account.Store].
func (s *Store) LoadAccount() (*account.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.account == nil {
		return nil, account.ErrNotLinked
	}
	// Defensive copy so callers can mutate without affecting the store.
	cp := *s.account
	return &cp, nil
}

// SaveAccount implements [account.Store].
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
