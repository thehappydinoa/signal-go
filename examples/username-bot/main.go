// Command username-bot is a small, end-to-end example of the
// pkg/signal/username flow and the server-facing operations on
// *signal.Client.
//
// It assumes the store is already linked (use echo-bot's `link`
// subcommand against the same -store directory first), then exercises:
//
//	username-bot reserve -store .signal-bot -nickname alice
//	    pick a candidate the server will hold, confirm it, publish a
//	    link, and print the signal.me share URL.
//
//	username-bot lookup -store .signal-bot -url https://signal.me/#eu/...
//	    parse a share URL, fetch the ciphertext, decrypt, resolve to ACI.
//
//	username-bot delete -store .signal-bot
//	    clear the username and any link.
package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/thehappydinoa/signal-go/internal/cliargs"
	sg "github.com/thehappydinoa/signal-go/pkg/signal"
	"github.com/thehappydinoa/signal-go/pkg/signal/username"
)

var handleRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "reserve":
		os.Exit(runReserve(os.Args[2:]))
	case "lookup":
		os.Exit(runLookup(os.Args[2:]))
	case "delete":
		os.Exit(runDelete(os.Args[2:]))
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `username-bot — username ops on an already-linked signal-go store

Usage:
  username-bot reserve -store <dir> -nickname <name>
  username-bot lookup  -store <dir> -url https://signal.me/#eu/<handle>/<entropy>
  username-bot delete  -store <dir>

Link your store first with another example, e.g.:
  go run ./examples/echo-bot link -store .signal-bot
`)
}

func openClient(ctx context.Context, storeDir, passphraseFile *string, plaintext *bool, clientPreset, userAgent *string) (*sg.Client, func(), error) {
	client, err := cliargs.ClientFromFlags(clientPreset, userAgent)
	if err != nil {
		return nil, nil, err
	}
	store, err := cliargs.StoreFromFlags(storeDir, passphraseFile, plaintext)
	if err != nil {
		return nil, nil, err
	}
	db, err := store.OpenSQLStore()
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	sgClient, err := sg.Open(ctx, sg.OpenOptions{
		AccountStore:  db,
		SignalStores:  db.SignalStores(),
		ClientProfile: client.Profile,
		UserAgent:     client.UserAgent,
	})
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("open client: %w", err)
	}
	cleanup := func() {
		_ = sgClient.Close()
		_ = db.Close()
	}
	return sgClient, cleanup, nil
}

func runReserve(args []string) int {
	fs := flag.NewFlagSet("reserve", flag.ExitOnError)
	nickname := fs.String("nickname", "", "nickname to seed candidate generation (required)")
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, ".signal-bot")
	clientPreset, userAgent := cliargs.ClientBind(fs)
	_ = fs.Parse(args)
	if *nickname == "" {
		fmt.Fprintln(os.Stderr, "-nickname is required")
		return 2
	}

	ctx, cancel := signalCancelCtx()
	defer cancel()

	sgClient, cleanup, err := openClient(ctx, storeDir, passphraseFile, plaintext, clientPreset, userAgent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()

	candidates, err := username.Candidates(*nickname, 0, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "candidates: %v\n", err)
		return 1
	}
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "no candidates generated for nickname")
		return 1
	}
	log.Printf("submitting %d candidates in priority order (first: %q)", len(candidates), candidates[0])

	reserved, err := sgClient.ReserveUsername(ctx, candidates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reserve: %v\n", err)
		return 1
	}
	log.Printf("server reserved: %s", reserved)

	published, err := sgClient.ConfirmUsername(ctx, reserved)
	if err != nil {
		fmt.Fprintf(os.Stderr, "confirm: %v\n", err)
		return 1
	}
	fmt.Println()
	fmt.Printf("Username confirmed: %s\n", reserved)
	fmt.Printf("Share URL:          %s\n", published.ShareURL())
	fmt.Println()
	fmt.Println("Save the share URL — anyone with it can start a conversation with this account")
	fmt.Println("without seeing a phone number. Rotate with 'username-bot rotate', clear with 'delete'.")
	return 0
}

func runLookup(args []string) int {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	shareURL := fs.String("url", "", "signal.me share URL from a previous reserve (required)")
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, ".signal-bot")
	clientPreset, userAgent := cliargs.ClientBind(fs)
	_ = fs.Parse(args)
	if *shareURL == "" {
		fmt.Fprintln(os.Stderr, "-url is required")
		return 2
	}
	handle, entropy, err := parseShareURL(*shareURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse url: %v\n", err)
		return 2
	}

	ctx, cancel := signalCancelCtx()
	defer cancel()

	sgClient, cleanup, err := openClient(ctx, storeDir, passphraseFile, plaintext, clientPreset, userAgent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()

	name, err := sgClient.LookupUsername(ctx, handle, entropy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup username: %v\n", err)
		return 1
	}
	fmt.Printf("Username: %s\n", name)

	hash, err := name.Hash()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash username: %v\n", err)
		return 1
	}
	aci, err := sgClient.LookupUsernameHash(ctx, hash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lookup hash: %v\n", err)
		return 1
	}
	fmt.Printf("ACI:      %s\n", aci)
	return 0
}

func runDelete(args []string) int {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, ".signal-bot")
	clientPreset, userAgent := cliargs.ClientBind(fs)
	_ = fs.Parse(args)

	ctx, cancel := signalCancelCtx()
	defer cancel()

	sgClient, cleanup, err := openClient(ctx, storeDir, passphraseFile, plaintext, clientPreset, userAgent)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer cleanup()

	if err := sgClient.DeleteUsername(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "delete: %v\n", err)
		return 1
	}
	fmt.Println("Username and link cleared.")
	return 0
}

// parseShareURL extracts the handle and entropy from a signal.me URL.
// Format: https://signal.me/#eu/<handle-uuid>/<entropy-url-safe-b64>
func parseShareURL(raw string) (handle string, entropy []byte, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil, err
	}
	if u.Host != "signal.me" {
		return "", nil, errors.New("not a signal.me URL")
	}
	// The payload lives in the fragment. Strip a leading "eu/".
	frag := strings.TrimPrefix(u.Fragment, "eu/")
	parts := strings.SplitN(frag, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", nil, errors.New("fragment must be eu/<handle>/<entropy>")
	}
	if !handleRe.MatchString(parts[0]) {
		return "", nil, fmt.Errorf("handle %q is not a canonical UUID", parts[0])
	}
	entropy, err = base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", nil, fmt.Errorf("decode entropy: %w", err)
	}
	if len(entropy) != 32 {
		return "", nil, fmt.Errorf("entropy must be 32 bytes, got %d", len(entropy))
	}
	return parts[0], entropy, nil
}

func signalCancelCtx() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()
	return ctx, cancel
}
