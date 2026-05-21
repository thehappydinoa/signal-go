// Package store defines the persistence interfaces signal-go uses for
// per-recipient session/identity/prekey/sender-key state backing
// libsignal's callback structs.
//
// The account-level store (linked-device credentials, identity keys,
// initial prekeys) lives in [internal/account] to avoid an import cycle
// with libsignal.
package store
