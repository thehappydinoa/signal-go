package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
)

// Aes256GcmSiv wraps libsignal's AES-256-GCM-SIV primitive. Used by
// profile field decryption (ProfileCipher) and other callers that need
// the same AEAD Signal clients use for profile blobs.
type Aes256GcmSiv struct {
	raw C.SignalMutPointerAes256GcmSiv
}

// NewAes256GcmSiv constructs an AES-256-GCM-SIV cipher from a 32-byte key.
func NewAes256GcmSiv(key []byte) (*Aes256GcmSiv, error) {
	if len(key) != 32 {
		return nil, errors.New("libsignal.NewAes256GcmSiv: key must be 32 bytes")
	}
	var out C.SignalMutPointerAes256GcmSiv
	if err := checkError(C.signal_aes256_gcm_siv_new(&out, borrowed(key))); err != nil {
		return nil, err
	}
	keepAlive(key)
	a := &Aes256GcmSiv{raw: out}
	runtime.SetFinalizer(a, (*Aes256GcmSiv).destroy)
	return a, nil
}

// Decrypt decrypts ciphertext with the given 12-byte nonce and optional
// associated data. Returns plaintext on success.
func (a *Aes256GcmSiv) Decrypt(ciphertext, nonce, associatedData []byte) ([]byte, error) {
	if len(nonce) != 12 {
		return nil, errors.New("libsignal.Aes256GcmSiv.Decrypt: nonce must be 12 bytes")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_aes256_gcm_siv_decrypt(
		&buf,
		a.constPtr(),
		borrowed(ciphertext),
		borrowed(nonce),
		borrowed(associatedData),
	)); err != nil {
		return nil, err
	}
	keepAlive(ciphertext)
	keepAlive(nonce)
	keepAlive(associatedData)
	return goBytesFromOwnedBuffer(buf), nil
}

// Encrypt encrypts plaintext with the given 12-byte nonce and optional
// associated data. Primarily used by tests to build ProfileCipher vectors.
func (a *Aes256GcmSiv) Encrypt(plaintext, nonce, associatedData []byte) ([]byte, error) {
	if len(nonce) != 12 {
		return nil, errors.New("libsignal.Aes256GcmSiv.Encrypt: nonce must be 12 bytes")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_aes256_gcm_siv_encrypt(
		&buf,
		a.constPtr(),
		borrowed(plaintext),
		borrowed(nonce),
		borrowed(associatedData),
	)); err != nil {
		return nil, err
	}
	keepAlive(plaintext)
	keepAlive(nonce)
	keepAlive(associatedData)
	return goBytesFromOwnedBuffer(buf), nil
}

func (a *Aes256GcmSiv) constPtr() C.SignalConstPointerAes256GcmSiv {
	return C.SignalConstPointerAes256GcmSiv{raw: a.raw.raw}
}

func (a *Aes256GcmSiv) destroy() {
	if a.raw.raw != nil {
		C.signal_aes256_gcm_siv_destroy(a.raw)
		a.raw.raw = nil
	}
}
