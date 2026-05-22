package profile

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// Profile field encryption wire versions. Modern clients emit version 1
// (AES-256-GCM-SIV); version 0 (AES-256-GCM) is still encountered in the
// wild for older profiles.
const (
	versionAESGCM    byte = 0
	versionAESGCMSIV byte = 1
)

var profileKeyInfo = []byte("ProfileKey")

// Cipher decrypts profile field blobs (name, about, emoji, etc.) using
// the recipient's 32-byte profile key. The wire format matches
// libsignal-service-java's ProfileCipher.
type Cipher struct {
	aesKey []byte
}

// NewCipher constructs a ProfileCipher from a 32-byte profile key.
func NewCipher(profileKey []byte) (*Cipher, error) {
	if err := libsignal.ValidateProfileKey(profileKey); err != nil {
		return nil, fmt.Errorf("profile.NewCipher: %w", err)
	}
	aesKey, err := libsignal.HKDFSHA256(32, profileKey, profileKeyInfo, nil)
	if err != nil {
		return nil, fmt.Errorf("profile.NewCipher: %w", err)
	}
	return &Cipher{aesKey: aesKey}, nil
}

// DecryptString decrypts a profile string field (name, about, aboutEmoji).
// Returns the empty string for nil/empty input.
func (c *Cipher) DecryptString(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	plain, err := c.decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DecryptName splits a decrypted profile name into given and family parts.
// Signal encodes names as "given\0family".
func DecryptName(c *Cipher, ciphertext []byte) (given, family string, err error) {
	raw, err := c.DecryptString(ciphertext)
	if err != nil {
		return "", "", err
	}
	if raw == "" {
		return "", "", nil
	}
	parts := strings.SplitN(raw, "\x00", 2)
	given = parts[0]
	if len(parts) == 2 {
		family = parts[1]
	}
	return given, family, nil
}

// DecryptWithLength decrypts a length-prefixed blob (payment address).
func (c *Cipher) DecryptWithLength(ciphertext []byte) ([]byte, error) {
	plain, err := c.decrypt(ciphertext)
	if err != nil {
		return nil, err
	}
	if len(plain) < 4 {
		return nil, errors.New("profile: truncated length-prefixed field")
	}
	length := int(binary.BigEndian.Uint32(plain[:4]))
	if length < 0 || length > len(plain)-4 {
		return nil, errors.New("profile: invalid length-prefixed field")
	}
	return plain[4 : 4+length], nil
}

// VerifyUnidentifiedAccess reports whether the profile's
// unidentifiedAccess verifier matches the UAK derived from our profile key.
func (c *Cipher) VerifyUnidentifiedAccess(profileKey, verifier []byte) (bool, error) {
	if len(verifier) == 0 {
		return false, nil
	}
	uak, err := libsignal.DeriveAccessKey(profileKey)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(uak[:])
	got := sum[:len(verifier)]
	if len(got) != len(verifier) {
		return false, nil
	}
	match := true
	for i := range verifier {
		if got[i] != verifier[i] {
			match = false
		}
	}
	return match, nil
}

func (c *Cipher) decrypt(input []byte) ([]byte, error) {
	if len(input) < 2 {
		return nil, errors.New("profile: ciphertext too short")
	}
	version := input[0]
	switch version {
	case versionAESGCMSIV:
		return c.decryptGCMSIV(input[1:])
	case versionAESGCM:
		return c.decryptGCM(input[1:])
	default:
		return nil, fmt.Errorf("profile: unsupported profile field version %d", version)
	}
}

func (c *Cipher) decryptGCMSIV(body []byte) ([]byte, error) {
	const nonceLen = 12
	if len(body) < nonceLen+16 {
		return nil, errors.New("profile: GCM-SIV blob too short")
	}
	nonce := body[:nonceLen]
	ct := body[nonceLen:]
	aead, err := libsignal.NewAes256GcmSiv(c.aesKey)
	if err != nil {
		return nil, err
	}
	return aead.Decrypt(ct, nonce, nil)
}

func (c *Cipher) decryptGCM(body []byte) ([]byte, error) {
	const nonceLen = 12
	if len(body) < nonceLen+16 {
		return nil, errors.New("profile: GCM blob too short")
	}
	nonce := body[:nonceLen]
	ct := body[nonceLen:]
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, fmt.Errorf("profile: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("profile: gcm: %w", err)
	}
	return gcm.Open(nil, nonce, ct, nil)
}

// EncryptStringForTest encrypts a string field using AES-GCM-SIV (version 1).
// Exported for round-trip tests only.
func EncryptStringForTest(profileKey []byte, plaintext string, nonce []byte) ([]byte, error) {
	c, err := NewCipher(profileKey)
	if err != nil {
		return nil, err
	}
	aead, err := libsignal.NewAes256GcmSiv(c.aesKey)
	if err != nil {
		return nil, err
	}
	ct, err := aead.Encrypt([]byte(plaintext), nonce, nil)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+len(nonce)+len(ct))
	out[0] = versionAESGCMSIV
	copy(out[1:], nonce)
	copy(out[1+len(nonce):], ct)
	return out, nil
}
