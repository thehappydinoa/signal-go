package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Agree performs X25519 ECDH between priv and pub, returning the 32-byte
// shared secret.
//
// Mirrors libsignal's signal_privatekey_agree, which keeps the key material
// inside Rust and does the scalar multiplication via libsignal's vetted
// Curve25519 implementation.
func Agree(priv *PrivateKey, pub *PublicKey) ([]byte, error) {
	if priv == nil || pub == nil {
		return nil, errors.New("libsignal.Agree: nil key")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_privatekey_agree(&buf, priv.constPtr(), pub.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// HKDFSHA256 derives outLen bytes of keying material from ikm using
// HKDF-SHA256 (RFC 5869). salt may be nil to use the all-zero salt; info
// is the application-specific context string.
//
// Forwards to libsignal's signal_hkdf_derive so we use the same HKDF
// implementation Signal's own clients do.
func HKDFSHA256(outLen int, ikm, info, salt []byte) ([]byte, error) {
	if outLen <= 0 {
		return nil, errors.New("libsignal.HKDFSHA256: outLen must be positive")
	}
	out := make([]byte, outLen)
	outBuf := C.SignalBorrowedMutableBuffer{
		base:   (*C.uchar)(unsafe.Pointer(&out[0])),
		length: C.size_t(outLen),
	}
	err := checkError(C.signal_hkdf_derive(outBuf, borrowed(ikm), borrowed(info), borrowed(salt)))
	keepAlive(ikm)
	keepAlive(info)
	keepAlive(salt)
	keepAlive(out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
