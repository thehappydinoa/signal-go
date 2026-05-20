package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
)

// Sign produces a 64-byte XEdDSA signature over message using priv.
// Identical to libsignal's signal_privatekey_sign; the Rust side handles
// the deterministic-nonce derivation internally.
func Sign(priv *PrivateKey, message []byte) ([]byte, error) {
	if priv == nil {
		return nil, errors.New("libsignal.Sign: nil private key")
	}
	if len(message) == 0 {
		return nil, errors.New("libsignal.Sign: empty message")
	}
	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_privatekey_sign(&buf, priv.constPtr(), borrowed(message)))
	keepAlive(message)
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// Verify reports whether signature is a valid XEdDSA signature of message
// under pub.
func Verify(pub *PublicKey, message, signature []byte) (bool, error) {
	if pub == nil {
		return false, errors.New("libsignal.Verify: nil public key")
	}
	if len(message) == 0 || len(signature) == 0 {
		return false, fmt.Errorf("libsignal.Verify: empty message or signature")
	}
	var out C.bool
	err := checkError(C.signal_publickey_verify(&out, pub.constPtr(), borrowed(message), borrowed(signature)))
	keepAlive(message)
	keepAlive(signature)
	if err != nil {
		return false, err
	}
	return bool(out), nil
}
