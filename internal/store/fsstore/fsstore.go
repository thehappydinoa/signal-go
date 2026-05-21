// Package fsstore persists signal-go state as JSON files on disk.
//
// Layout under the supplied directory:
//
//	./account.json     marshalled *account.Account
//
// Future phases will add subdirectories for sessions, prekeys, identities,
// and sender keys.
//
// Concurrency: one process at a time. We do not lock the directory.
package fsstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/thehappydinoa/signal-go/internal/account"
)

const accountFile = "account.json"

// Store is a filesystem-backed [account.Store].
type Store struct {
	dir string
}

// New returns a Store that reads from and writes into dir. The directory
// is created if it does not exist (perm 0700).
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("fsstore: dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore: mkdir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// LoadAccount implements [account.Store].
func (s *Store) LoadAccount() (*account.Account, error) {
	path := filepath.Join(s.dir, accountFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, account.ErrNotLinked
		}
		return nil, fmt.Errorf("fsstore: read account: %w", err)
	}
	var acct account.Account
	if err := json.Unmarshal(data, &acct); err != nil {
		return nil, fmt.Errorf("fsstore: parse account: %w", err)
	}
	return &acct, nil
}

// SaveAccount writes the account atomically (write to tmp, then rename),
// implementing [account.Store]. Returns an error if the account fails
// validation.
func (s *Store) SaveAccount(acct *account.Account) error {
	if err := acct.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		return fmt.Errorf("fsstore: marshal account: %w", err)
	}
	final := filepath.Join(s.dir, accountFile)
	tmp, err := os.CreateTemp(s.dir, accountFile+".tmp-*")
	if err != nil {
		return fmt.Errorf("fsstore: tmp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsstore: write account: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsstore: chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("fsstore: close tmp: %w", err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		cleanup()
		return fmt.Errorf("fsstore: rename: %w", err)
	}
	return nil
}
