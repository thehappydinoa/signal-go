package sqlstore

import (
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// ProfileKeyStore implements [store.ProfileKeyStore].
type ProfileKeyStore struct {
	db *DB
}

// ProfileKeyStore returns a profile key store backed by db.
func (s *DB) ProfileKeyStore() *ProfileKeyStore {
	return &ProfileKeyStore{db: s}
}

func (p *ProfileKeyStore) LoadProfileKeys() (map[string][]byte, error) {
	rows, err := p.db.db.Query(`SELECT aci, profile_key FROM recipient_profile_keys ORDER BY aci`)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: load profile keys: %w", err)
	}
	defer rows.Close()
	out := make(map[string][]byte)
	for rows.Next() {
		var aci string
		var key []byte
		if err := rows.Scan(&aci, &key); err != nil {
			return nil, fmt.Errorf("sqlstore: scan profile key: %w", err)
		}
		if len(key) != libsignal.ProfileKeyLen {
			continue
		}
		out[aci] = append([]byte(nil), key...)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlstore: load profile keys: %w", err)
	}
	return out, nil
}

func (p *ProfileKeyStore) StoreProfileKey(aci string, profileKey []byte) error {
	if aci == "" {
		return errors.New("sqlstore: profile key ACI required")
	}
	if len(profileKey) != libsignal.ProfileKeyLen {
		return fmt.Errorf("sqlstore: profile key length %d, want %d", len(profileKey), libsignal.ProfileKeyLen)
	}
	_, err := p.db.db.Exec(
		`INSERT INTO recipient_profile_keys(aci, profile_key) VALUES(?, ?)
		 ON CONFLICT(aci) DO UPDATE SET profile_key=excluded.profile_key`,
		aci,
		append([]byte(nil), profileKey...),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store profile key: %w", err)
	}
	return nil
}

func (p *ProfileKeyStore) DeleteProfileKey(aci string) error {
	if aci == "" {
		return nil
	}
	_, err := p.db.db.Exec(`DELETE FROM recipient_profile_keys WHERE aci = ?`, aci)
	if err != nil {
		return fmt.Errorf("sqlstore: delete profile key: %w", err)
	}
	return nil
}
