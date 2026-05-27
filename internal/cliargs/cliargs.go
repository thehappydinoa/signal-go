// Package cliargs registers shared flags for signal-go CLIs and examples
// (store path, passphrase, client preset) and helpers to open sqlstore.
package cliargs

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/thehappydinoa/signal-go/internal/store/seal"
	"github.com/thehappydinoa/signal-go/internal/store/sqlstore"
	"github.com/thehappydinoa/signal-go/internal/web/useragent"
)

// Store holds parsed store-related flag values.
type Store struct {
	Dir            string
	PassphraseFile string
	Plaintext      bool
}

// StoreBind registers -store, -passphrase-file, and -plaintext on fs.
func StoreBind(fs *flag.FlagSet, defaultDir string) (dir, passphraseFile *string, plaintext *bool) {
	dir = fs.String("store", defaultDir, "directory where account state is persisted (SQLite signal.db)")
	passphraseFile = fs.String("passphrase-file", "", "path to a file containing the passphrase (newline-trimmed); overrides interactive prompt")
	plaintext = fs.Bool("plaintext", false, "EXPERIMENTAL: do NOT encrypt the store on disk. Test-only.")
	return dir, passphraseFile, plaintext
}

// StoreFromFlags resolves an absolute store directory from flag pointers.
func StoreFromFlags(dir, passphraseFile *string, plaintext *bool) (Store, error) {
	if dir == nil || passphraseFile == nil || plaintext == nil {
		return Store{}, errors.New("cliargs: nil store flag pointer")
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return Store{}, fmt.Errorf("cliargs: resolve store dir: %w", err)
	}
	return Store{
		Dir:            abs,
		PassphraseFile: *passphraseFile,
		Plaintext:      *plaintext,
	}, nil
}

// OpenSQLStore opens or creates the sqlstore at s.Dir.
func (s Store) OpenSQLStore() (*sqlstore.DB, error) {
	if s.Plaintext {
		fmt.Fprintln(os.Stderr, "WARNING: -plaintext requested; credentials will be stored unencrypted.")
		return sqlstore.Open(s.Dir)
	}
	passphrase, err := ReadPassphrase(s.PassphraseFile)
	if err != nil {
		return nil, err
	}
	return sqlstore.OpenWithPassphrase(s.Dir, passphrase)
}

// IsWrongPassphrase reports whether err is a store decryption failure.
func IsWrongPassphrase(err error) bool {
	return errors.Is(err, seal.ErrWrongPassphrase)
}

// ReadPassphrase reads a passphrase from file (trailing newline trimmed) or the TTY.
func ReadPassphrase(file string) (string, error) {
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("cliargs: read passphrase file: %w", err)
		}
		return strings.TrimRight(strings.TrimRight(string(raw), "\n"), "\r"), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("cliargs: no -passphrase-file given and stdin is not a terminal; use -passphrase-file=<path>")
	}
	fmt.Fprint(os.Stderr, "Store passphrase (used to encrypt credentials at rest): ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("cliargs: read passphrase: %w", err)
	}
	if len(pw) == 0 {
		return "", errors.New("cliargs: empty passphrase")
	}
	_ = bufio.NewReader(os.Stdin)
	return string(pw), nil
}

// Client holds a resolved client User-Agent preset and optional override.
type Client struct {
	Profile   useragent.Profile
	UserAgent string
}

// ClientBind registers -client and -user-agent on fs.
func ClientBind(fs *flag.FlagSet) (clientPreset, userAgent *string) {
	clientPreset = fs.String("client", string(useragent.SignalGo),
		"client User-Agent preset: signal-go, android, ios, desktop-linux, desktop-macos, desktop-windows")
	userAgent = fs.String("user-agent", "", "override User-Agent / X-Signal-Agent (disables -client preset)")
	return clientPreset, userAgent
}

// ClientFromFlags parses client flags after [flag.FlagSet.Parse].
func ClientFromFlags(clientPreset, userAgent *string) (Client, error) {
	if clientPreset == nil || userAgent == nil {
		return Client{}, errors.New("cliargs: nil client flag pointer")
	}
	profile, err := useragent.Parse(*clientPreset)
	if err != nil {
		return Client{}, fmt.Errorf("cliargs: invalid -client: %w", err)
	}
	return Client{Profile: profile, UserAgent: *userAgent}, nil
}

// LinkBind registers flags common to device-link subcommands (-timeout, -no-qr).
// Register -name on fs separately when a non-empty default is needed.
func LinkBind(fs *flag.FlagSet) (timeout *time.Duration, noQR *bool) {
	timeout = fs.Duration("timeout", 5*time.Minute, "how long to wait for the user to scan")
	noQR = fs.Bool("no-qr", false, "do not render a QR code in the terminal (URL is still printed)")
	return timeout, noQR
}

// Subcommand splits argv into the first token (subcommand) and remaining args.
func Subcommand(argv []string) (cmd string, rest []string, ok bool) {
	if len(argv) == 0 {
		return "", nil, false
	}
	if len(argv) == 1 {
		return argv[0], nil, true
	}
	return argv[0], argv[1:], true
}
