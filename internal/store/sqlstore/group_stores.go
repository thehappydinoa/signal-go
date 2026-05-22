package sqlstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// GroupDistributionStore implements [store.GroupDistributionStore].
type GroupDistributionStore struct {
	db *DB
}

// GroupDistributionStore returns a group distribution store backed by db.
func (s *DB) GroupDistributionStore() *GroupDistributionStore {
	return &GroupDistributionStore{db: s}
}

func (g *GroupDistributionStore) LoadGroupDistributionID(masterKeyHex string) (string, error) {
	var id string
	err := g.db.db.QueryRow(
		`SELECT distribution_id FROM group_distributions WHERE master_key_hex = ?`,
		masterKeyHex,
	).Scan(&id)
	if err != nil {
		if isNoRows(err) {
			return "", store.ErrGroupDistributionNotFound
		}
		return "", fmt.Errorf("sqlstore: load group distribution: %w", err)
	}
	return id, nil
}

func (g *GroupDistributionStore) StoreGroupDistributionID(masterKeyHex, distributionID string) error {
	_, err := g.db.db.Exec(
		`INSERT INTO group_distributions(master_key_hex, distribution_id) VALUES(?, ?)
		 ON CONFLICT(master_key_hex) DO UPDATE SET distribution_id = excluded.distribution_id`,
		masterKeyHex, distributionID,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store group distribution: %w", err)
	}
	return nil
}

// GroupEndorsementStore implements [store.GroupEndorsementStore].
type GroupEndorsementStore struct {
	db *DB
}

// GroupEndorsementStore returns a group endorsement store backed by db.
func (s *DB) GroupEndorsementStore() *GroupEndorsementStore {
	return &GroupEndorsementStore{db: s}
}

type groupEndorsementBlob struct {
	ExpirationUnix int64             `json:"expirationUnix"`
	Response       []byte            `json:"response"`
	Endorsements   map[string][]byte `json:"endorsements"`
}

func (g *GroupEndorsementStore) LoadGroupEndorsements(masterKeyHex string) (*store.GroupEndorsementRecord, error) {
	var raw []byte
	err := g.db.db.QueryRow(
		`SELECT data FROM group_endorsements WHERE master_key_hex = ?`,
		masterKeyHex,
	).Scan(&raw)
	if err != nil {
		if isNoRows(err) {
			return nil, store.ErrGroupEndorsementNotFound
		}
		return nil, fmt.Errorf("sqlstore: load group endorsements: %w", err)
	}
	var blob groupEndorsementBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return nil, fmt.Errorf("sqlstore: decode group endorsements: %w", err)
	}
	if len(blob.Response) == 0 || len(blob.Endorsements) == 0 {
		return nil, store.ErrGroupEndorsementNotFound
	}
	endorsements := make(map[string][]byte, len(blob.Endorsements))
	for k, v := range blob.Endorsements {
		endorsements[k] = append([]byte(nil), v...)
	}
	return &store.GroupEndorsementRecord{
		Expiration:   time.Unix(blob.ExpirationUnix, 0),
		Response:     append([]byte(nil), blob.Response...),
		Endorsements: endorsements,
	}, nil
}

func (g *GroupEndorsementStore) StoreGroupEndorsements(masterKeyHex string, rec *store.GroupEndorsementRecord) error {
	if rec == nil {
		return errors.New("sqlstore: nil group endorsement record")
	}
	endorsements := make(map[string][]byte, len(rec.Endorsements))
	for k, v := range rec.Endorsements {
		endorsements[k] = append([]byte(nil), v...)
	}
	raw, err := json.Marshal(groupEndorsementBlob{
		ExpirationUnix: rec.Expiration.Unix(),
		Response:       append([]byte(nil), rec.Response...),
		Endorsements:   endorsements,
	})
	if err != nil {
		return fmt.Errorf("sqlstore: encode group endorsements: %w", err)
	}
	_, err = g.db.db.Exec(
		`INSERT INTO group_endorsements(master_key_hex, data) VALUES(?, ?)
		 ON CONFLICT(master_key_hex) DO UPDATE SET data = excluded.data`,
		masterKeyHex, raw,
	)
	if err != nil {
		return fmt.Errorf("sqlstore: store group endorsements: %w", err)
	}
	return nil
}

func (g *GroupEndorsementStore) DeleteGroupEndorsements(masterKeyHex string) error {
	_, err := g.db.db.Exec(`DELETE FROM group_endorsements WHERE master_key_hex = ?`, masterKeyHex)
	if err != nil {
		return fmt.Errorf("sqlstore: delete group endorsements: %w", err)
	}
	return nil
}
