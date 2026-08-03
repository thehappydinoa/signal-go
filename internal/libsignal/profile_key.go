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

// ProfileKeyLen is the byte length of a Signal profile encryption key.
const ProfileKeyLen = 32

// AccessKeyLen is the byte length of an unidentified access key (UAK).
const AccessKeyLen = 16

// ProfileKeyVersionEncodedLen is the hex-encoded profile key version string
// length returned by [ProfileKeyVersion].
const ProfileKeyVersionEncodedLen = 64

func copyProfileKey(pk *[32]C.uchar, src []byte) {
	for i, b := range src {
		pk[i] = C.uchar(b)
	}
}

// DeriveAccessKey derives the 16-byte unidentified access key from a
// 32-byte profile key using libsignal's ProfileKey::derive_access_key
// (AES-256-ECB on a zero block with last byte = 2 — not HKDF).
func DeriveAccessKey(profileKey []byte) ([AccessKeyLen]byte, error) {
	var out [AccessKeyLen]byte
	if len(profileKey) != ProfileKeyLen {
		return out, fmt.Errorf("libsignal.DeriveAccessKey: profile key length %d, want %d", len(profileKey), ProfileKeyLen)
	}
	var pk [32]C.uchar
	copyProfileKey(&pk, profileKey)
	var uak [16]C.uint8_t
	if err := checkError(C.signal_profile_key_derive_access_key((*C.SignalType_FixedArray16_uint8_t)(unsafe.Pointer(&uak)), &pk)); err != nil {
		return out, err
	}
	copy(out[:], C.GoBytes(unsafe.Pointer(&uak), C.int(AccessKeyLen)))
	return out, nil
}

// ProfileKeyVersion returns the 64-byte hex ASCII profile key version for
// the given (profileKey, aci) pair. The version is used as a path segment
// in GET /v1/profile/{aci}/{version}.
func ProfileKeyVersion(profileKey []byte, aci string) (string, error) {
	if len(profileKey) != ProfileKeyLen {
		return "", fmt.Errorf("libsignal.ProfileKeyVersion: profile key length %d, want %d", len(profileKey), ProfileKeyLen)
	}
	if aci == "" {
		return "", errors.New("libsignal.ProfileKeyVersion: empty aci")
	}
	cstr := C.CString(aci)
	defer C.free(unsafe.Pointer(cstr))
	var sid C.SignalServiceIdFixedWidthBinaryBytes
	if err := checkError(C.signal_service_id_parse_from_service_id_string(&sid, (*C.int8_t)(unsafe.Pointer(cstr)))); err != nil {
		return "", err
	}
	var pk [32]C.uchar
	copyProfileKey(&pk, profileKey)
	var version [64]C.uint8_t
	if err := checkError(C.signal_profile_key_get_profile_key_version(
		(*C.SignalType_FixedArray64_uint8_t)(unsafe.Pointer(&version)),
		&pk,
		cServiceIDPtr(&sid),
	)); err != nil {
		return "", err
	}
	return string(C.GoBytes(unsafe.Pointer(&version), C.int(ProfileKeyVersionEncodedLen))), nil
}

// ValidateProfileKey checks that profileKey has the correct length for
// downstream profile / UAK operations.
func ValidateProfileKey(profileKey []byte) error {
	if len(profileKey) != ProfileKeyLen {
		return errors.New("libsignal.ValidateProfileKey: want 32-byte profile key")
	}
	return nil
}
