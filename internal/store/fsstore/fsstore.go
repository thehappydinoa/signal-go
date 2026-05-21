// Package fsstore persists signal-go state as files on disk.
//
// Two modes:
//
//   - Plaintext via [New]. Single file `account.json` with mode 0600.
//     Test-only. The struct embeds none of the production safety
//     properties and should never be used to hold a real account.
//
//   - Encrypted via [NewWithKey] or [NewWithPassphrase]. File
//     `account.enc` contains the JSON sealed with AES-256-GCM; the
//     32-byte key is supplied at construction or derived from a
//     passphrase via Argon2id (parameters in `kdf.json`). See
//     [docs/adr/0012-encrypted-store.md] for the wire format and
//     threat-model discussion.
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

const (
	accountFile    = "account.json" // plaintext
	accountEncFile = "account.enc"  // encrypted
)

// Store is a filesystem-backed [account.Store].
//
// Construct via [New] (plaintext, tests only), [NewWithKey] (raw 32-byte
// key), or [NewWithPassphrase] (Argon2id-derived key). The mode is fixed
// at construction.
type Store struct {
	dir string
	// key is non-zero in encrypted mode. We carry it in memory for the
	// lifetime of the Store; Go's GC makes deterministic zeroization
	// impossible. See ADR 0012.
	key       [keyLen]byte
	encrypted bool
}

// New returns a *plaintext* Store. Test-only. Production code should use
// [NewWithKey] or [NewWithPassphrase].
//
// Returns [ErrDirEncrypted] if the directory already contains an
// encrypted store.
func New(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("fsstore: dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore: mkdir: %w", err)
	}
	encExists, err := existsFile(filepath.Join(dir, accountEncFile))
	if err != nil {
		return nil, err
	}
	if encExists {
		return nil, ErrDirEncrypted
	}
	return &Store{dir: dir}, nil
}

// NewWithKey returns an encrypted Store using the supplied 32-byte key.
// The key never touches the filesystem; the caller owns it.
//
// Returns [ErrDirPlaintext] if the directory already contains an
// unencrypted store (so a careless mode-switch can't leave the plaintext
// file behind).
func NewWithKey(dir string, key [keyLen]byte) (*Store, error) {
	if dir == "" {
		return nil, errors.New("fsstore: dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore: mkdir: %w", err)
	}
	plainExists, err := existsFile(filepath.Join(dir, accountFile))
	if err != nil {
		return nil, err
	}
	if plainExists {
		return nil, ErrDirPlaintext
	}
	return &Store{dir: dir, key: key, encrypted: true}, nil
}

// NewWithPassphrase returns an encrypted Store whose key is derived from
// passphrase via Argon2id. The Argon2id parameters + a per-store salt
// are persisted in `kdf.json` so subsequent opens with the same
// passphrase produce the same key.
//
// On first call against an empty directory a fresh salt + default
// parameters are written. Subsequent calls reuse the stored parameters.
//
// Returns [ErrDirPlaintext] if the directory already contains an
// unencrypted store.
func NewWithPassphrase(dir, passphrase string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("fsstore: dir is required")
	}
	if passphrase == "" {
		return nil, errors.New("fsstore: passphrase is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore: mkdir: %w", err)
	}
	plainExists, err := existsFile(filepath.Join(dir, accountFile))
	if err != nil {
		return nil, err
	}
	if plainExists {
		return nil, ErrDirPlaintext
	}
	meta, err := readKDFMeta(dir)
	switch {
	case err == nil:
		// reuse the existing salt + params
	case errors.Is(err, fs.ErrNotExist):
		meta, err = newKDFMeta()
		if err != nil {
			return nil, err
		}
		if err := writeKDFMeta(dir, meta); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	key, err := meta.derive(passphrase)
	if err != nil {
		return nil, err
	}
	return &Store{dir: dir, key: key, encrypted: true}, nil
}

// IsEncrypted reports whether this Store reads/writes encrypted blobs.
func (s *Store) IsEncrypted() bool { return s.encrypted }

// LoadAccount implements [account.Store].
func (s *Store) LoadAccount() (*account.Account, error) {
	path := filepath.Join(s.dir, s.accountFilename())
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, account.ErrNotLinked
		}
		return nil, fmt.Errorf("fsstore: read account: %w", err)
	}
	if s.encrypted {
		data, err = open(s.key, data)
		if err != nil {
			return nil, err
		}
	}
	var acct account.Account
	if err := json.Unmarshal(data, &acct); err != nil {
		return nil, fmt.Errorf("fsstore: parse account: %w", err)
	}
	return &acct, nil
}

// SaveAccount writes the account atomically. Returns an error if the
// account fails validation. Implements [account.Store].
func (s *Store) SaveAccount(acct *account.Account) error {
	if err := acct.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		return fmt.Errorf("fsstore: marshal account: %w", err)
	}
	if s.encrypted {
		data, err = seal(s.key, data)
		if err != nil {
			return err
		}
	}
	return atomicWrite(s.dir, s.accountFilename(), data)
}

func (s *Store) accountFilename() string {
	if s.encrypted {
		return accountEncFile
	}
	return accountFile
}
