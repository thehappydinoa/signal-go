package sqlstore

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store/fsstore"
)

// ErrWrongPassphrase is returned when account decryption fails.
var ErrWrongPassphrase = fsstore.ErrWrongPassphrase

// OpenWithKey opens an encrypted SQLite store using a raw 32-byte key.
func OpenWithKey(dir string, key [fsstore.KeyLen]byte) (*DB, error) {
	s, err := Open(dir)
	if err != nil {
		return nil, err
	}
	s.key = key
	s.encrypted = true
	return s, nil
}

// OpenWithPassphrase opens an encrypted SQLite store whose key is derived
// from passphrase via Argon2id. KDF metadata is stored in dir/kdf.json
// (shared wire format with [fsstore]).
func OpenWithPassphrase(dir, passphrase string) (*DB, error) {
	if passphrase == "" {
		return nil, errors.New("sqlstore: passphrase is required")
	}
	key, err := loadOrCreatePassphraseKey(dir, passphrase)
	if err != nil {
		return nil, err
	}
	return OpenWithKey(dir, key)
}

// LoadAccount implements [account.Store].
func (s *DB) LoadAccount() (*account.Account, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM account WHERE id = 1`).Scan(&data)
	if err != nil {
		if isNoRows(err) {
			return nil, account.ErrNotLinked
		}
		return nil, fmt.Errorf("sqlstore: load account: %w", err)
	}
	if s.encrypted {
		var err error
		data, err = fsstore.Open(s.key, data)
		if err != nil {
			return nil, err
		}
	}
	var acct account.Account
	if err := json.Unmarshal(data, &acct); err != nil {
		return nil, fmt.Errorf("sqlstore: parse account: %w", err)
	}
	return &acct, nil
}

// SaveAccount implements [account.Store].
func (s *DB) SaveAccount(acct *account.Account) error {
	if err := acct.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		return fmt.Errorf("sqlstore: marshal account: %w", err)
	}
	if s.encrypted {
		data, err = fsstore.Seal(s.key, data)
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(
		`INSERT INTO account(id, data) VALUES(1, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		data,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: save account: %w", err)
	}
	return nil
}
