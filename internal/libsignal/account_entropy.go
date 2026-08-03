package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// SVRKeyLen is the byte length of a Signal SVR / master key derived from
// an AccountEntropyPool.
const SVRKeyLen = 32

// DeriveBackupKey derives the 32-byte backup key from an AccountEntropyPool.
func DeriveBackupKey(accountEntropyPool string) ([BackupKeyLen]byte, error) {
	var out [BackupKeyLen]byte
	if accountEntropyPool == "" {
		return out, errors.New("libsignal.DeriveBackupKey: empty account entropy pool")
	}
	cstr := C.CString(accountEntropyPool)
	defer C.free(unsafe.Pointer(cstr))
	var key [32]C.uint8_t
	if err := checkError(C.signal_account_entropy_pool_derive_backup_key((*C.SignalType_FixedArray32_uint8_t)(unsafe.Pointer(&key)), (*C.int8_t)(unsafe.Pointer(cstr)))); err != nil {
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
	var key [32]C.uint8_t
	if err := checkError(C.signal_account_entropy_pool_derive_svr_key((*C.SignalType_FixedArray32_uint8_t)(unsafe.Pointer(&key)), (*C.int8_t)(unsafe.Pointer(cstr)))); err != nil {
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
	defer C.signal_free_string((*C.int8_t)(unsafe.Pointer(out)))
	return C.GoString((*C.char)(unsafe.Pointer(out))), nil
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
	if err := checkError(C.signal_account_entropy_pool_is_valid(&ok, (*C.int8_t)(unsafe.Pointer(cstr)))); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("libsignal.ValidateAccountEntropyPool: invalid pool")
	}
	return nil
}
