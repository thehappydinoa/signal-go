// Command signal-go is the demo CLI for the signal-go library.
//
// Phase 1 supports one subcommand:
//
//	signal-go link    Print the sgnl://linkdevice URL and wait for the user
//	                  to scan it with their primary device.
//
// As later phases land, this CLI will gain `send`, `recv`, `groups`, etc.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	_ = fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	// Honour Ctrl-C.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	acct, err := sg.Link(ctx, sg.LinkOptions{
		UserAgent: *userAgent,
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
	fmt.Println("Linked! Decoded provisioning material from your primary device:")
	fmt.Printf("  ACI:    %s\n", acct.ACI)
	fmt.Printf("  PNI:    %s\n", acct.PNI)
	fmt.Printf("  number: %s\n", acct.Number)
	fmt.Printf("  code:   %s\n", acct.ProvisioningCode)
	fmt.Printf("  profile key: %d bytes\n", len(acct.ProfileKey))
	fmt.Println()
	fmt.Println("Next: Phase 2c will generate prekeys and register against /v1/devices/link.")
	fmt.Println("See ROADMAP.md.")
	return 0
}
