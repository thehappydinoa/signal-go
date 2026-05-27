//go:build e2e

package signal_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/qrterminal"
	"github.com/thehappydinoa/signal-go/internal/store/sqlstore"
	"github.com/thehappydinoa/signal-go/pkg/signal"
)

func TestE2E_Open(t *testing.T) {
	e2eEnabled(t)
	db := openE2EDB(t)
	t.Cleanup(func() { _ = db.Close() })

	acct, err := db.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	if acct.ACI == "" || acct.DeviceID == 0 {
		t.Fatalf("linked account looks incomplete: ACI=%q deviceID=%d", acct.ACI, acct.DeviceID)
	}

	ctx, cancel := e2eContext(t)
	defer cancel()

	client, err := signal.Open(ctx, e2eOpenOptions(db))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()
}

func TestE2E_Send(t *testing.T) {
	e2eEnabled(t)
	peer := requireEnv(t, "SIGNAL_E2E_PEER_ACI")
	db := openE2EDB(t)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := e2eContext(t)
	defer cancel()

	client, err := signal.Open(ctx, e2eOpenOptions(db))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	token := fmt.Sprintf("signal-go e2e send %d", time.Now().UnixNano())
	body := token
	if extra := os.Getenv("SIGNAL_E2E_SEND_PREFIX"); extra != "" {
		body = extra + " " + token
	}

	receipt, err := client.Send(ctx, peer, body)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receipt.Timestamp.IsZero() {
		t.Fatal("Send: zero receipt timestamp")
	}
	t.Logf("sent to %s at %s (body contains %q)", peer, receipt.Timestamp, token)
}

func TestE2E_Recv(t *testing.T) {
	e2eEnabled(t)
	db := openE2EDB(t)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := e2eRecvContext(t)
	defer cancel()

	client, err := signal.Open(ctx, e2eOpenOptions(db))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	peerFilter := strings.TrimSpace(os.Getenv("SIGNAL_E2E_PEER_ACI"))
	expectSubstr := strings.TrimSpace(os.Getenv("SIGNAL_E2E_RECV_CONTAINS"))
	if expectSubstr == "" {
		expectSubstr = strings.TrimSpace(os.Getenv("SIGNAL_E2E_EXPECT_BODY"))
	}
	if peerFilter == "" && expectSubstr == "" {
		t.Skip("set SIGNAL_E2E_PEER_ACI and/or SIGNAL_E2E_RECV_CONTAINS (or SIGNAL_E2E_EXPECT_BODY) so Recv knows what to wait for")
	}

	t.Log("waiting for inbound MessageEvent on the linked device websocket")
	if peerFilter != "" {
		t.Logf("  sender filter: %s", peerFilter)
	}
	if expectSubstr != "" {
		t.Logf("  body must contain: %q", expectSubstr)
	} else {
		t.Log("  any non-empty body from the filtered sender (or any sender if no filter)")
	}

	for {
		select {
		case ev, ok := <-client.Events():
			if !ok {
				t.Fatal("Events channel closed before a matching message arrived")
			}
			msg, ok := ev.(*signal.MessageEvent)
			if !ok {
				t.Logf("non-message event: %T %+v", ev, ev)
				continue
			}
			if msg.Body == "" {
				continue
			}
			if peerFilter != "" && msg.Sender != peerFilter {
				continue
			}
			if expectSubstr != "" && !strings.Contains(msg.Body, expectSubstr) {
				t.Logf("ignoring message from %s (body %q)", msg.Sender, truncateRunes(msg.Body, 80))
				continue
			}
			t.Logf("received from %s device %d at %s: %q", msg.Sender, msg.SenderDevice, msg.Timestamp, truncateRunes(msg.Body, 200))
			return
		case <-ctx.Done():
			t.Fatalf("timeout waiting for recv: %v", ctx.Err())
		}
	}
}

func TestE2E_GroupManagement(t *testing.T) {
	e2eEnabled(t)
	masterKey := decodeMasterKey(t, requireEnv(t, "SIGNAL_E2E_GROUP_MASTER_KEY"))
	db := openE2EDB(t)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := e2eContext(t)
	defer cancel()

	client, err := signal.Open(ctx, e2eOpenOptions(db))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer client.Close()

	grp, err := client.FetchGroup(ctx, masterKey)
	if err != nil {
		t.Fatalf("FetchGroup: %v", err)
	}
	if grp.ID == "" {
		t.Fatal("FetchGroup: empty group ID")
	}
	if len(grp.Members) == 0 {
		t.Fatal("FetchGroup: no members")
	}
	t.Logf("group %q title=%q revision=%d members=%d", grp.ID, grp.Title, grp.Revision, len(grp.Members))

	synced, err := client.SyncGroup(ctx, masterKey, grp.Revision)
	if err != nil {
		t.Fatalf("SyncGroup: %v", err)
	}
	if synced == nil {
		t.Fatal("SyncGroup: nil group")
	}
	if synced.Revision < grp.Revision {
		t.Logf("SyncGroup: revision still %d (fetched %d); group may be unchanged", synced.Revision, grp.Revision)
	} else {
		t.Logf("SyncGroup: revision %d title=%q members=%d", synced.Revision, synced.Title, len(synced.Members))
	}

	if os.Getenv("SIGNAL_E2E_GROUP_SEND") != "1" {
		t.Log("skipping SendGroup (set SIGNAL_E2E_GROUP_SEND=1 to deliver a test message to the group)")
		return
	}

	token := fmt.Sprintf("signal-go e2e group %d", time.Now().UnixNano())
	receipt, err := client.SendGroup(ctx, masterKey, token)
	if err != nil {
		t.Fatalf("SendGroup: %v", err)
	}
	if receipt.Timestamp.IsZero() {
		t.Fatal("SendGroup: zero receipt timestamp")
	}
	t.Logf("SendGroup ok at %s", receipt.Timestamp)
}

func TestE2E_Link(t *testing.T) {
	e2eEnabled(t)
	if os.Getenv("SIGNAL_E2E_LINK") != "1" {
		t.Skip("set SIGNAL_E2E_LINK=1 to run an interactive link against chat.signal.org (destructive if store already linked)")
	}
	dir := requireEnv(t, "SIGNAL_E2E_STORE_DIR")
	if _, err := os.Stat(filepath.Join(dir, "signal.db")); err == nil {
		t.Fatalf("store %s already has signal.db; use a fresh directory or link via signal-go link", dir)
	}

	db := openE2EStore(t)
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	linked, err := signal.Link(ctx, signal.LinkOptions{
		Store:             db,
		SignalStores:      db.SignalStores(),
		BackupImportStore: db,
		OnURL: func(linkURL string) error {
			fmt.Fprintln(os.Stderr, "Signal → Settings → Linked devices → + (scan QR or use URL below)")
			if err := qrterminal.Write(linkURL, qrterminal.Options{Writer: os.Stderr}); err != nil {
				fmt.Fprintln(os.Stderr, linkURL)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	t.Logf("linked ACI=%s deviceID=%d number=%s", linked.ACI, linked.DeviceID, linked.Number)
}

func e2eEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("SIGNAL_GO_E2E") != "1" {
		t.Skip("set SIGNAL_GO_E2E=1 (or run task test:e2e)")
	}
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		t.Skipf("set %s to run this test", key)
	}
	return v
}

// openE2EStore opens the e2e sqlstore directory without requiring a linked account.
func openE2EStore(t *testing.T) *sqlstore.DB {
	t.Helper()
	dir := requireEnv(t, "SIGNAL_E2E_STORE_DIR")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("store dir: %v", err)
	}

	var db *sqlstore.DB
	if os.Getenv("SIGNAL_E2E_PLAINTEXT") == "1" {
		db, err = sqlstore.Open(abs)
	} else {
		passphrase, passErr := e2ePassphrase()
		if passErr != nil {
			t.Fatalf("passphrase: %v", passErr)
		}
		db, err = sqlstore.OpenWithPassphrase(abs, passphrase)
	}
	if err != nil {
		t.Fatalf("open store %s: %v", abs, err)
	}
	return db
}

func openE2EDB(t *testing.T) *sqlstore.DB {
	t.Helper()
	db := openE2EStore(t)
	if _, err := db.LoadAccount(); errors.Is(err, account.ErrNotLinked) {
		t.Fatalf("store is not linked; run TestE2E_Link with SIGNAL_E2E_LINK=1 or link into %s first", os.Getenv("SIGNAL_E2E_STORE_DIR"))
	} else if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	return db
}

func e2ePassphrase() (string, error) {
	if p := strings.TrimSpace(os.Getenv("SIGNAL_E2E_PASSPHRASE")); p != "" {
		return p, nil
	}
	path := strings.TrimSpace(os.Getenv("SIGNAL_E2E_PASSPHRASE_FILE"))
	if path == "" {
		return "", errors.New("set SIGNAL_E2E_PASSPHRASE or SIGNAL_E2E_PASSPHRASE_FILE (or SIGNAL_E2E_PLAINTEXT=1 for test-only plaintext stores)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read passphrase file: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

func e2eOpenOptions(db *sqlstore.DB) signal.OpenOptions {
	return signal.OpenOptions{
		AccountStore:           db,
		SignalStores:           db.SignalStores(),
		GroupDistributionStore: db.GroupDistributionStore(),
		GroupEndorsementStore:  db.GroupEndorsementStore(),
		BackupImportStore:      db,
	}
}

func e2eContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	d := 3 * time.Minute
	if s := strings.TrimSpace(os.Getenv("SIGNAL_E2E_TIMEOUT")); s != "" {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			t.Fatalf("SIGNAL_E2E_TIMEOUT: %v", err)
		}
		d = parsed
	}
	return context.WithTimeout(context.Background(), d)
}

func e2eRecvContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	d := 5 * time.Minute
	if s := strings.TrimSpace(os.Getenv("SIGNAL_E2E_RECV_TIMEOUT")); s != "" {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			t.Fatalf("SIGNAL_E2E_RECV_TIMEOUT: %v", err)
		}
		d = parsed
	}
	return context.WithTimeout(context.Background(), d)
}

func decodeMasterKey(t *testing.T, hexKey string) []byte {
	t.Helper()
	hexKey = strings.TrimSpace(hexKey)
	hexKey = strings.TrimPrefix(hexKey, "0x")
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		t.Fatalf("SIGNAL_E2E_GROUP_MASTER_KEY: decode hex: %v", err)
	}
	if len(raw) != libsignal.GroupMasterKeyLen {
		t.Fatalf("SIGNAL_E2E_GROUP_MASTER_KEY: length %d, want %d", len(raw), libsignal.GroupMasterKeyLen)
	}
	return raw
}

func truncateRunes(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
