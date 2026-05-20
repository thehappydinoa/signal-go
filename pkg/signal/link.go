package signal

import (
	"context"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/provisioning"
)

// LinkOptions configures [Link].
type LinkOptions struct {
	// OnURL is called once the linking URL is ready. Typical implementations
	// print the URL or render it as a QR code in the terminal. Required.
	OnURL func(linkURL string) error
	// UserAgent reported to Signal in the X-Signal-Agent header.
	// Defaults to "signal-go".
	UserAgent string
}

// IdentityKey pairs the public + private halves of one of the linked
// account's long-term identity keys. There are two: one for the ACI
// (account identifier) and one for the PNI (phone-number identifier).
type IdentityKey struct {
	Public  []byte // 33 bytes: 0x05 prefix + 32-byte X25519 point
	Private []byte // 32 bytes
}

// LinkedAccount is the decoded provisioning material delivered by the user's
// primary device after a successful QR-link handshake.
//
// In Phase 1 we hand this back to the caller as-is. Phase 2's later steps
// will use it to generate prekeys and register against `PUT /v1/devices/link`,
// turning a LinkedAccount into a registered, persistable Account.
type LinkedAccount struct {
	// ACI is the account identifier (UUID v4 string).
	ACI string
	// PNI is the phone-number identifier (UUID v4 string).
	PNI string
	// Number is the E.164 phone number.
	Number string
	// ProvisioningCode is the short-lived code the primary device sent that
	// we present to Signal's server during /v1/devices/link.
	ProvisioningCode string
	// ACIIdentityKey is the long-term identity keypair for the ACI.
	ACIIdentityKey IdentityKey
	// PNIIdentityKey is the long-term identity keypair for the PNI.
	PNIIdentityKey IdentityKey
	// ProfileKey is the 32-byte profile encryption key.
	ProfileKey []byte
	// ReadReceipts is the account's read-receipts preference.
	ReadReceipts bool
	// AccountEntropyPool, if present, is the master backup secret. Empty
	// for accounts that have not enrolled in the new backup system.
	AccountEntropyPool string
}

// Link runs the secondary-device QR-link handshake against
// chat.signal.org. On success it returns the decrypted [LinkedAccount]
// material from the user's primary device.
//
// The user must scan the URL passed to opts.OnURL with their primary
// device's "Linked devices" menu within the Signal mobile app. ctx
// cancellation aborts the wait.
func Link(ctx context.Context, opts LinkOptions) (*LinkedAccount, error) {
	if opts.OnURL == nil {
		return nil, errors.New("signal.Link: LinkOptions.OnURL is required")
	}
	sess, err := provisioning.Link(ctx, provisioning.Options{
		UserAgent: opts.UserAgent,
		OnURL:     opts.OnURL,
	})
	if err != nil {
		return nil, err
	}
	return convertSession(sess)
}

// convertSession projects a provisioning.Session onto the public
// [LinkedAccount] type.
func convertSession(sess *provisioning.Session) (*LinkedAccount, error) {
	if sess == nil || sess.Message == nil {
		return nil, errors.New("signal: empty provisioning session")
	}
	msg := sess.Message

	// Either aci/pni or aciBinary/pniBinary may be present depending on
	// upstream client version. The string forms are canonical for our
	// public API; we'll normalise the binary forms later if needed.
	if msg.GetAci() == "" || msg.GetNumber() == "" || msg.GetProvisioningCode() == "" {
		return nil, fmt.Errorf("signal: provisioning message missing required fields")
	}
	if err := validateIdentityKey("ACI", msg.GetAciIdentityKeyPublic(), msg.GetAciIdentityKeyPrivate()); err != nil {
		return nil, err
	}
	if err := validateIdentityKey("PNI", msg.GetPniIdentityKeyPublic(), msg.GetPniIdentityKeyPrivate()); err != nil {
		return nil, err
	}

	return &LinkedAccount{
		ACI:                msg.GetAci(),
		PNI:                msg.GetPni(),
		Number:             msg.GetNumber(),
		ProvisioningCode:   msg.GetProvisioningCode(),
		ACIIdentityKey:     IdentityKey{Public: msg.GetAciIdentityKeyPublic(), Private: msg.GetAciIdentityKeyPrivate()},
		PNIIdentityKey:     IdentityKey{Public: msg.GetPniIdentityKeyPublic(), Private: msg.GetPniIdentityKeyPrivate()},
		ProfileKey:         msg.GetProfileKey(),
		ReadReceipts:       msg.GetReadReceipts(),
		AccountEntropyPool: msg.GetAccountEntropyPool(),
	}, nil
}

// validateIdentityKey confirms the provisioning message handed us a
// well-formed keypair by deserializing both halves through libsignal.
func validateIdentityKey(label string, pub, priv []byte) error {
	if len(pub) == 0 || len(priv) == 0 {
		return fmt.Errorf("signal: %s identity key missing", label)
	}
	if _, err := libsignal.DeserializePublicKey(pub); err != nil {
		return fmt.Errorf("signal: %s public key invalid: %w", label, err)
	}
	if _, err := libsignal.DeserializePrivateKey(priv); err != nil {
		return fmt.Errorf("signal: %s private key invalid: %w", label, err)
	}
	return nil
}
