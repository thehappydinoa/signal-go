package libsignal

/*
#include <stdlib.h>
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// SVRKeyLen is the byte length of a Signal SVR / master key derived from
// an AccountEntropyPool. libsignal exposed this as the `SignalSVR_KEY_LEN`
// macro through v0.97.2; v0.101.2 dropped it in favor of an anonymous
// FixedArray32 typedef on the FFI signatures, so this is now a literal
// (confirmed against signal_ffi.h).
const SVRKeyLen = 32

// DeriveBackupKey derives the 32-byte backup key from an AccountEntropyPool.
func DeriveBackupKey(accountEntropyPool string) ([BackupKeyLen]byte, error) {
	var out [BackupKeyLen]byte
	if accountEntropyPool == "" {
		return out, errors.New("libsignal.DeriveBackupKey: empty account entropy pool")
	}
	cstr := C.CString(accountEntropyPool)
	defer C.free(unsafe.Pointer(cstr))
	var key [BackupKeyLen]C.uint8_t
	if err := checkError(C.signal_account_entropy_pool_derive_backup_key((*C.SignalType_FixedArray32_uint8_t)(unsafe.Pointer(&key)), cInt8(cstr))); err != nil {
		return out, err
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&key), C.int(BackupKeyLen)))
	return out, nil
}

// DeriveSVRKey derives the 32-byte master key from an AccountEntropyPool
// string via libsignal's signal_account_entropy_pool_derive_svr_key.
func DeriveSVRKey(accountEntropyPool string) ([SVRKeyLen]byte, error) {
	var out [SVRKeyLen]byte
	if accountEntropyPool == "" {
		return out, errors.New("libsignal.DeriveSVRKey: empty account entropy pool")
	}
	cstr := C.CString(accountEntropyPool)
	defer C.free(unsafe.Pointer(cstr))
	var key [SVRKeyLen]C.uint8_t
	if err := checkError(C.signal_account_entropy_pool_derive_svr_key((*C.SignalType_FixedArray32_uint8_t)(unsafe.Pointer(&key)), cInt8(cstr))); err != nil {
		return out, err
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&key), C.int(SVRKeyLen)))
	return out, nil
}

// GenerateAccountEntropyPool returns a new random AccountEntropyPool string.
func GenerateAccountEntropyPool() (string, error) {
	var out C.SignalCStringPtr
	if err := checkError(C.signal_account_entropy_pool_generate(&out)); err != nil {
		return "", err
	}
	defer C.signal_free_string(out)
	return C.GoString(cChar(out)), nil
}

// ValidateAccountEntropyPool reports whether accountEntropyPool is a valid
// AccountEntropyPool string according to libsignal.
func ValidateAccountEntropyPool(accountEntropyPool string) error {
	if accountEntropyPool == "" {
		return errors.New("libsignal.ValidateAccountEntropyPool: empty")
	}
	cstr := C.CString(accountEntropyPool)
	defer C.free(unsafe.Pointer(cstr))
	var ok C.bool
	if err := checkError(C.signal_account_entropy_pool_is_valid(&ok, cInt8(cstr))); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("libsignal.ValidateAccountEntropyPool: invalid pool")
	}
	return nil
}
