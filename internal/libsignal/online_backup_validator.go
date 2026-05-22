package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
)

// OnlineBackupValidator incrementally validates backup frames during import.
type OnlineBackupValidator struct {
	raw C.SignalMutPointerOnlineBackupValidator
}

// NewOnlineBackupValidator creates a validator from the first BackupInfo frame.
func NewOnlineBackupValidator(backupInfoFrame []byte, purpose BackupPurpose) (*OnlineBackupValidator, error) {
	if len(backupInfoFrame) == 0 {
		return nil, errors.New("libsignal.NewOnlineBackupValidator: empty backup info frame")
	}
	var out C.SignalMutPointerOnlineBackupValidator
	if err := checkError(C.signal_online_backup_validator_new(
		&out,
		borrowed(backupInfoFrame),
		C.uint8_t(purpose),
	)); err != nil {
		return nil, err
	}
	keepAlive(backupInfoFrame)
	v := &OnlineBackupValidator{raw: out}
	runtime.SetFinalizer(v, (*OnlineBackupValidator).destroy)
	return v, nil
}

// AddFrame validates and accepts one Frame protobuf blob.
func (v *OnlineBackupValidator) AddFrame(frame []byte) error {
	if v == nil || v.raw.raw == nil {
		return errors.New("libsignal.OnlineBackupValidator.AddFrame: nil validator")
	}
	if len(frame) == 0 {
		return errors.New("libsignal.OnlineBackupValidator.AddFrame: empty frame")
	}
	if err := checkError(C.signal_online_backup_validator_add_frame(v.raw, borrowed(frame))); err != nil {
		return err
	}
	keepAlive(frame)
	return nil
}

// Finalize completes incremental validation.
func (v *OnlineBackupValidator) Finalize() error {
	if v == nil || v.raw.raw == nil {
		return errors.New("libsignal.OnlineBackupValidator.Finalize: nil validator")
	}
	return checkError(C.signal_online_backup_validator_finalize(v.raw))
}

// Close releases the native validator handle.
func (v *OnlineBackupValidator) Close() {
	v.destroy()
}

func (v *OnlineBackupValidator) destroy() {
	if v.raw.raw != nil {
		C.signal_online_backup_validator_destroy(v.raw)
		v.raw.raw = nil
	}
}
