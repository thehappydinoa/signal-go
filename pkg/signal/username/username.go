// Package username provides local-only helpers for Signal's username
// feature: candidate generation from a nickname, server-hash
// derivation, zero-knowledge proof of ownership, and username-link
// encrypt/decrypt.
//
// Server-facing operations (reserving a candidate, confirming
// ownership, publishing a link, looking up by hash or link) live on
// [signal.Client] because they use its authenticated or unauthenticated
// chat connection. See [ADR 0041] for the full surface; this package is
// the local half.
//
// [ADR 0041]: https://github.com/thehappydinoa/signal-go/blob/main/docs/adr/0041-usernames.md
package username

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// HashLen is the byte length of a Signal username server hash.
const HashLen = libsignal.UsernameHashLen

// DefaultMinLen and DefaultMaxLen are the standard length bounds Signal
// clients pass for candidate generation.
const (
	DefaultMinLen uint32 = 3
	DefaultMaxLen uint32 = 32
)

// Candidate is one "nickname.discriminator" string produced by
// [Candidates]. The zero value is not a valid candidate.
type Candidate string

// Candidates generates up to a short list of candidate usernames derived
// from nickname. minLen and maxLen bound the total username length
// (nickname + "." + discriminator). Pass 0 for either to use the
// Signal-standard bounds.
func Candidates(nickname string, minLen, maxLen uint32) ([]Candidate, error) {
	if nickname == "" {
		return nil, errors.New("signal/username: empty nickname")
	}
	if minLen == 0 {
		minLen = DefaultMinLen
	}
	if maxLen == 0 {
		maxLen = DefaultMaxLen
	}
	raw, err := libsignal.UsernameCandidates(nickname, minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("signal/username: Candidates: %w", err)
	}
	out := make([]Candidate, len(raw))
	for i, c := range raw {
		out[i] = Candidate(c)
	}
	return out, nil
}

// Hash returns the 32-byte server hash of c. The hash is what the
// server stores under the account and what [Verify] takes.
func (c Candidate) Hash() ([HashLen]byte, error) {
	var out [HashLen]byte
	h, err := libsignal.UsernameHash(string(c))
	if err != nil {
		return out, fmt.Errorf("signal/username: Hash: %w", err)
	}
	return h, nil
}

// Proof generates a zero-knowledge proof that the caller owns c,
// bound to randomness. If randomness is nil, Proof generates fresh
// 32 bytes from crypto/rand.
func (c Candidate) Proof(randomness []byte) ([]byte, error) {
	var seed [32]byte
	switch {
	case randomness == nil:
		if _, err := rand.Read(seed[:]); err != nil {
			return nil, fmt.Errorf("signal/username: Proof: rand: %w", err)
		}
	case len(randomness) == 32:
		copy(seed[:], randomness)
	default:
		return nil, fmt.Errorf("signal/username: Proof: randomness must be 32 bytes, got %d", len(randomness))
	}
	p, err := libsignal.UsernameProof(string(c), seed)
	if err != nil {
		return nil, fmt.Errorf("signal/username: Proof: %w", err)
	}
	return p, nil
}

// Verify checks that proof demonstrates ownership of a username whose
// hash matches hash. Returns an error if not.
func Verify(proof []byte, hash [HashLen]byte) error {
	if err := libsignal.VerifyUsernameProof(proof, hash); err != nil {
		return fmt.Errorf("signal/username: Verify: %w", err)
	}
	return nil
}

// Link is a published username link: the server-stored ciphertext plus
// the entropy the discoverer needs to decrypt it. Share
// [Link.ShareString] as the public handle.
type Link struct {
	// Entropy is 32 bytes of random secret; the server never sees this.
	Entropy []byte
	// Ciphertext is the server-stored encrypted username payload.
	Ciphertext []byte
}

// NewLink encrypts username and returns a [Link]. The Ciphertext is
// what the caller publishes to the server (via
// signal.Client.SetUsernameLink); the Entropy is the secret embedded
// in the shareable URL.
func NewLink(username string) (*Link, error) {
	raw, err := libsignal.UsernameLinkCreate(username, nil)
	if err != nil {
		return nil, fmt.Errorf("signal/username: NewLink: %w", err)
	}
	if len(raw) < 32 {
		return nil, fmt.Errorf("signal/username: NewLink: short payload (%d bytes)", len(raw))
	}
	return &Link{
		Entropy:    append([]byte(nil), raw[:32]...),
		Ciphertext: append([]byte(nil), raw[32:]...),
	}, nil
}

// RotateLink re-encrypts username with the existing link's entropy, so
// the published link handle keeps the same shareable URL but is
// cryptographically refreshed on the server side. Useful when
// rotating after a compromise or periodically for hygiene.
func (l *Link) RotateLink(username string) (*Link, error) {
	if len(l.Entropy) != 32 {
		return nil, errors.New("signal/username: RotateLink: entropy must be 32 bytes")
	}
	raw, err := libsignal.UsernameLinkCreate(username, l.Entropy)
	if err != nil {
		return nil, fmt.Errorf("signal/username: RotateLink: %w", err)
	}
	if len(raw) < 32 {
		return nil, fmt.Errorf("signal/username: RotateLink: short payload (%d bytes)", len(raw))
	}
	return &Link{
		Entropy:    append([]byte(nil), raw[:32]...),
		Ciphertext: append([]byte(nil), raw[32:]...),
	}, nil
}

// DecryptLink reverses [NewLink]: given the entropy from a link URL and
// the ciphertext fetched from the server, it returns the plaintext
// "nickname.discriminator" username.
func DecryptLink(entropy, ciphertext []byte) (Candidate, error) {
	s, err := libsignal.UsernameLinkDecrypt(entropy, ciphertext)
	if err != nil {
		return "", fmt.Errorf("signal/username: DecryptLink: %w", err)
	}
	return Candidate(s), nil
}
