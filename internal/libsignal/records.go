package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
)

// SessionRecord wraps an opaque libsignal session record. Production code
// rarely constructs these directly: libsignal builds them inside
// session_cipher_decrypt / pre_key_signal_message decrypt and we hand
// them to the SessionStore for persistence as []byte blobs.
type SessionRecord struct {
	raw C.SignalMutPointerSessionRecord
}

// DeserializeSessionRecord rehydrates a session record from its byte
// encoding. The libsignal-allocated object is freed via finalizer.
func DeserializeSessionRecord(data []byte) (*SessionRecord, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeSessionRecord: empty input")
	}
	var out C.SignalMutPointerSessionRecord
	err := checkError(C.signal_session_record_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapSessionRecord(out), nil
}

// Serialize returns the canonical encoding of the session record. Callers
// hand the result to their store.SessionStore implementation.
func (r *SessionRecord) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_session_record_serialize(&buf, r.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (r *SessionRecord) constPtr() C.SignalConstPointerSessionRecord {
	return C.SignalConstPointerSessionRecord{raw: r.raw.raw}
}

func (r *SessionRecord) rawMut() C.SignalMutPointerSessionRecord { return r.raw }

func wrapSessionRecord(raw C.SignalMutPointerSessionRecord) *SessionRecord {
	r := &SessionRecord{raw: raw}
	runtime.SetFinalizer(r, func(r *SessionRecord) {
		_ = checkError(C.signal_session_record_destroy(r.raw))
	})
	return r
}

// PreKeyRecord wraps an opaque libsignal one-time prekey record.
type PreKeyRecord struct {
	raw C.SignalMutPointerPreKeyRecord
}

// NewPreKeyRecord builds a record from an id + keypair. Used at link
// time when we generate the initial batches.
func NewPreKeyRecord(id uint32, priv *PrivateKey, pub *PublicKey) (*PreKeyRecord, error) {
	if priv == nil || pub == nil {
		return nil, errors.New("libsignal.NewPreKeyRecord: nil key")
	}
	var out C.SignalMutPointerPreKeyRecord
	if err := checkError(C.signal_pre_key_record_new(&out, C.uint32_t(id), pub.constPtr(), priv.constPtr())); err != nil {
		return nil, err
	}
	return wrapPreKeyRecord(out), nil
}

// DeserializePreKeyRecord rehydrates the record from its byte encoding.
func DeserializePreKeyRecord(data []byte) (*PreKeyRecord, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializePreKeyRecord: empty input")
	}
	var out C.SignalMutPointerPreKeyRecord
	err := checkError(C.signal_pre_key_record_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapPreKeyRecord(out), nil
}

// Serialize returns the canonical encoding.
func (r *PreKeyRecord) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_pre_key_record_serialize(&buf, r.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// ID returns the prekey identifier.
func (r *PreKeyRecord) ID() (uint32, error) {
	var out C.uint32_t
	if err := checkError(C.signal_pre_key_record_get_id(&out, r.constPtr())); err != nil {
		return 0, err
	}
	return uint32(out), nil
}

func (r *PreKeyRecord) constPtr() C.SignalConstPointerPreKeyRecord {
	return C.SignalConstPointerPreKeyRecord{raw: r.raw.raw}
}

func (r *PreKeyRecord) rawMut() C.SignalMutPointerPreKeyRecord { return r.raw }

func wrapPreKeyRecord(raw C.SignalMutPointerPreKeyRecord) *PreKeyRecord {
	r := &PreKeyRecord{raw: raw}
	runtime.SetFinalizer(r, func(r *PreKeyRecord) {
		_ = checkError(C.signal_pre_key_record_destroy(r.raw))
	})
	return r
}

// SignedPreKeyRecord wraps an opaque libsignal signed prekey record.
type SignedPreKeyRecord struct {
	raw C.SignalMutPointerSignedPreKeyRecord
}

// DeserializeSignedPreKeyRecord rehydrates the record from its byte
// encoding.
func DeserializeSignedPreKeyRecord(data []byte) (*SignedPreKeyRecord, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeSignedPreKeyRecord: empty input")
	}
	var out C.SignalMutPointerSignedPreKeyRecord
	err := checkError(C.signal_signed_pre_key_record_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapSignedPreKeyRecord(out), nil
}

// Serialize returns the canonical encoding.
func (r *SignedPreKeyRecord) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_signed_pre_key_record_serialize(&buf, r.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (r *SignedPreKeyRecord) constPtr() C.SignalConstPointerSignedPreKeyRecord {
	return C.SignalConstPointerSignedPreKeyRecord{raw: r.raw.raw}
}

func (r *SignedPreKeyRecord) rawMut() C.SignalMutPointerSignedPreKeyRecord { return r.raw }

func wrapSignedPreKeyRecord(raw C.SignalMutPointerSignedPreKeyRecord) *SignedPreKeyRecord {
	r := &SignedPreKeyRecord{raw: raw}
	runtime.SetFinalizer(r, func(r *SignedPreKeyRecord) {
		_ = checkError(C.signal_signed_pre_key_record_destroy(r.raw))
	})
	return r
}

// KyberPreKeyRecord wraps an opaque libsignal Kyber prekey record.
type KyberPreKeyRecord struct {
	raw C.SignalMutPointerKyberPreKeyRecord
}

// DeserializeKyberPreKeyRecord rehydrates the record from its byte
// encoding.
func DeserializeKyberPreKeyRecord(data []byte) (*KyberPreKeyRecord, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeKyberPreKeyRecord: empty input")
	}
	var out C.SignalMutPointerKyberPreKeyRecord
	err := checkError(C.signal_kyber_pre_key_record_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapKyberPreKeyRecord(out), nil
}

// Serialize returns the canonical encoding.
func (r *KyberPreKeyRecord) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_kyber_pre_key_record_serialize(&buf, r.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (r *KyberPreKeyRecord) constPtr() C.SignalConstPointerKyberPreKeyRecord {
	return C.SignalConstPointerKyberPreKeyRecord{raw: r.raw.raw}
}

func (r *KyberPreKeyRecord) rawMut() C.SignalMutPointerKyberPreKeyRecord { return r.raw }

func wrapKyberPreKeyRecord(raw C.SignalMutPointerKyberPreKeyRecord) *KyberPreKeyRecord {
	r := &KyberPreKeyRecord{raw: raw}
	runtime.SetFinalizer(r, func(r *KyberPreKeyRecord) {
		_ = checkError(C.signal_kyber_pre_key_record_destroy(r.raw))
	})
	return r
}

// SenderKeyRecord wraps an opaque libsignal sender-key record (groups v2).
type SenderKeyRecord struct {
	raw C.SignalMutPointerSenderKeyRecord
}

// DeserializeSenderKeyRecord rehydrates the record from its byte
// encoding.
func DeserializeSenderKeyRecord(data []byte) (*SenderKeyRecord, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeSenderKeyRecord: empty input")
	}
	var out C.SignalMutPointerSenderKeyRecord
	err := checkError(C.signal_sender_key_record_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapSenderKeyRecord(out), nil
}

// Serialize returns the canonical encoding.
func (r *SenderKeyRecord) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_sender_key_record_serialize(&buf, r.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (r *SenderKeyRecord) constPtr() C.SignalConstPointerSenderKeyRecord {
	return C.SignalConstPointerSenderKeyRecord{raw: r.raw.raw}
}

func (r *SenderKeyRecord) rawMut() C.SignalMutPointerSenderKeyRecord { return r.raw }

func wrapSenderKeyRecord(raw C.SignalMutPointerSenderKeyRecord) *SenderKeyRecord {
	r := &SenderKeyRecord{raw: raw}
	runtime.SetFinalizer(r, func(r *SenderKeyRecord) {
		_ = checkError(C.signal_sender_key_record_destroy(r.raw))
	})
	return r
}
