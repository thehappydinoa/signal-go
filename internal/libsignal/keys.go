package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
)

// PrivateKey is a Curve25519 private scalar owned by libsignal.
// Constructed via [GeneratePrivateKey] or [DeserializePrivateKey]; freed by
// a finalizer when no longer referenced.
type PrivateKey struct {
	raw C.SignalMutPointerPrivateKey
}

// PublicKey is a Curve25519 public point owned by libsignal.
type PublicKey struct {
	raw C.SignalMutPointerPublicKey
}

// IdentityKeyPair groups a public/private Curve25519 keypair used as a
// long-term identity. ACI and PNI each have one.
type IdentityKeyPair struct {
	Private *PrivateKey
	Public  *PublicKey
}

// GeneratePrivateKey creates a fresh Curve25519 private key using
// libsignal's CSPRNG.
func GeneratePrivateKey() (*PrivateKey, error) {
	var out C.SignalMutPointerPrivateKey
	if err := checkError(C.signal_privatekey_generate(&out)); err != nil {
		return nil, err
	}
	return wrapPrivateKey(out), nil
}

// DeserializePrivateKey re-hydrates a private key from its 32-byte
// serialization. Returns an error if the bytes do not decode to a valid
// scalar.
func DeserializePrivateKey(b []byte) (*PrivateKey, error) {
	if len(b) == 0 {
		return nil, errors.New("libsignal.DeserializePrivateKey: empty input")
	}
	var out C.SignalMutPointerPrivateKey
	err := checkError(C.signal_privatekey_deserialize(&out, borrowed(b)))
	keepAlive(b)
	if err != nil {
		return nil, err
	}
	return wrapPrivateKey(out), nil
}

// Serialize returns the 32-byte canonical encoding of the private key.
func (k *PrivateKey) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_privatekey_serialize(&buf, k.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// PublicKey derives the matching public key.
func (k *PrivateKey) PublicKey() (*PublicKey, error) {
	var out C.SignalMutPointerPublicKey
	if err := checkError(C.signal_privatekey_get_public_key(&out, k.constPtr())); err != nil {
		return nil, err
	}
	return wrapPublicKey(out), nil
}

func (k *PrivateKey) constPtr() C.SignalConstPointerPrivateKey {
	return C.SignalConstPointerPrivateKey{raw: k.raw.raw}
}

func wrapPrivateKey(raw C.SignalMutPointerPrivateKey) *PrivateKey {
	k := &PrivateKey{raw: raw}
	runtime.SetFinalizer(k, func(k *PrivateKey) {
		// Destructors return SignalFfiError* but on success-of-destroy we
		// have nothing meaningful to do with it.
		_ = checkError(C.signal_privatekey_destroy(k.raw))
	})
	return k
}

// DeserializePublicKey re-hydrates a public key from its 33-byte
// type-tagged encoding.
func DeserializePublicKey(b []byte) (*PublicKey, error) {
	if len(b) == 0 {
		return nil, errors.New("libsignal.DeserializePublicKey: empty input")
	}
	var out C.SignalMutPointerPublicKey
	err := checkError(C.signal_publickey_deserialize(&out, borrowed(b)))
	keepAlive(b)
	if err != nil {
		return nil, err
	}
	return wrapPublicKey(out), nil
}

// Serialize returns the 33-byte canonical encoding of the public key.
// The first byte is the libsignal type tag (0x05 = Curve25519); the rest
// are the X25519 point.
func (k *PublicKey) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_publickey_serialize(&buf, k.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (k *PublicKey) constPtr() C.SignalConstPointerPublicKey {
	return C.SignalConstPointerPublicKey{raw: k.raw.raw}
}

func wrapPublicKey(raw C.SignalMutPointerPublicKey) *PublicKey {
	k := &PublicKey{raw: raw}
	runtime.SetFinalizer(k, func(k *PublicKey) {
		_ = checkError(C.signal_publickey_destroy(k.raw))
	})
	return k
}

// GenerateIdentityKeyPair creates a fresh long-term identity keypair.
func GenerateIdentityKeyPair() (*IdentityKeyPair, error) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		return nil, err
	}
	pub, err := priv.PublicKey()
	if err != nil {
		return nil, err
	}
	return &IdentityKeyPair{Private: priv, Public: pub}, nil
}
