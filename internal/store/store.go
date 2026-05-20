// Package store defines the persistence interface and ships a couple of
// reference implementations (in-memory for tests, filesystem for users).
//
// The interface is intentionally narrow at Phase 2: just account state.
// Later phases will extend it with per-recipient session, identity,
// prekey, and sender-key sub-stores backing libsignal's callback structs.
package store

import (
	"errors"

	"github.com/thehappydinoa/signal-go/internal/account"
)

// ErrNotLinked is returned by [Store.LoadAccount] when no account has been
// persisted yet.
var ErrNotLinked = errors.New("store: no linked account")

// Store is the durable backing store for a signal-go client.
type Store interface {
	// LoadAccount returns the previously-persisted account or [ErrNotLinked].
	LoadAccount() (*account.Account, error)
	// SaveAccount writes acct atomically. Subsequent LoadAccount calls
	// observe the new state.
	SaveAccount(acct *account.Account) error
}
