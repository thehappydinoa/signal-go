// Package seal implements AES-256-GCM encryption for at-rest account blobs
// (ADR 0012). [internal/store/sqlstore] uses it for the account row in
// signal.db; kdf.json lives beside the database directory.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// KeyLen is the AES-256 key size for encrypted stores (ADR 0012).
const KeyLen = 32

// ErrWrongPassphrase is returned when AEAD decryption fails (wrong key or
// corrupted blob).
var ErrWrongPassphrase = errors.New("seal: decryption failed (wrong passphrase or corrupted store)")

// Seal encrypts plaintext with key using the ADR 0012 wire format.
func Seal(key [KeyLen]byte, plaintext []byte) ([]byte, error) {
	return seal(key, plaintext)
}

// Open decrypts a blob produced by [Seal].
func Open(key [KeyLen]byte, blob []byte) ([]byte, error) {
	return open(key, blob)
}

const (
	formatVersion byte = 0x01
	nonceLen      int  = 12
)

func seal(key [KeyLen]byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("seal: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("seal: gcm init: %w", err)
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("seal: read nonce: %w", err)
	}
	out := make([]byte, 0, 1+nonceLen+len(plaintext)+gcm.Overhead())
	out = append(out, formatVersion)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plaintext, nil)
	return out, nil
}

func open(key [KeyLen]byte, blob []byte) ([]byte, error) {
	if len(blob) < 1+nonceLen+16 {
		return nil, fmt.Errorf("seal: encrypted blob too short (%d bytes)", len(blob))
	}
	if blob[0] != formatVersion {
		return nil, fmt.Errorf("seal: unsupported encrypted format version 0x%02x", blob[0])
	}
	nonce := blob[1 : 1+nonceLen]
	ciphertext := blob[1+nonceLen:]
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("seal: aes init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("seal: gcm init: %w", err)
	}
	pt, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w (%s)", ErrWrongPassphrase, err.Error())
	}
	return pt, nil
}
