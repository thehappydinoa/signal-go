package account

import "errors"

// ErrNotLinked is returned by [Store.LoadAccount] when no account has
// been persisted yet.
var ErrNotLinked = errors.New("account: not linked")

// Store is the durable backing store for the linked-device account
// material. Implementations live under internal/store/{memstore,fsstore}.
type Store interface {
	// LoadAccount returns the previously-persisted account or [ErrNotLinked].
	LoadAccount() (*Account, error)
	// SaveAccount writes acct atomically. Subsequent LoadAccount calls
	// observe the new state.
	SaveAccount(acct *Account) error
}
