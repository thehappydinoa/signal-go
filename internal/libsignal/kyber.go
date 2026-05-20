package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
)

// KyberKeyPair is a post-quantum (ML-KEM) keypair owned by libsignal,
// used by Signal's PQXDH key agreement (now mandatory).
type KyberKeyPair struct {
	raw C.SignalMutPointerKyberKeyPair
}

// KyberPublicKey is the public half of a [KyberKeyPair].
type KyberPublicKey struct {
	raw C.SignalMutPointerKyberPublicKey
}

// KyberSecretKey is the secret half of a [KyberKeyPair].
type KyberSecretKey struct {
	raw C.SignalMutPointerKyberSecretKey
}

// GenerateKyberKeyPair creates a fresh ML-KEM keypair using libsignal's
// vetted implementation.
func GenerateKyberKeyPair() (*KyberKeyPair, error) {
	var out C.SignalMutPointerKyberKeyPair
	if err := checkError(C.signal_kyber_key_pair_generate(&out)); err != nil {
		return nil, err
	}
	return wrapKyberKeyPair(out), nil
}

// Public derives the public half. Stable across calls.
func (k *KyberKeyPair) Public() (*KyberPublicKey, error) {
	var out C.SignalMutPointerKyberPublicKey
	if err := checkError(C.signal_kyber_key_pair_get_public_key(&out, k.constPtr())); err != nil {
		return nil, err
	}
	return wrapKyberPublicKey(out), nil
}

// Secret derives the secret half. Stable across calls.
func (k *KyberKeyPair) Secret() (*KyberSecretKey, error) {
	var out C.SignalMutPointerKyberSecretKey
	if err := checkError(C.signal_kyber_key_pair_get_secret_key(&out, k.constPtr())); err != nil {
		return nil, err
	}
	return wrapKyberSecretKey(out), nil
}

func (k *KyberKeyPair) constPtr() C.SignalConstPointerKyberKeyPair {
	return C.SignalConstPointerKyberKeyPair{raw: k.raw.raw}
}

func wrapKyberKeyPair(raw C.SignalMutPointerKyberKeyPair) *KyberKeyPair {
	k := &KyberKeyPair{raw: raw}
	runtime.SetFinalizer(k, func(k *KyberKeyPair) {
		_ = checkError(C.signal_kyber_key_pair_destroy(k.raw))
	})
	return k
}

// Serialize returns the canonical byte encoding of the Kyber public key.
func (k *KyberPublicKey) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_kyber_public_key_serialize(&buf, k.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (k *KyberPublicKey) constPtr() C.SignalConstPointerKyberPublicKey {
	return C.SignalConstPointerKyberPublicKey{raw: k.raw.raw}
}

func wrapKyberPublicKey(raw C.SignalMutPointerKyberPublicKey) *KyberPublicKey {
	k := &KyberPublicKey{raw: raw}
	runtime.SetFinalizer(k, func(k *KyberPublicKey) {
		_ = checkError(C.signal_kyber_public_key_destroy(k.raw))
	})
	return k
}

// DeserializeKyberPublicKey re-hydrates a public key from its canonical
// encoding.
func DeserializeKyberPublicKey(b []byte) (*KyberPublicKey, error) {
	if len(b) == 0 {
		return nil, errors.New("libsignal.DeserializeKyberPublicKey: empty input")
	}
	var out C.SignalMutPointerKyberPublicKey
	err := checkError(C.signal_kyber_public_key_deserialize(&out, borrowed(b)))
	keepAlive(b)
	if err != nil {
		return nil, err
	}
	return wrapKyberPublicKey(out), nil
}

// Serialize returns the canonical byte encoding of the Kyber secret key.
func (k *KyberSecretKey) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_kyber_secret_key_serialize(&buf, k.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (k *KyberSecretKey) constPtr() C.SignalConstPointerKyberSecretKey {
	return C.SignalConstPointerKyberSecretKey{raw: k.raw.raw}
}

func wrapKyberSecretKey(raw C.SignalMutPointerKyberSecretKey) *KyberSecretKey {
	k := &KyberSecretKey{raw: raw}
	runtime.SetFinalizer(k, func(k *KyberSecretKey) {
		_ = checkError(C.signal_kyber_secret_key_destroy(k.raw))
	})
	return k
}
