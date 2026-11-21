package libsignal

/*
#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// RegisterResponseBadge is one entry from the badges list the server
// returned with a RegisterAccountResponse.
type RegisterResponseBadge struct {
	// ID is the server-assigned badge identifier.
	ID string
	// Visible tracks whether the badge is user-visible.
	Visible bool
	// ExpirationSecs is the badge expiration, in seconds since Unix epoch.
	ExpirationSecs float64
}

// RegisterAccountResponse is the libsignal response shape returned by a
// successful register_account_without_number call. Callers copy the
// accessor return values out; the handle is destroyed on Close.
type RegisterAccountResponse struct {
	ptr *C.SignalRegisterAccountResponse
}

// ACI copies the new account's ACI out as a 16-byte UUID.
func (r *RegisterAccountResponse) ACI() ([16]byte, error) {
	var out [16]byte
	if r == nil || r.ptr == nil {
		return out, errors.New("libsignal.RegisterAccountResponse.ACI: closed")
	}
	var u C.SignalUuid
	if err := checkError(C.signal_register_account_response_get_aci(&u, r.cPtr())); err != nil {
		return out, err
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&u), 16))
	return out, nil
}

// PNI copies the new account's PNI out as a 16-byte UUID. Returns zero +
// ok=false if the response had no PNI.
func (r *RegisterAccountResponse) PNI() ([16]byte, bool, error) {
	var out [16]byte
	if r == nil || r.ptr == nil {
		return out, false, errors.New("libsignal.RegisterAccountResponse.PNI: closed")
	}
	var opt C.SignalOptionalOfUuid
	if err := checkError(C.signal_register_account_response_get_pni(&opt, r.cPtr())); err != nil {
		return out, false, err
	}
	if !bool(opt.present) {
		return out, false, nil
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&opt.value), 16))
	return out, true, nil
}

// Number returns the E.164 phone number the server bound to the new
// account. For paid no-number registration this is the empty string.
func (r *RegisterAccountResponse) Number() (string, error) {
	if r == nil || r.ptr == nil {
		return "", errors.New("libsignal.RegisterAccountResponse.Number: closed")
	}
	var out C.SignalCStringPtr
	if err := checkError(C.signal_register_account_response_get_number(&out, r.cPtr())); err != nil {
		return "", err
	}
	defer C.signal_free_string((*C.int8_t)(unsafe.Pointer(out)))
	return C.GoString((*C.char)(unsafe.Pointer(out))), nil
}

// UsernameHash copies the 32-byte username hash the account was
// registered with, or nil if no username was presented.
func (r *RegisterAccountResponse) UsernameHash() ([]byte, error) {
	if r == nil || r.ptr == nil {
		return nil, errors.New("libsignal.RegisterAccountResponse.UsernameHash: closed")
	}
	var out C.SignalOwnedBuffer
	if err := checkError(C.signal_register_account_response_get_username_hash(&out, r.cPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(out), nil
}

// UsernameLinkHandle returns the username link handle UUID the account
// registered with, if any.
func (r *RegisterAccountResponse) UsernameLinkHandle() ([16]byte, bool, error) {
	var out [16]byte
	if r == nil || r.ptr == nil {
		return out, false, errors.New("libsignal.RegisterAccountResponse.UsernameLinkHandle: closed")
	}
	var opt C.SignalOptionalOfUuid
	if err := checkError(C.signal_register_account_response_get_username_link_handle(&opt, r.cPtr())); err != nil {
		return out, false, err
	}
	if !bool(opt.present) {
		return out, false, nil
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&opt.value), 16))
	return out, true, nil
}

// AuthCredentialSalt returns the auth credential salt the server issued.
func (r *RegisterAccountResponse) AuthCredentialSalt() ([]byte, error) {
	if r == nil || r.ptr == nil {
		return nil, errors.New("libsignal.RegisterAccountResponse.AuthCredentialSalt: closed")
	}
	var out C.SignalOwnedBuffer
	if err := checkError(C.signal_register_account_response_get_auth_credential_salt(&out, r.cPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(out), nil
}

// EntitlementBackupLevel returns the backup entitlement level (0 = none).
func (r *RegisterAccountResponse) EntitlementBackupLevel() (uint64, error) {
	if r == nil || r.ptr == nil {
		return 0, errors.New("libsignal.RegisterAccountResponse.EntitlementBackupLevel: closed")
	}
	var out C.uint64_t
	if err := checkError(C.signal_register_account_response_get_entitlement_backup_level(&out, r.cPtr())); err != nil {
		return 0, err
	}
	return uint64(out), nil
}

// EntitlementBackupExpirationSeconds returns the backup entitlement
// expiration, in seconds since Unix epoch.
func (r *RegisterAccountResponse) EntitlementBackupExpirationSeconds() (uint64, error) {
	if r == nil || r.ptr == nil {
		return 0, errors.New("libsignal.RegisterAccountResponse.EntitlementBackupExpirationSeconds: closed")
	}
	var out C.uint64_t
	if err := checkError(C.signal_register_account_response_get_entitlement_backup_expiration_seconds(&out, r.cPtr())); err != nil {
		return 0, err
	}
	return uint64(out), nil
}

// EntitlementBadges returns the badges the server granted the new account.
func (r *RegisterAccountResponse) EntitlementBadges() ([]RegisterResponseBadge, error) {
	if r == nil || r.ptr == nil {
		return nil, errors.New("libsignal.RegisterAccountResponse.EntitlementBadges: closed")
	}
	var out C.SignalOwnedBufferOfFfiRegisterResponseBadge
	if err := checkError(C.signal_register_account_response_get_entitlement_badges(&out, r.cPtr())); err != nil {
		return nil, err
	}
	badgeSize := C.size_t(unsafe.Sizeof(C.SignalFfiRegisterResponseBadge{}))
	if out.base == nil || out.length == 0 {
		if out.base != nil {
			C.signal_free_buffer((*C.uchar)(unsafe.Pointer(out.base)), out.length*badgeSize)
		}
		return nil, nil
	}
	defer C.signal_free_buffer(
		(*C.uchar)(unsafe.Pointer(out.base)),
		out.length*badgeSize,
	)
	badges := unsafe.Slice(out.base, int(out.length))
	res := make([]RegisterResponseBadge, len(badges))
	for i := range badges {
		if badges[i].id != nil {
			res[i].ID = C.GoString((*C.char)(unsafe.Pointer(badges[i].id)))
		}
		res[i].Visible = bool(badges[i].visible)
		res[i].ExpirationSecs = float64(badges[i].expiration_secs)
	}
	return res, nil
}

// IsReregistration reports whether the server treated the call as a
// re-registration of an existing account.
func (r *RegisterAccountResponse) IsReregistration() (bool, error) {
	if r == nil || r.ptr == nil {
		return false, errors.New("libsignal.RegisterAccountResponse.IsReregistration: closed")
	}
	var out C.bool
	if err := checkError(C.signal_register_account_response_get_reregistration(&out, r.cPtr())); err != nil {
		return false, err
	}
	return bool(out), nil
}

// IsStorageCapable reports whether the account is eligible to read and
// write the storage service.
func (r *RegisterAccountResponse) IsStorageCapable() (bool, error) {
	if r == nil || r.ptr == nil {
		return false, errors.New("libsignal.RegisterAccountResponse.IsStorageCapable: closed")
	}
	var out C.bool
	if err := checkError(C.signal_register_account_response_get_storage_capable(&out, r.cPtr())); err != nil {
		return false, err
	}
	return bool(out), nil
}

// Close destroys the response handle. Idempotent.
func (r *RegisterAccountResponse) Close() {
	if r == nil || r.ptr == nil {
		return
	}
	C.signal_register_account_response_destroy(C.SignalMutPointerRegisterAccountResponse{raw: r.ptr})
	r.ptr = nil
}

func (r *RegisterAccountResponse) cPtr() C.SignalConstPointerRegisterAccountResponse {
	return C.SignalConstPointerRegisterAccountResponse{raw: r.ptr}
}
