package sqlstore

import (
	"bytes"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// SaveImportedContact implements [store.BackupImportStore].
func (s *DB) SaveImportedContact(c store.ImportedContact) error {
	if c.ACI == "" {
		return errors.New("sqlstore: imported contact ACI required")
	}
	_, err := s.db.Exec(
		`INSERT INTO backup_contacts(aci, pni, e164, profile_key, given_name, family_name, blocked)
		 VALUES(?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(aci) DO UPDATE SET
		   pni=excluded.pni, e164=excluded.e164, profile_key=excluded.profile_key,
		   given_name=excluded.given_name, family_name=excluded.family_name, blocked=excluded.blocked`,
		c.ACI,
		nullIfEmpty(c.PNI),
		nullIfEmpty(c.E164),
		bytesOrNil(c.ProfileKey),
		nullIfEmpty(c.GivenName),
		nullIfEmpty(c.FamilyName),
		boolToInt(c.Blocked),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: save imported contact: %w", err)
	}
	return nil
}

// SaveImportedGroup implements [store.BackupImportStore].
func (s *DB) SaveImportedGroup(g store.ImportedGroup) error {
	if len(g.MasterKey) == 0 {
		return errors.New("sqlstore: imported group master key required")
	}
	id := hex.EncodeToString(g.MasterKey)
	_, err := s.db.Exec(
		`INSERT INTO backup_groups(master_key_hex, master_key, title, blocked)
		 VALUES(?, ?, ?, ?)
		 ON CONFLICT(master_key_hex) DO UPDATE SET
		   master_key=excluded.master_key, title=excluded.title, blocked=excluded.blocked`,
		id,
		bytes.Clone(g.MasterKey),
		nullIfEmpty(g.Title),
		boolToInt(g.Blocked),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: save imported group: %w", err)
	}
	return nil
}

// LoadImportedContacts implements [store.BackupImportStore].
func (s *DB) LoadImportedContacts() ([]store.ImportedContact, error) {
	rows, err := s.db.Query(`SELECT aci, pni, e164, profile_key, given_name, family_name, blocked FROM backup_contacts ORDER BY aci`)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: load imported contacts: %w", err)
	}
	defer rows.Close()
	var out []store.ImportedContact
	for rows.Next() {
		var c store.ImportedContact
		var pni, e164, given, family sql.NullString
		var profileKey []byte
		var blocked int
		if err := rows.Scan(&c.ACI, &pni, &e164, &profileKey, &given, &family, &blocked); err != nil {
			return nil, fmt.Errorf("sqlstore: scan imported contact: %w", err)
		}
		c.PNI = pni.String
		c.E164 = e164.String
		c.ProfileKey = bytes.Clone(profileKey)
		c.GivenName = given.String
		c.FamilyName = family.String
		c.Blocked = blocked != 0
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlstore: load imported contacts: %w", err)
	}
	return out, nil
}

// LoadImportedGroups implements [store.BackupImportStore].
func (s *DB) LoadImportedGroups() ([]store.ImportedGroup, error) {
	rows, err := s.db.Query(`SELECT master_key, title, blocked FROM backup_groups ORDER BY master_key_hex`)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: load imported groups: %w", err)
	}
	defer rows.Close()
	var out []store.ImportedGroup
	for rows.Next() {
		var g store.ImportedGroup
		var title sql.NullString
		var blocked int
		if err := rows.Scan(&g.MasterKey, &title, &blocked); err != nil {
			return nil, fmt.Errorf("sqlstore: scan imported group: %w", err)
		}
		g.Title = title.String
		g.Blocked = blocked != 0
		g.MasterKey = bytes.Clone(g.MasterKey)
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlstore: load imported groups: %w", err)
	}
	return out, nil
}

func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func bytesOrNil(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return bytes.Clone(b)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
