// Command signal-go is the demo CLI for the signal-go library.
//
// Subcommands so far:
//
//	signal-go link -store <dir>    Pair as a Signal secondary device.
//
// Phase 3+ will add `recv`, `send`, `groups`, etc.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/store/fsstore"
	sg "github.com/thehappydinoa/signal-go/pkg/signal"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "link":
		os.Exit(runLink(os.Args[2:]))
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `signal-go — Go client for Signal (pre-alpha)

Usage:
  signal-go link [flags]      Link as a Signal secondary device

Run 'signal-go <subcommand> -h' for subcommand flags.
`)
}

func runLink(args []string) int {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	timeout := fs.Duration("timeout", 5*time.Minute, "how long to wait for the user to scan")
	userAgent := fs.String("user-agent", "signal-go", "value sent in X-Signal-Agent")
	storeDir := fs.String("store", ".signal-data", "directory where account state is persisted")
	deviceName := fs.String("name", "", "device name shown in the user's linked devices list")
	passphraseFile := fs.String("passphrase-file", "", "path to a file containing the passphrase (newline-trimmed); overrides interactive prompt")
	plaintext := fs.Bool("plaintext", false, "EXPERIMENTAL: do NOT encrypt the store on disk. Test-only.")
	_ = fs.Parse(args)

	dir, err := filepath.Abs(*storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store dir: %v\n", err)
		return 1
	}

	s, err := openStore(dir, *passphraseFile, *plaintext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}

	if existing, err := s.LoadAccount(); err == nil {
		fmt.Fprintf(os.Stderr, "already linked at %s (ACI=%s, deviceID=%d).\n", dir, existing.ACI, existing.DeviceID)
		fmt.Fprintln(os.Stderr, "Delete the store directory if you want to re-link.")
		return 1
	} else if !errors.Is(err, account.ErrNotLinked) {
		if errors.Is(err, fsstore.ErrWrongPassphrase) {
			fmt.Fprintln(os.Stderr, "wrong passphrase (or the store is corrupted).")
		} else {
			fmt.Fprintf(os.Stderr, "store: %v\n", err)
		}
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	la, err := sg.Link(ctx, sg.LinkOptions{
		UserAgent:  *userAgent,
		Store:      s,
		DeviceName: *deviceName,
		OnURL: func(linkURL string) error {
			fmt.Println("Open Signal on your phone → Settings → Linked devices → +")
			fmt.Println("Scan the URL below as a QR code, or paste it manually:")
			fmt.Println()
			fmt.Println("  " + linkURL)
			fmt.Println()
			fmt.Println("Waiting for you to approve the link…")
			return nil
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "link failed: %v\n", err)
		return 1
	}
	fmt.Println()
	fmt.Println("Linked!")
	fmt.Printf("  ACI:       %s\n", la.ACI)
	fmt.Printf("  PNI:       %s\n", la.PNI)
	fmt.Printf("  number:    %s\n", la.Number)
	fmt.Printf("  deviceID:  %d\n", la.DeviceID)
	fmt.Printf("  store:     %s\n", dir)
	if s.IsEncrypted() {
		fmt.Println("  encrypted: yes (AES-256-GCM)")
	} else {
		fmt.Println("  encrypted: NO — plaintext mode (test only)")
	}
	fmt.Println()
	fmt.Println("Phase 3 will add real-time receive over the chat websocket.")
	return 0
}

// openStore picks the right fsstore mode based on flags and (for the
// interactive case) prompts for a passphrase.
func openStore(dir, passphraseFile string, plaintext bool) (*fsstore.Store, error) {
	if plaintext {
		fmt.Fprintln(os.Stderr, "WARNING: -plaintext requested; credentials will be stored unencrypted.")
		return fsstore.New(dir)
	}
	passphrase, err := readPassphrase(passphraseFile)
	if err != nil {
		return nil, err
	}
	return fsstore.NewWithPassphrase(dir, passphrase)
}

// readPassphrase reads a passphrase from the supplied file (with trailing
// newline trimmed) or interactively from the TTY. Returns an error if
// neither source is available (typical for piped/non-interactive use
// without -passphrase-file).
func readPassphrase(file string) (string, error) {
	if file != "" {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("read passphrase file: %w", err)
		}
		return strings.TrimRight(strings.TrimRight(string(raw), "\n"), "\r"), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("no -passphrase-file given and stdin is not a terminal; pipe a passphrase via -passphrase-file=<path>")
	}
	fmt.Fprint(os.Stderr, "Store passphrase (used to encrypt credentials at rest): ")
	pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	if len(pw1) == 0 {
		return "", errors.New("empty passphrase")
	}
	// For first-time use we'd ideally ask twice; defer until we can
	// tell whether kdf.json already exists (which would mean the user
	// is opening an existing store, not creating one). For now, accept
	// one read — the wrong-passphrase code path is graceful.
	_ = bufio.NewReader(os.Stdin)
	return string(pw1), nil
}
