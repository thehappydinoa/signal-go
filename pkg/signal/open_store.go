package signal

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/thehappydinoa/signal-go/internal/store/sqlstore"
)

// StoreOpenOptions configures [OpenFromStore].
type StoreOpenOptions struct {
	// Dir is the directory containing signal.db (required).
	Dir string
	// PassphraseFile is the path to a file containing the store encryption
	// passphrase (trailing newline trimmed). If empty, the store must have
	// been created without a passphrase (plaintext mode, test only).
	PassphraseFile string
	// ClientProfile selects a User-Agent preset. Default: SignalGo.
	ClientProfile UserAgentProfile
	// UserAgent overrides the User-Agent / X-Signal-Agent header.
	UserAgent string
	// Logger for structured diagnostics. Default: slog.Default().
	Logger *slog.Logger
	// AutoSyncStorage triggers a background SyncStorage when a linked device
	// requests a storage-manifest fetch-latest sync.
	AutoSyncStorage bool
	// AutoSyncGroupUpdates triggers a background SyncGroup on change notification.
	AutoSyncGroupUpdates bool
}

// OpenFromStore opens the Signal store at opts.Dir, connects to the Signal
// chat websocket, and returns a ready [Client]. The underlying SQLite store
// is closed automatically when [Client.Close] is called.
//
// Returns [ErrNotLinked] if no linked account exists in the store.
func OpenFromStore(ctx context.Context, opts StoreOpenOptions) (*Client, error) {
	db, err := openSQLStore(opts.Dir, opts.PassphraseFile)
	if err != nil {
		return nil, fmt.Errorf("signal.OpenFromStore: %w", err)
	}
	c, err := Open(ctx, OpenOptions{
		AccountStore:           db,
		SignalStores:           db.SignalStores(),
		GroupDistributionStore: db.GroupDistributionStore(),
		GroupEndorsementStore:  db.GroupEndorsementStore(),
		ClientProfile:          opts.ClientProfile,
		UserAgent:              opts.UserAgent,
		Logger:                 opts.Logger,
		AutoSyncStorage:        opts.AutoSyncStorage,
		AutoSyncGroupUpdates:   opts.AutoSyncGroupUpdates,
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	c.storeCloser = db
	return c, nil
}

func openSQLStore(dir, passphraseFile string) (*sqlstore.DB, error) {
	if passphraseFile == "" {
		return sqlstore.Open(dir)
	}
	raw, err := os.ReadFile(passphraseFile)
	if err != nil {
		return nil, fmt.Errorf("read passphrase file: %w", err)
	}
	passphrase := strings.TrimRight(strings.TrimRight(string(raw), "\n"), "\r")
	return sqlstore.OpenWithPassphrase(dir, passphrase)
}
