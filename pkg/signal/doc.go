// Package signal is the public API for signal-go.
//
// The package exposes:
//
//   - [Link] — QR-based device linking (pairs this process as a secondary
//     device on an existing Signal account).
//   - [Open] — loads a previously-linked account and connects to Signal's
//     authenticated chat websocket.
//   - [Client] — receives typed events (messages, receipts, typing
//     indicators, sync messages) on a buffered channel.
//
// Typical usage:
//
//	// First run: link as a new device.
//	la, err := signal.Link(ctx, signal.LinkOptions{...})
//
//	// Subsequent runs: open the existing account and receive.
//	client, err := signal.Open(ctx, signal.OpenOptions{...})
//	for ev := range client.Events() {
//	    switch e := ev.(type) {
//	    case *signal.MessageEvent:       ...
//	    case *signal.ReceiptEvent:       ...
//	    case *signal.TypingEvent:        ...
//	    case *signal.SyncMessageEvent:   ...
//	    case *signal.DecryptionErrorEvent: ...
//	    }
//	}
//
// See ROADMAP.md for the staged build-out plan: sending (Phase 4),
// groups (Phase 5), and the bot framework (Phase 6) are forthcoming.
package signal
