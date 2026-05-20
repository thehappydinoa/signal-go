package signal

import (
	"context"
	"errors"

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

// PendingLink is the Phase-1 outcome of [Link]: the QR handshake completed
// and we received an encrypted envelope from the user's primary device.
// Phase 2 will turn this into a fully-registered linked-device account.
type PendingLink struct {
	// EnvelopeBody is the encrypted ProvisionMessage payload. Phase 2's
	// decrypt step consumes it.
	EnvelopeBody []byte
	// EnvelopePublicKey is the primary device's ephemeral public key, needed
	// for the ProvisioningCipher decrypt.
	EnvelopePublicKey []byte
	// EphemeralPrivateKey is our half of the ECDH; must be kept secret.
	// Serialized form of the libsignal private key.
	EphemeralPrivateKey []byte
}

// Link runs the secondary-device QR-link handshake against
// chat.signal.org. On success it returns a [PendingLink] containing the
// material needed to complete registration in Phase 2.
//
// The user must scan the URL passed to opts.OnURL with their primary
// device's "Linked devices" menu within the Signal mobile app. ctx
// cancellation aborts the wait.
func Link(ctx context.Context, opts LinkOptions) (*PendingLink, error) {
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
	privBytes, err := sess.EphemeralKey.Private.Serialize()
	if err != nil {
		return nil, err
	}
	return &PendingLink{
		EnvelopeBody:        sess.Envelope.GetBody(),
		EnvelopePublicKey:   sess.Envelope.GetPublicKey(),
		EphemeralPrivateKey: privBytes,
	}, nil
}
