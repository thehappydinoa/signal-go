package sqlstore

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// SignalStores implements [store.SignalStores] backed by SQLite tables.
type SignalStores struct {
	db *sql.DB
}

// SignalStores returns the [store.SignalStores] view of db.
func (s *DB) SignalStores() *SignalStores {
	return &SignalStores{db: s.db}
}

// SetLocalIdentity initialises the local identity for libsignal bootstrap.
func (s *SignalStores) SetLocalIdentity(pub, priv []byte, regID uint32) error {
	_, err := s.db.Exec(
		`INSERT INTO local_identity(id, pub, priv, registration_id) VALUES(1, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET pub=excluded.pub, priv=excluded.priv, registration_id=excluded.registration_id`,
		append([]byte(nil), pub...),
		append([]byte(nil), priv...),
		regID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: set local identity: %w", err)
	}
	return nil
}

func (s *SignalStores) LocalIdentityKey() ([]byte, []byte, error) {
	var pub, priv []byte
	err := s.db.QueryRow(`SELECT pub, priv FROM local_identity WHERE id = 1`).Scan(&pub, &priv)
	if err != nil {
		if isNoRows(err) {
			return nil, nil, store.ErrRecordNotFound
		}
		return nil, nil, fmt.Errorf("sqlstore: load local identity: %w", err)
	}
	return bytes.Clone(pub), bytes.Clone(priv), nil
}

func (s *SignalStores) LocalRegistrationID() (uint32, error) {
	var regID uint32
	err := s.db.QueryRow(`SELECT registration_id FROM local_identity WHERE id = 1`).Scan(&regID)
	if err != nil {
		if isNoRows(err) {
			return 0, store.ErrRecordNotFound
		}
		return 0, fmt.Errorf("sqlstore: load registration id: %w", err)
	}
	return regID, nil
}

func (s *SignalStores) LoadIdentity(addr store.Address) ([]byte, error) {
	var pub []byte
	err := s.db.QueryRow(`SELECT pub FROM identities WHERE addr = ?`, addr.String()).Scan(&pub)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrRecordNotFound
		}
		return nil, fmt.Errorf("sqlstore: load identity: %w", err)
	}
	return bytes.Clone(pub), nil
}

func (s *SignalStores) SaveIdentity(addr store.Address, pub []byte) (store.SaveIdentityResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return store.IdentityUnchanged, fmt.Errorf("sqlstore: save identity: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var prev []byte
	selectErr := tx.QueryRow(`SELECT pub FROM identities WHERE addr = ?`, addr.String()).Scan(&prev)
	if selectErr != nil && !isNoRows(selectErr) {
		return store.IdentityUnchanged, fmt.Errorf("sqlstore: save identity: %w", selectErr)
	}

	_, err = tx.Exec(
		`INSERT INTO identities(addr, pub) VALUES(?, ?)
		 ON CONFLICT(addr) DO UPDATE SET pub = excluded.pub`,
		addr.String(), bytes.Clone(pub),
	)
	if err != nil {
		return store.IdentityUnchanged, fmt.Errorf("sqlstore: save identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return store.IdentityUnchanged, fmt.Errorf("sqlstore: save identity: %w", err)
	}

	if isNoRows(selectErr) || bytes.Equal(prev, pub) {
		return store.IdentityUnchanged, nil
	}
	return store.IdentityReplaced, nil
}

func (s *SignalStores) IsTrustedIdentity(addr store.Address, pub []byte, _ store.Direction) (bool, error) {
	stored, err := s.LoadIdentity(addr)
	if errors.Is(err, store.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return bytes.Equal(stored, pub), nil
}

func (s *SignalStores) LoadSession(addr store.Address) ([]byte, error) {
	var record []byte
	err := s.db.QueryRow(`SELECT record FROM sessions WHERE addr = ?`, addr.String()).Scan(&record)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrRecordNotFound
		}
		return nil, fmt.Errorf("sqlstore: load session: %w", err)
	}
	return bytes.Clone(record), nil
}

func (s *SignalStores) StoreSession(addr store.Address, record []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions(addr, record) VALUES(?, ?)
		 ON CONFLICT(addr) DO UPDATE SET record = excluded.record`,
		addr.String(), bytes.Clone(record),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store session: %w", err)
	}
	return nil
}

func (s *SignalStores) DeleteSession(addr store.Address) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE addr = ?`, addr.String())
	if err != nil {
		return fmt.Errorf("sqlstore: delete session: %w", err)
	}
	return nil
}

func (s *SignalStores) LoadPreKey(id uint32) ([]byte, error) {
	var record []byte
	err := s.db.QueryRow(`SELECT record FROM prekeys WHERE id = ?`, id).Scan(&record)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrRecordNotFound
		}
		return nil, fmt.Errorf("sqlstore: load prekey: %w", err)
	}
	return bytes.Clone(record), nil
}

func (s *SignalStores) StorePreKey(id uint32, record []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO prekeys(id, record) VALUES(?, ?)
		 ON CONFLICT(id) DO UPDATE SET record = excluded.record`,
		id, bytes.Clone(record),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store prekey: %w", err)
	}
	return nil
}

func (s *SignalStores) RemovePreKey(id uint32) error {
	_, err := s.db.Exec(`DELETE FROM prekeys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: remove prekey: %w", err)
	}
	return nil
}

func (s *SignalStores) LoadSignedPreKey(id uint32) ([]byte, error) {
	var record []byte
	err := s.db.QueryRow(`SELECT record FROM signed_prekeys WHERE id = ?`, id).Scan(&record)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrRecordNotFound
		}
		return nil, fmt.Errorf("sqlstore: load signed prekey: %w", err)
	}
	return bytes.Clone(record), nil
}

func (s *SignalStores) StoreSignedPreKey(id uint32, record []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO signed_prekeys(id, record) VALUES(?, ?)
		 ON CONFLICT(id) DO UPDATE SET record = excluded.record`,
		id, bytes.Clone(record),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store signed prekey: %w", err)
	}
	return nil
}

func (s *SignalStores) LoadKyberPreKey(id uint32) ([]byte, error) {
	var record []byte
	err := s.db.QueryRow(`SELECT record FROM kyber_prekeys WHERE id = ?`, id).Scan(&record)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrRecordNotFound
		}
		return nil, fmt.Errorf("sqlstore: load kyber prekey: %w", err)
	}
	return bytes.Clone(record), nil
}

func (s *SignalStores) StoreKyberPreKey(id uint32, record []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO kyber_prekeys(id, record, used) VALUES(?, ?, 0)
		 ON CONFLICT(id) DO UPDATE SET record = excluded.record, used = 0`,
		id, bytes.Clone(record),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store kyber prekey: %w", err)
	}
	return nil
}

func (s *SignalStores) MarkKyberPreKeyUsed(id uint32) error {
	_, err := s.db.Exec(`UPDATE kyber_prekeys SET used = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlstore: mark kyber prekey used: %w", err)
	}
	return nil
}

func (s *SignalStores) LoadSenderKey(sender store.Address, distID string) ([]byte, error) {
	var record []byte
	err := s.db.QueryRow(
		`SELECT record FROM sender_keys WHERE sender = ? AND dist_id = ?`,
		sender.String(), distID,
	).Scan(&record)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrRecordNotFound
		}
		return nil, fmt.Errorf("sqlstore: load sender key: %w", err)
	}
	return bytes.Clone(record), nil
}

func (s *SignalStores) StoreSenderKey(sender store.Address, distID string, record []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO sender_keys(sender, dist_id, record) VALUES(?, ?, ?)
		 ON CONFLICT(sender, dist_id) DO UPDATE SET record = excluded.record`,
		sender.String(), distID, bytes.Clone(record),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store sender key: %w", err)
	}
	return nil
}

// CountAvailableOneTimePreKeys implements [store.PreKeyInventory].
func (s *SignalStores) CountAvailableOneTimePreKeys(lastResortKyberID uint32) (int, int, error) {
	var ec int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM prekeys`).Scan(&ec); err != nil {
		return 0, 0, err
	}
	var kem int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM kyber_prekeys WHERE id != ? AND used = 0`,
		lastResortKyberID,
	).Scan(&kem); err != nil {
		return 0, 0, err
	}
	return ec, kem, nil
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
