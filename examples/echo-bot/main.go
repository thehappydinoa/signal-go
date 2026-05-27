// Command echo-bot is a tiny, end-to-end example using pkg/signal.
//
// It supports linking into a SQLite store (account + libsignal state) and
// then running a receive loop that replies to inbound 1:1 messages.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/thehappydinoa/signal-go/internal/cliargs"
	"github.com/thehappydinoa/signal-go/internal/profile"
	"github.com/thehappydinoa/signal-go/internal/qrterminal"
	sg "github.com/thehappydinoa/signal-go/pkg/signal"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "link":
		os.Exit(runLink(os.Args[2:]))
	case "run":
		os.Exit(runBot(os.Args[2:]))
	case "-h", "--help", "help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `echo-bot — example Signal bot using signal-go

Usage:
  echo-bot link [flags]   Link as a Signal secondary device into a SQLite store
  echo-bot run  [flags]   Run an echo bot (reply to inbound 1:1 messages)

Profiling (run): -memprofile / -cpuprofile write on exit (see README).

Typical flow:
  go run ./examples/echo-bot link -store .signal-bot
  go run ./examples/echo-bot run  -store .signal-bot

Run 'echo-bot <subcommand> -h' for subcommand flags.
`)
}

func runLink(args []string) int {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	timeout, noQR := cliargs.LinkBind(fs)
	deviceName := fs.String("name", "echo-bot", "device name shown in the user's linked devices list")
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, ".signal-bot")
	clientPreset, userAgent := cliargs.ClientBind(fs)
	_ = fs.Parse(args)

	client, err := cliargs.ClientFromFlags(clientPreset, userAgent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	store, err := cliargs.StoreFromFlags(storeDir, passphraseFile, plaintext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	db, err := store.OpenSQLStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	if existing, err := db.LoadAccount(); err == nil {
		fmt.Fprintf(os.Stderr, "already linked at %s (ACI=%s, deviceID=%d).\n", store.Dir, existing.ACI, existing.DeviceID)
		fmt.Fprintln(os.Stderr, "Delete the store directory if you want to re-link.")
		return 1
	} else if !errors.Is(err, sg.ErrNotLinked) {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	linked, err := sg.Link(ctx, sg.LinkOptions{
		ClientProfile: client.Profile,
		UserAgent:     client.UserAgent,
		Store:         db,
		SignalStores:  db.SignalStores(),
		DeviceName:    *deviceName,
		OnURL: func(linkURL string) error {
			fmt.Println("Open Signal on your phone → Settings → Linked devices → +")
			fmt.Println("Scan the QR code below (or use the URL if -no-qr / non-TTY):")
			fmt.Println()
			if err := qrterminal.Write(linkURL, qrterminal.Options{OptOut: *noQR}); err != nil {
				if !errors.Is(err, qrterminal.ErrDisabled) {
					fmt.Fprintf(os.Stderr, "qr render: %v\n", err)
				}
				fmt.Println("  " + linkURL)
			} else {
				fmt.Println()
				fmt.Println("URL (fallback):")
				fmt.Println("  " + linkURL)
			}
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
	fmt.Printf("  ACI:       %s\n", linked.ACI)
	fmt.Printf("  number:    %s\n", linked.Number)
	fmt.Printf("  deviceID:  %d\n", linked.DeviceID)
	fmt.Printf("  store:     %s\n", store.Dir)
	if db.IsEncrypted() {
		fmt.Println("  encrypted: yes (AES-256-GCM)")
	} else {
		fmt.Println("  encrypted: NO — plaintext mode (test only)")
	}
	return 0
}

func setupRunProfiling(memProfile, cpuProfile *string) (func(), error) {
	memPath, cpuPath, err := profile.PathsFromFlags(memProfile, cpuProfile)
	if err != nil {
		return nil, err
	}
	prof, err := profile.Start(memPath, cpuPath)
	if err != nil {
		return nil, err
	}
	return func() {
		if err := prof.Close(); err != nil {
			log.Printf("write profiles: %v", err)
		} else if memPath != "" || cpuPath != "" {
			log.Printf("profiles written (mem=%q cpu=%q)", memPath, cpuPath)
		}
	}, nil
}

func runBot(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, ".signal-bot")
	clientPreset, userAgent := cliargs.ClientBind(fs)
	replyPrefix := fs.String("reply-prefix", "echo: ", "prefix added to replies")
	basicAuth := fs.Bool("basic-auth", false, "send replies with basic auth (not sealed sender); use if the peer never receives echoes")
	memProfile, cpuProfile := profile.Bind(fs)
	_ = fs.Parse(args)

	flushProf, err := setupRunProfiling(memProfile, cpuProfile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	defer flushProf()

	client, err := cliargs.ClientFromFlags(clientPreset, userAgent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}

	store, err := cliargs.StoreFromFlags(storeDir, passphraseFile, plaintext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}

	db, err := store.OpenSQLStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	sgClient, err := sg.Open(ctx, sg.OpenOptions{
		AccountStore:           db,
		SignalStores:           db.SignalStores(),
		GroupDistributionStore: db.GroupDistributionStore(),
		GroupEndorsementStore:  db.GroupEndorsementStore(),
		ClientProfile:          client.Profile,
		UserAgent:              client.UserAgent,
		AutoSyncStorage:        true,
		AutoSyncGroupUpdates:   true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open client: %v\n", err)
		return 1
	}
	defer func() { _ = sgClient.Close() }()

	selfACI := sgClient.Account().ACI
	log.Printf("connected (aci=%s device_id=%d go=%s)", selfACI, sgClient.Account().DeviceID, runtime.Version())
	if *basicAuth {
		log.Printf("basic-auth mode: replies use identifiable send (not sealed sender)")
	}
	log.Printf("listening for inbound messages; Ctrl+C to stop")

	for {
		select {
		case <-ctx.Done():
			log.Printf("shutting down")
			return 0
		case <-sgClient.Done():
			log.Printf("connection closed")
			return 0
		case ev, ok := <-sgClient.Events():
			if !ok {
				log.Printf("event channel closed")
				return 0
			}
			switch e := ev.(type) {
			case *sg.MessageEvent:
				if e.Body == "" {
					continue
				}
				if e.GroupID != "" {
					log.Printf("skip group message (group_id=%s sender=%s)", e.GroupID, e.Sender)
					continue
				}
				if e.Sender == selfACI {
					log.Printf("skip: sender is this linked device’s own ACI (%s); message the bot number from another account", selfACI)
					continue
				}
				reply := *replyPrefix + e.Body
				log.Printf("recv msg sender=%s device=%d body=%q", e.Sender, e.SenderDevice, truncate(e.Body, 160))
				if err := sendReply(ctx, sgClient, e.Sender, reply, *basicAuth); err != nil {
					log.Printf("send failed recipient=%s err=%v", e.Sender, err)
				}
			case *sg.DecryptionErrorEvent:
				log.Printf("decrypt error sender=%s err=%v", e.Sender, e.Err)
			}
		}
	}
}

func sendReply(ctx context.Context, client *sg.Client, recipientACI, text string, basicAuth bool) error {
	if basicAuth {
		client.SetRecipientProfileKey(recipientACI, nil)
	} else if _, err := client.FetchProfile(ctx, recipientACI, nil); err != nil {
		log.Printf("fetch profile for %s: %v (send uses basic auth until profile is known)", recipientACI, err)
	}

	receipt, err := client.Send(ctx, recipientACI, text)
	if err != nil {
		return err
	}
	log.Printf("replied recipient=%s ts=%s body=%q", recipientACI, receipt.Timestamp.Format(time.RFC3339), truncate(text, 160))
	return nil
}

func truncate(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
