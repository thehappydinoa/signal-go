package store

import (
	"errors"
	"fmt"
)

// ErrRecordNotFound is returned by Load methods when no record exists
// for the supplied key. libsignal callback bridges translate this to the
// FFI "not found" return value (1) rather than a hard error (-1).
var ErrRecordNotFound = errors.New("store: record not found")

// Address is a ProtocolAddress, the libsignal-side identifier of one
// (ACI/PNI, deviceID) endpoint within an account.
type Address struct {
	// ServiceID is the ACI or PNI as a canonical UUID string. Signal
	// distinguishes them via a prefix in some contexts ("ACI:<uuid>",
	// "PNI:<uuid>"); we keep the prefix when present.
	ServiceID string
	// DeviceID is the per-account device number (1 = primary).
	DeviceID uint32
}

// String returns a canonical "<serviceID>.<deviceID>" representation
// used as a map key in in-memory stores and a filename component on disk.
func (a Address) String() string {
	return fmt.Sprintf("%s.%d", a.ServiceID, a.DeviceID)
}

// Direction is the trust-check direction passed to
// [IdentityStore.IsTrustedIdentity].
type Direction uint32

const (
	// DirectionSending means we are about to encrypt outbound to address.
	DirectionSending Direction = 0
	// DirectionReceiving means we are decrypting inbound from address.
	DirectionReceiving Direction = 1
)

// SaveIdentityResult is the return value of [IdentityStore.SaveIdentity].
type SaveIdentityResult uint8

const (
	// IdentityUnchanged means no record existed (or the existing record
	// already matched). No prior trust was overturned.
	IdentityUnchanged SaveIdentityResult = 0
	// IdentityReplaced means we replaced a previously-trusted identity
	// key with a different one. Callers typically surface a "safety
	// number changed" event to the user.
	IdentityReplaced SaveIdentityResult = 1
)

// SessionStore persists Double-Ratchet session state per peer.
//
// Records are opaque serialized blobs from libsignal — produced by
// signal_session_record_serialize and consumed by
// signal_session_record_deserialize. The store is not expected to
// interpret them.
type SessionStore interface {
	LoadSession(addr Address) (record []byte, err error)
	StoreSession(addr Address, record []byte) error
}

// IdentityStore persists the local identity key pair, the local
// registration ID, and the trusted-identity table.
type IdentityStore interface {
	// LocalIdentityKey returns the long-term identity keypair for this
	// namespace (ACI or PNI). publicKey is 33 bytes (tagged); privateKey
	// is 32 bytes.
	LocalIdentityKey() (publicKey, privateKey []byte, err error)
	// LocalRegistrationID returns the 14-bit registration ID for this
	// namespace.
	LocalRegistrationID() (uint32, error)
	// LoadIdentity returns the 33-byte tagged public identity key
	// previously stored for addr, or [ErrRecordNotFound].
	LoadIdentity(addr Address) (publicKey []byte, err error)
	// SaveIdentity records publicKey as the trusted identity for addr.
	// Returns whether an existing key was replaced.
	SaveIdentity(addr Address, publicKey []byte) (SaveIdentityResult, error)
	// IsTrustedIdentity reports whether publicKey is acceptable for use
	// with addr in the given direction. Trust-on-first-use is permitted.
	IsTrustedIdentity(addr Address, publicKey []byte, dir Direction) (bool, error)
}

// PreKeyStore manages one-time Curve25519 prekeys.
//
// Records are PreKeyRecord blobs from libsignal.
type PreKeyStore interface {
	LoadPreKey(id uint32) (record []byte, err error)
	StorePreKey(id uint32, record []byte) error
	RemovePreKey(id uint32) error
}

// SignedPreKeyStore manages the rotating signed prekey.
//
// Records are SignedPreKeyRecord blobs from libsignal.
type SignedPreKeyStore interface {
	LoadSignedPreKey(id uint32) (record []byte, err error)
	StoreSignedPreKey(id uint32, record []byte) error
}

// KyberPreKeyStore manages Kyber/ML-KEM prekeys (both rotating
// last-resort and one-time).
//
// Records are KyberPreKeyRecord blobs from libsignal.
type KyberPreKeyStore interface {
	LoadKyberPreKey(id uint32) (record []byte, err error)
	StoreKyberPreKey(id uint32, record []byte) error
	// MarkKyberPreKeyUsed is invoked after a successful PQXDH agreement
	// consumed the prekey identified by id. One-time prekeys may then be
	// deleted; last-resort prekeys are retained.
	MarkKyberPreKeyUsed(id uint32) error
}

// SenderKeyStore manages group v2 sender-key state, keyed by
// (sender address, group distribution UUID).
type SenderKeyStore interface {
	LoadSenderKey(sender Address, distributionID string) (record []byte, err error)
	StoreSenderKey(sender Address, distributionID string, record []byte) error
}

// SignalStores groups every libsignal-facing sub-store. A single Store
// implementation typically satisfies all of these via its sub-fields.
type SignalStores interface {
	SessionStore
	IdentityStore
	PreKeyStore
	SignedPreKeyStore
	KyberPreKeyStore
	SenderKeyStore
}
