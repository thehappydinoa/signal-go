package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"
)

// BackupPurpose matches libsignal_message_backup::backup::Purpose.
type BackupPurpose uint8

const (
	// BackupPurposeDeviceTransfer is link-and-sync / device transfer archives.
	BackupPurposeDeviceTransfer BackupPurpose = 0
	// BackupPurposeRemoteBackup is Signal Secure Backups remote storage.
	BackupPurposeRemoteBackup BackupPurpose = 1
	// BackupPurposeTakeoutExport is human-readable exports.
	BackupPurposeTakeoutExport BackupPurpose = 2
)

// MessageBackupKey decrypts and validates message backup bundles.
type MessageBackupKey struct {
	raw C.SignalMutPointerMessageBackupKey
}

// NewMessageBackupKeyFromBackupKeyAndBackupID derives a message backup key.
// forwardSecrecyToken may be nil for device-transfer archives.
func NewMessageBackupKeyFromBackupKeyAndBackupID(
	backupKey [BackupKeyLen]byte,
	backupID [BackupIDLen]byte,
	forwardSecrecyToken *[32]byte,
) (*MessageBackupKey, error) {
	cKey := (*[BackupKeyLen]C.uint8_t)(unsafe.Pointer(&backupKey[0]))
	cID := (*[BackupIDLen]C.uint8_t)(unsafe.Pointer(&backupID[0]))
	var cToken *[32]C.uint8_t
	if forwardSecrecyToken != nil {
		cToken = (*[32]C.uint8_t)(unsafe.Pointer(forwardSecrecyToken))
	}
	var out C.SignalMutPointerMessageBackupKey
	if err := checkError(C.signal_message_backup_key_from_backup_key_and_backup_id(
		&out, cKey, cID, cToken,
	)); err != nil {
		return nil, err
	}
	k := &MessageBackupKey{raw: out}
	runtime.SetFinalizer(k, (*MessageBackupKey).destroy)
	return k, nil
}

// NewMessageBackupKeyForEphemeralTransfer derives the message backup key used
// for linked-device synchronized start from the one-time ephemeral backup key
// and the account ACI.
func NewMessageBackupKeyForEphemeralTransfer(ephemeralKey [BackupKeyLen]byte, aci string) (*MessageBackupKey, error) {
	backupID, err := DeriveBackupID(ephemeralKey, aci)
	if err != nil {
		return nil, fmt.Errorf("libsignal.NewMessageBackupKeyForEphemeralTransfer: %w", err)
	}
	return NewMessageBackupKeyFromBackupKeyAndBackupID(ephemeralKey, backupID, nil)
}

// AesKey returns the AES-256 key for decrypting backup archives.
func (k *MessageBackupKey) AesKey() ([32]byte, error) {
	if k == nil || k.raw.raw == nil {
		return [32]byte{}, errors.New("libsignal.MessageBackupKey.AesKey: nil key")
	}
	var out [32]C.uint8_t
	if err := checkError(C.signal_message_backup_key_get_aes_key(&out, C.SignalConstPointerMessageBackupKey{raw: k.raw.raw})); err != nil {
		return [32]byte{}, err
	}
	var key [32]byte
	for i := range key {
		key[i] = byte(out[i])
	}
	return key, nil
}

// HmacKey returns the HMAC-SHA256 key for backup archive authentication.
func (k *MessageBackupKey) HmacKey() ([32]byte, error) {
	if k == nil || k.raw.raw == nil {
		return [32]byte{}, errors.New("libsignal.MessageBackupKey.HmacKey: nil key")
	}
	var out [32]C.uint8_t
	if err := checkError(C.signal_message_backup_key_get_hmac_key(&out, C.SignalConstPointerMessageBackupKey{raw: k.raw.raw})); err != nil {
		return [32]byte{}, err
	}
	var key [32]byte
	for i := range key {
		key[i] = byte(out[i])
	}
	return key, nil
}

// MessageBackupValidationOutcome is the result of validating a backup archive.
type MessageBackupValidationOutcome struct {
	raw C.SignalMutPointerMessageBackupValidationOutcome
}

// OK reports whether validation succeeded.
func (o *MessageBackupValidationOutcome) OK() bool {
	if o == nil || o.raw.raw == nil {
		return false
	}
	msg, err := o.ErrorMessage()
	return err == nil && msg == ""
}

// ErrorMessage returns a developer-facing validation error, or empty on success.
func (o *MessageBackupValidationOutcome) ErrorMessage() (string, error) {
	if o == nil || o.raw.raw == nil {
		return "", errors.New("libsignal.MessageBackupValidationOutcome: nil outcome")
	}
	var cstr *C.char
	if err := checkError(C.signal_message_backup_validation_outcome_get_error_message(&cstr, o.constPtr())); err != nil {
		return "", err
	}
	if cstr == nil {
		return "", nil
	}
	defer C.signal_free_string(cstr)
	return C.GoString(cstr), nil
}

// UnknownFields lists unknown protobuf fields encountered during validation.
func (o *MessageBackupValidationOutcome) UnknownFields() ([]string, error) {
	if o == nil || o.raw.raw == nil {
		return nil, errors.New("libsignal.MessageBackupValidationOutcome: nil outcome")
	}
	var arr C.SignalStringArray
	if err := checkError(C.signal_message_backup_validation_outcome_get_unknown_fields(&arr, o.constPtr())); err != nil {
		return nil, err
	}
	defer C.signal_free_bytestring_array(arr)
	chunks := goBytestringArrayFromC(arr)
	out := make([]string, len(chunks))
	for i, b := range chunks {
		out[i] = string(b)
	}
	return out, nil
}

func (o *MessageBackupValidationOutcome) constPtr() C.SignalConstPointerMessageBackupValidationOutcome {
	return C.SignalConstPointerMessageBackupValidationOutcome{raw: o.raw.raw}
}

// Close releases the native validation outcome handle.
func (o *MessageBackupValidationOutcome) Close() {
	o.destroy()
}

func (o *MessageBackupValidationOutcome) destroy() {
	if o.raw.raw != nil {
		C.signal_message_backup_validation_outcome_destroy(o.raw)
		o.raw.raw = nil
	}
}

// ValidateMessageBackup decrypts and validates an encrypted backup archive.
func ValidateMessageBackup(key *MessageBackupKey, purpose BackupPurpose, ciphertext []byte) (*MessageBackupValidationOutcome, error) {
	if key == nil || key.raw.raw == nil {
		return nil, errors.New("libsignal.ValidateMessageBackup: nil key")
	}
	if len(ciphertext) == 0 {
		return nil, errors.New("libsignal.ValidateMessageBackup: empty ciphertext")
	}
	first := newBytesInputStream(ciphertext)
	defer first.release()
	second := newBytesInputStream(ciphertext)
	defer second.release()

	var pinner runtime.Pinner
	if len(ciphertext) > 0 {
		pinner.Pin(&ciphertext[0])
	}
	first.pin(&pinner)
	second.pin(&pinner)
	defer pinner.Unpin()

	var out C.SignalMutPointerMessageBackupValidationOutcome
	if err := checkError(C.signal_message_backup_validator_validate(
		&out,
		C.SignalConstPointerMessageBackupKey{raw: key.raw.raw},
		first.ptr(),
		second.ptr(),
		C.uint64_t(len(ciphertext)),
		C.uint8_t(purpose),
	)); err != nil {
		return nil, err
	}
	o := &MessageBackupValidationOutcome{raw: out}
	runtime.SetFinalizer(o, (*MessageBackupValidationOutcome).destroy)
	return o, nil
}

// Close releases the native message backup key handle.
func (k *MessageBackupKey) Close() {
	k.destroy()
}

func (k *MessageBackupKey) destroy() {
	if k.raw.raw != nil {
		C.signal_message_backup_key_destroy(k.raw)
		k.raw.raw = nil
	}
}
