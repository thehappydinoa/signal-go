// Package sqlstore persists signal-go state in a SQLite database.
//
// It implements [account.Store], [store.SignalStores], and the optional
// group distribution / endorsement stores in one file. Account material
// uses the same AES-256-GCM + Argon2id wire format as [fsstore] (ADR 0012).
//
// Open via [Open], [OpenWithKey], or [OpenWithPassphrase]. The database
// file is created with mode 0600 inside a 0700 directory.
package sqlstore

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // register database driver
)

const (
	schemaVersion = 1
	dbFile        = "signal.db"
)

// DB is the root handle for a SQLite-backed signal-go store.
type DB struct {
	path      string
	db        *sql.DB
	key       [32]byte
	encrypted bool
}

// Open opens or creates a plaintext SQLite store at dir/signal.db.
// Test-only; production callers should use [OpenWithKey] or
// [OpenWithPassphrase].
func Open(dir string) (*DB, error) {
	if dir == "" {
		return nil, errors.New("sqlstore: dir is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("sqlstore: mkdir: %w", err)
	}
	path := filepath.Join(dir, dbFile)
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	s := &DB{path: path, db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func openDB(path string) (*sql.DB, error) {
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("sqlstore: chmod dir: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("sqlstore: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
		_ = db.Close()
		return nil, fmt.Errorf("sqlstore: chmod db: %w", err)
	}
	return db, nil
}

func (s *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS account (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			data BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS local_identity (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			pub BLOB NOT NULL,
			priv BLOB NOT NULL,
			registration_id INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			addr TEXT PRIMARY KEY,
			record BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS identities (
			addr TEXT PRIMARY KEY,
			pub BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS prekeys (
			id INTEGER PRIMARY KEY,
			record BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS signed_prekeys (
			id INTEGER PRIMARY KEY,
			record BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS kyber_prekeys (
			id INTEGER PRIMARY KEY,
			record BLOB NOT NULL,
			used INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS sender_keys (
			sender TEXT NOT NULL,
			dist_id TEXT NOT NULL,
			record BLOB NOT NULL,
			PRIMARY KEY (sender, dist_id)
		)`,
		`CREATE TABLE IF NOT EXISTS group_distributions (
			master_key_hex TEXT PRIMARY KEY,
			distribution_id TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS group_endorsements (
			master_key_hex TEXT PRIMARY KEY,
			data BLOB NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlstore: migrate: %w", err)
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES('schema_version', ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		fmt.Sprintf("%d", schemaVersion),
	)
	if err != nil {
		return fmt.Errorf("sqlstore: set schema version: %w", err)
	}
	return nil
}

// Close closes the underlying database handle.
func (s *DB) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// IsEncrypted reports whether account blobs are sealed with AES-GCM.
func (s *DB) IsEncrypted() bool { return s.encrypted }
