// Command signal-go is the demo CLI for the signal-go library.
//
// Phase 2 supports:
//
//	signal-go link -store <dir>    Pair as a Signal secondary device and
//	                               persist credentials under <dir>.
//
// Phase 3+ will add `recv`, `send`, `groups`, etc.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

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
	deviceName := fs.String("name", "", "device name shown in the user's linked devices list (TODO: encrypt)")
	_ = fs.Parse(args)

	dir, err := filepath.Abs(*storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve store dir: %v\n", err)
		return 1
	}
	s, err := fsstore.New(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	if existing, err := s.LoadAccount(); err == nil {
		fmt.Fprintf(os.Stderr, "already linked at %s (ACI=%s, deviceID=%d).\n", dir, existing.ACI, existing.DeviceID)
		fmt.Fprintln(os.Stderr, "Delete the store directory if you want to re-link.")
		return 1
	} else if !errors.Is(err, account.ErrNotLinked) {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
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
	fmt.Printf("  ACI:      %s\n", la.ACI)
	fmt.Printf("  PNI:      %s\n", la.PNI)
	fmt.Printf("  number:   %s\n", la.Number)
	fmt.Printf("  deviceID: %d\n", la.DeviceID)
	fmt.Printf("  store:    %s\n", dir)
	fmt.Println()
	fmt.Println("Phase 3 will add real-time receive over the chat websocket.")
	return 0
}
