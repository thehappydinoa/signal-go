package libsignal

/*
#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

// IdentityType selects which identity (ACI or PNI) a RegisterAccountRequest
// setter applies to. The uint8 values match libsignal's FfiIdentityType.
type IdentityType uint8

const (
	// IdentityACI is the account's primary identity namespace.
	IdentityACI IdentityType = 0
	// IdentityPNI is the account's phone-number identity namespace.
	IdentityPNI IdentityType = 1
)

// SignedECPreKey is an EC signed pre-key handed to a RegisterAccountRequest.
type SignedECPreKey struct {
	KeyID     uint32
	PublicKey *PublicKey
	Signature []byte
}

// SignedKyberPreKey is a Kyber (ML-KEM) signed pre-key handed to a
// RegisterAccountRequest.
type SignedKyberPreKey struct {
	KeyID     uint32
	PublicKey *KyberPublicKey
	Signature []byte
}

// RegisterAccountRequest is a libsignal RegisterAccountRequest builder.
// Call [NewRegisterAccountRequest], populate via its setters, and hand it
// to [RegistrationService.RegisterAccountWithoutNumber] (added in a follow-up
// commit). Close is idempotent.
type RegisterAccountRequest struct {
	ptr *C.SignalRegisterAccountRequest
}

// NewRegisterAccountRequest allocates an empty RegisterAccountRequest.
func NewRegisterAccountRequest() (*RegisterAccountRequest, error) {
	var out C.SignalMutPointerRegisterAccountRequest
	if err := checkError(C.signal_register_account_request_create(&out)); err != nil {
		return nil, err
	}
	return &RegisterAccountRequest{ptr: out.raw}, nil
}

// SetAccountPassword sets the base64-encoded authentication password the
// server will accept for the new account.
func (r *RegisterAccountRequest) SetAccountPassword(password string) error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetAccountPassword: closed")
	}
	cstr := C.CString(password)
	defer C.free(unsafe.Pointer(cstr))
	return checkError(C.signal_register_account_request_set_account_password(
		r.cPtr(), (*C.int8_t)(unsafe.Pointer(cstr)),
	))
}

// SetAPNPushToken registers an APNs push token for the account. Headless
// Go bots almost never need this; included for FFI completeness.
func (r *RegisterAccountRequest) SetAPNPushToken(token string) error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetAPNPushToken: closed")
	}
	cstr := C.CString(token)
	defer C.free(unsafe.Pointer(cstr))
	return checkError(C.signal_register_account_request_set_apn_push_token(
		r.cPtr(), (*C.int8_t)(unsafe.Pointer(cstr)),
	))
}

// SetIdentityPublicKey sets the EC identity public key for the given
// identity namespace.
func (r *RegisterAccountRequest) SetIdentityPublicKey(id IdentityType, key *PublicKey) error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetIdentityPublicKey: closed")
	}
	if key == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetIdentityPublicKey: nil public key")
	}
	err := checkError(C.signal_register_account_request_set_identity_public_key(
		r.cPtr(), C.uint8_t(id), key.constPtr(),
	))
	runtime.KeepAlive(key)
	return err
}

// SetIdentitySignedPreKey sets the EC signed pre-key for the given identity.
func (r *RegisterAccountRequest) SetIdentitySignedPreKey(id IdentityType, pk SignedECPreKey) error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetIdentitySignedPreKey: closed")
	}
	if pk.PublicKey == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetIdentitySignedPreKey: nil public key")
	}
	cPk := C.SignalFfiSignedPublicPreKey{
		key_id:          C.uint32_t(pk.KeyID),
		public_key_type: C.SignalFfiPublicKeyType(publicKeyTypeECC),
		public_key:      unsafe.Pointer(pk.PublicKey.raw.raw),
		signature:       borrowed(pk.Signature),
	}
	err := checkError(C.signal_register_account_request_set_identity_signed_pre_key(
		r.cPtr(), C.uint8_t(id), cPk,
	))
	runtime.KeepAlive(pk.PublicKey)
	keepAlive(pk.Signature)
	return err
}

// SetIdentityPQLastResortPreKey sets the Kyber last-resort pre-key for the
// given identity.
func (r *RegisterAccountRequest) SetIdentityPQLastResortPreKey(id IdentityType, pk SignedKyberPreKey) error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetIdentityPQLastResortPreKey: closed")
	}
	if pk.PublicKey == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetIdentityPQLastResortPreKey: nil public key")
	}
	cPk := C.SignalFfiSignedPublicPreKey{
		key_id:          C.uint32_t(pk.KeyID),
		public_key_type: C.SignalFfiPublicKeyType(publicKeyTypeKyber),
		public_key:      unsafe.Pointer(pk.PublicKey.raw.raw),
		signature:       borrowed(pk.Signature),
	}
	err := checkError(C.signal_register_account_request_set_identity_pq_last_resort_pre_key(
		r.cPtr(), C.uint8_t(id), cPk,
	))
	runtime.KeepAlive(pk.PublicKey)
	keepAlive(pk.Signature)
	return err
}

// SetOneTimePassword sets the one-time password for registration transport
// verification. Ignored by the no-number flow; included for FFI completeness.
func (r *RegisterAccountRequest) SetOneTimePassword(pwd uint32) error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetOneTimePassword: closed")
	}
	return checkError(C.signal_register_account_request_set_one_time_password(
		r.cPtr(), C.uint32_t(pwd),
	))
}

// SetSkipDeviceTransfer marks the new account as not transferring state
// from an existing device.
func (r *RegisterAccountRequest) SetSkipDeviceTransfer() error {
	if r == nil || r.ptr == nil {
		return errors.New("libsignal.RegisterAccountRequest.SetSkipDeviceTransfer: closed")
	}
	return checkError(C.signal_register_account_request_set_skip_device_transfer(r.cPtr()))
}

// Close destroys the request. Idempotent.
func (r *RegisterAccountRequest) Close() {
	if r == nil || r.ptr == nil {
		return
	}
	C.signal_register_account_request_destroy(C.SignalMutPointerRegisterAccountRequest{raw: r.ptr})
	r.ptr = nil
}

func (r *RegisterAccountRequest) cPtr() C.SignalConstPointerRegisterAccountRequest {
	return C.SignalConstPointerRegisterAccountRequest{raw: r.ptr}
}

// libsignal public_key_type discriminator values.
const (
	publicKeyTypeECC   uint8 = 0
	publicKeyTypeKyber uint8 = 1
)
