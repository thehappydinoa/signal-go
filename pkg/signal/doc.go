// Package signal is the public API for signal-go.
//
// The package will eventually expose: [Link] (QR-based device linking),
// [Open] (load a previously-linked account), [Client] (send/receive on
// behalf of a linked account), and related types.
//
// Phase 1 exposes only [Link], which performs the QR-handshake step but
// does not yet complete device registration. See ROADMAP.md for the staged
// build-out plan.
package signal
