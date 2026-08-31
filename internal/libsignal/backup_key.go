package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// BackupKeyLen is the byte length of a Signal backup key. libsignal exposed
// this as the `SignalBACKUP_KEY_LEN` macro through v0.97.2; v0.101.2 dropped
// it in favor of an anonymous FixedArray32 typedef on the FFI signatures, so
// this is now a literal (confirmed against signal_ffi.h).
const BackupKeyLen = 32

// BackupIDLen is the byte length of a derived backup ID.
const BackupIDLen = 16

// DeriveBackupID derives the 16-byte backup ID for aci from backupKey.
func DeriveBackupID(backupKey [BackupKeyLen]byte, aci string) ([BackupIDLen]byte, error) {
	var out [BackupIDLen]byte
	sid, err := ParseServiceIDString(aci)
	if err != nil {
		return out, fmt.Errorf("libsignal.DeriveBackupID: %w", err)
	}
	// backup_key (const input) and aci (const input, via cServiceID) resolve
	// to the plain anonymous array pointer type on GCC-style hosts; out (the
	// non-const output param) resolves to the named FixedArray16 type. See
	// service_id.go's cServiceID doc for the GCC/clang cgo split.
	cKey := (*[BackupKeyLen]C.uint8_t)(unsafe.Pointer(&backupKey[0]))
	var cOut [BackupIDLen]C.uint8_t
	if err := checkError(C.signal_backup_key_derive_backup_id((*C.SignalType_FixedArray16_uint8_t)(unsafe.Pointer(&cOut)), cKey, cServiceID(sid))); err != nil {
		return out, err
	}
	for i := range out {
		out[i] = byte(cOut[i])
	}
	return out, nil
}
