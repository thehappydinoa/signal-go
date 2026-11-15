package signal

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/pkg/signal/username"
)

// PublishedUsernameLink is a username link once it has been published
// to the server. The Handle is the UUID portion of the share URL; the
// Entropy is the client-side secret a discoverer needs to decrypt the
// server-stored ciphertext. Together they form a sharable link.
type PublishedUsernameLink struct {
	Handle  string
	Entropy []byte
}

// ShareURL returns the signal.me URL that reveals the published
// username when opened in a Signal client. The format matches what
// Signal-Desktop and Signal-Android produce.
func (p PublishedUsernameLink) ShareURL() string {
	if p.Handle == "" || len(p.Entropy) == 0 {
		return ""
	}
	return fmt.Sprintf("https://signal.me/#eu/%s/%s", p.Handle, base64.RawURLEncoding.EncodeToString(p.Entropy))
}

// ReserveUsername submits candidates in priority order and returns
// whichever one the server is willing to hold. The reservation expires
// in about five minutes; call [Client.ConfirmUsername] within that
// window to finalize.
//
// Returns a non-nil error wrapping [web.Error] StatusCode 409 if every
// candidate was already taken.
func (c *Client) ReserveUsername(ctx context.Context, candidates []username.Candidate) (username.Candidate, error) {
	if len(candidates) == 0 {
		return "", errors.New("signal.ReserveUsername: no candidates")
	}
	hashes := make([][]byte, 0, len(candidates))
	byHash := make(map[[libsignal.UsernameHashLen]byte]username.Candidate, len(candidates))
	for i, cand := range candidates {
		h, err := cand.Hash()
		if err != nil {
			return "", fmt.Errorf("signal.ReserveUsername: hash candidate %d: %w", i, err)
		}
		hashes = append(hashes, append([]byte(nil), h[:]...))
		byHash[h] = cand
	}
	res, err := c.webc.ReserveUsernameHash(ctx, c.credentials(), hashes)
	if err != nil {
		return "", fmt.Errorf("signal.ReserveUsername: %w", err)
	}
	var key [libsignal.UsernameHashLen]byte
	if n := copy(key[:], res.UsernameHash); n != libsignal.UsernameHashLen {
		return "", fmt.Errorf("signal.ReserveUsername: server returned %d-byte hash", n)
	}
	cand, ok := byHash[key]
	if !ok {
		return "", errors.New("signal.ReserveUsername: server returned a hash not in our candidate list")
	}
	return cand, nil
}

// ConfirmUsername finalizes reserved, generates a fresh username link
// for it, publishes both in a single round trip, and returns the
// published link. Nothing is persisted locally; the caller stores the
// username itself. Call within ~5 minutes of [Client.ReserveUsername].
//
// Error wraps [web.Error] StatusCode 409 if the reservation lapsed,
// 410 if the username was taken by another account between reserve
// and confirm.
func (c *Client) ConfirmUsername(ctx context.Context, reserved username.Candidate) (*PublishedUsernameLink, error) {
	if reserved == "" {
		return nil, errors.New("signal.ConfirmUsername: empty candidate")
	}
	hash, err := reserved.Hash()
	if err != nil {
		return nil, fmt.Errorf("signal.ConfirmUsername: hash: %w", err)
	}
	proof, err := reserved.Proof(nil) // auto-randomness
	if err != nil {
		return nil, fmt.Errorf("signal.ConfirmUsername: proof: %w", err)
	}
	link, err := username.NewLink(string(reserved))
	if err != nil {
		return nil, fmt.Errorf("signal.ConfirmUsername: link: %w", err)
	}
	res, err := c.webc.ConfirmUsernameHash(ctx, c.credentials(), hash[:], proof, link.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("signal.ConfirmUsername: %w", err)
	}
	return &PublishedUsernameLink{
		Handle:  res.UsernameLinkHandle,
		Entropy: link.Entropy,
	}, nil
}

// DeleteUsername removes any username and its published link from the
// account. Safe to call on an account with no username (the server
// returns 204 in both cases).
func (c *Client) DeleteUsername(ctx context.Context) error {
	if err := c.webc.DeleteUsernameHash(ctx, c.credentials()); err != nil {
		return fmt.Errorf("signal.DeleteUsername: %w", err)
	}
	return nil
}

// RotateUsernameLink re-encrypts current with fresh ciphertext and
// publishes it. If keepHandle is true, the server preserves the
// existing handle so the share URL stays stable; if false, a fresh
// handle is issued (useful after a suspected compromise).
//
// 409 if the account has no username yet — call ConfirmUsername first.
func (c *Client) RotateUsernameLink(ctx context.Context, current username.Candidate, keepHandle bool) (*PublishedUsernameLink, error) {
	if current == "" {
		return nil, errors.New("signal.RotateUsernameLink: empty candidate")
	}
	link, err := username.NewLink(string(current))
	if err != nil {
		return nil, fmt.Errorf("signal.RotateUsernameLink: encrypt: %w", err)
	}
	res, err := c.webc.SetUsernameLink(ctx, c.credentials(), link.Ciphertext, keepHandle)
	if err != nil {
		return nil, fmt.Errorf("signal.RotateUsernameLink: %w", err)
	}
	return &PublishedUsernameLink{
		Handle:  res.UsernameLinkHandle,
		Entropy: link.Entropy,
	}, nil
}

// DeleteUsernameLink removes only the account's published link; the
// username itself stays associated with the account. Useful when
// taking a bot offline for maintenance without losing the username.
func (c *Client) DeleteUsernameLink(ctx context.Context) error {
	if err := c.webc.DeleteUsernameLink(ctx, c.credentials()); err != nil {
		return fmt.Errorf("signal.DeleteUsernameLink: %w", err)
	}
	return nil
}

// LookupUsernameHash resolves a 32-byte server hash to the ACI UUID of
// the account that confirmed it. Uses an unauthenticated request — the
// server rejects this endpoint when authenticated, which matches
// Signal's discovery model.
//
// Returns an error wrapping [web.Error] StatusCode 404 if no account
// owns the hash.
func (c *Client) LookupUsernameHash(ctx context.Context, hash [HashLen]byte) (string, error) {
	res, err := c.webc.LookupUsernameHash(ctx, hash[:])
	if err != nil {
		return "", fmt.Errorf("signal.LookupUsernameHash: %w", err)
	}
	return res.ACI, nil
}

// HashLen is the byte length of a username server hash, re-exported for
// callers of [Client.LookupUsernameHash].
const HashLen = libsignal.UsernameHashLen

// LookupUsername fetches the ciphertext stored under handle, decrypts
// it with entropy, and returns the plaintext candidate. The caller
// typically extracts handle + entropy from a signal.me URL.
//
// Returns an error wrapping [web.Error] StatusCode 404 if the handle
// has no associated ciphertext (link deleted or never published).
func (c *Client) LookupUsername(ctx context.Context, handle string, entropy []byte) (username.Candidate, error) {
	if handle == "" {
		return "", errors.New("signal.LookupUsername: empty handle")
	}
	if len(entropy) != 32 {
		return "", fmt.Errorf("signal.LookupUsername: entropy must be 32 bytes, got %d", len(entropy))
	}
	res, err := c.webc.LookupUsernameLink(ctx, handle)
	if err != nil {
		return "", fmt.Errorf("signal.LookupUsername: %w", err)
	}
	return username.DecryptLink(entropy, res.UsernameLinkEncryptedValue)
}
