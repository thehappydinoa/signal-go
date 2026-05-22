package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// BackupKeyLen is the byte length of a Signal backup key.
const BackupKeyLen = int(C.SignalBACKUP_KEY_LEN)

// BackupIDLen is the byte length of a derived backup ID.
const BackupIDLen = 16

// DeriveBackupID derives the 16-byte backup ID for aci from backupKey.
func DeriveBackupID(backupKey [BackupKeyLen]byte, aci string) ([BackupIDLen]byte, error) {
	var out [BackupIDLen]byte
	sid, err := ParseServiceIDString(aci)
	if err != nil {
		return out, fmt.Errorf("libsignal.DeriveBackupID: %w", err)
	}
	cKey := (*[BackupKeyLen]C.uint8_t)(unsafe.Pointer(&backupKey[0]))
	var cOut [BackupIDLen]C.uint8_t
	if err := checkError(C.signal_backup_key_derive_backup_id(&cOut, cKey, cServiceID(sid))); err != nil {
		return out, err
	}
	for i := range out {
		out[i] = byte(cOut[i])
	}
	return out, nil
}
