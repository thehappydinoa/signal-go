package signal

import (
	"io"
	"log/slog"
	"testing"
)

// TestMaybeWriteStorageOnBareClient covers Clients built as struct literals
// rather than through Open, which is how the dispatch tests construct them.
// Two things used to go wrong on that path:
//
//   - storageWriteInFlight is only allocated in Open, so recording an
//     in-flight key panicked with "assignment to entry in nil map".
//   - maybeWriteStorageGroup had no c.acct guard, so it would spawn
//     writeStorageGroup, which reads c.acct.AccountEntropyPool. A nil
//     dereference there happens on a background goroutine and takes down the
//     process instead of failing a call.
//
// Both helpers must be no-ops (not panics) when the Client cannot possibly
// perform a storage write.
func TestMaybeWriteStorageOnBareClient(t *testing.T) {
	masterKey := make([]byte, 32)

	tests := []struct {
		name string
		call func(c *Client)
	}{
		{"group with nil acct", func(c *Client) { c.maybeWriteStorageGroup(masterKey) }},
		{"group with empty master key", func(c *Client) { c.maybeWriteStorageGroup(nil) }},
		{"contact with nil acct", func(c *Client) { c.maybeWriteStorageContact("bob-aci") }},
		{"contact with empty aci", func(c *Client) { c.maybeWriteStorageContact("") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{
				log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
				events: make(chan Event, 1),
			}

			tt.call(c)

			// No write may have been registered: every case above lacks an
			// account, so nothing is eligible to be written.
			c.storageMu.RLock()
			n := len(c.storageWriteInFlight)
			c.storageMu.RUnlock()
			if n != 0 {
				t.Fatalf("storageWriteInFlight has %d entries, want 0", n)
			}
		})
	}
}
