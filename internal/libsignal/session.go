package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"time"
)

// PreKeyBundleParams holds the public material for a prekey bundle.
type PreKeyBundleParams struct {
	RegistrationID        uint32
	DeviceID              uint32
	PreKeyID              uint32
	PreKeyPublic          *PublicKey
	SignedPreKeyID        uint32
	SignedPreKeyPublic    *PublicKey
	SignedPreKeySignature []byte
	IdentityKey           *PublicKey
	KyberPreKeyID         uint32
	KyberPreKeyPublic     *KyberPublicKey
	KyberPreKeySignature  []byte
}

// NewPreKeyBundle builds a libsignal PreKeyBundle from public keys.
func NewPreKeyBundle(p PreKeyBundleParams) (*PreKeyBundle, error) {
	if p.PreKeyPublic == nil || p.SignedPreKeyPublic == nil || p.IdentityKey == nil || p.KyberPreKeyPublic == nil {
		return nil, errors.New("libsignal.NewPreKeyBundle: nil key")
	}
	var out C.SignalMutPointerPreKeyBundle
	err := checkError(C.signal_pre_key_bundle_new(
		&out,
		C.uint32_t(p.RegistrationID),
		C.uint32_t(p.DeviceID),
		C.uint32_t(p.PreKeyID),
		p.PreKeyPublic.constPtr(),
		C.uint32_t(p.SignedPreKeyID),
		p.SignedPreKeyPublic.constPtr(),
		borrowed(p.SignedPreKeySignature),
		p.IdentityKey.constPtr(),
		C.uint32_t(p.KyberPreKeyID),
		p.KyberPreKeyPublic.constPtr(),
		borrowed(p.KyberPreKeySignature),
	))
	keepAlive(p.SignedPreKeySignature)
	keepAlive(p.KyberPreKeySignature)
	if err != nil {
		return nil, err
	}
	return wrapPreKeyBundle(out), nil
}

// PreKeyBundle wraps a libsignal prekey bundle.
type PreKeyBundle struct {
	raw C.SignalMutPointerPreKeyBundle
}

func (b *PreKeyBundle) constPtr() C.SignalConstPointerPreKeyBundle {
	return C.SignalConstPointerPreKeyBundle{raw: b.raw.raw}
}

func wrapPreKeyBundle(raw C.SignalMutPointerPreKeyBundle) *PreKeyBundle {
	b := &PreKeyBundle{raw: raw}
	runtime.SetFinalizer(b, func(b *PreKeyBundle) {
		if b.raw.raw == nil {
			return
		}
		_ = checkError(C.signal_pre_key_bundle_destroy(b.raw))
		b.raw.raw = nil
	})
	return b
}

// Destroy frees the bundle. Idempotent: subsequent calls are no-ops.
// Callers should still rely on the finalizer for short-lived bundles;
// Destroy exists so long-lived bundles can free promptly.
func (b *PreKeyBundle) Destroy() {
	if b == nil || b.raw.raw == nil {
		return
	}
	_ = checkError(C.signal_pre_key_bundle_destroy(b.raw))
	b.raw.raw = nil
	runtime.SetFinalizer(b, nil)
}

// ProcessPreKeyBundle establishes a session with the bundle's owner.
func ProcessPreKeyBundle(
	bundle *PreKeyBundle,
	remote *Address,
	local *Address,
	h *StoreHandle,
	now time.Time,
) error {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(bundle)
	pinner.Pin(remote)
	pinner.Pin(local)
	h.pinForFFI(&pinner)
	return checkError(C.signal_process_prekey_bundle(
		bundle.constPtr(),
		remote.constPtr(),
		local.constPtr(),
		h.SessionStoreStruct(),
		h.IdentityKeyStoreStruct(),
		C.uint64_t(now.UnixMilli()),
	))
}

// EncryptMessage encrypts plaintext to an established session peer.
func EncryptMessage(
	ptext []byte,
	remote *Address,
	local *Address,
	h *StoreHandle,
	now time.Time,
) (*CiphertextMessage, error) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(remote)
	pinner.Pin(local)
	h.pinForFFI(&pinner)
	var out C.SignalMutPointerCiphertextMessage
	err := checkError(C.signal_encrypt_message(
		&out,
		borrowed(ptext),
		remote.constPtr(),
		local.constPtr(),
		h.SessionStoreStruct(),
		h.IdentityKeyStoreStruct(),
		C.uint64_t(now.UnixMilli()),
	))
	keepAlive(ptext)
	if err != nil {
		return nil, err
	}
	return wrapCiphertextMessage(out), nil
}

// CiphertextMessage is an encrypted outbound/inbound payload wrapper.
//
// The Rust allocation is freed by a [runtime.SetFinalizer]; callers may
// also free explicitly via [CiphertextMessage.Destroy] when the lifetime
// is well-defined (e.g. a single send call). Once Destroy has run, further
// method calls are no-ops returning an error rather than dereferencing a
// freed pointer.
type CiphertextMessage struct {
	raw C.SignalMutPointerCiphertextMessage
}

func (m *CiphertextMessage) constPtr() C.SignalConstPointerCiphertextMessage {
	return C.SignalConstPointerCiphertextMessage{raw: m.raw.raw}
}

// Type returns whisper/prekey/plaintext/sender-key.
func (m *CiphertextMessage) Type() (CiphertextMessageType, error) {
	if m == nil || m.raw.raw == nil {
		return 0, errors.New("libsignal.CiphertextMessage.Type: nil or destroyed")
	}
	var t C.uint8_t
	if err := checkError(C.signal_ciphertext_message_type(&t, m.constPtr())); err != nil {
		return 0, err
	}
	return CiphertextMessageType(t), nil
}

// Serialize returns the wire encoding.
func (m *CiphertextMessage) Serialize() ([]byte, error) {
	if m == nil || m.raw.raw == nil {
		return nil, errors.New("libsignal.CiphertextMessage.Serialize: nil or destroyed")
	}
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_ciphertext_message_serialize(&buf, m.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// Destroy frees the underlying Rust allocation. Subsequent method calls
// return an error rather than touching freed memory. Safe to call multiple
// times.
func (m *CiphertextMessage) Destroy() {
	if m == nil || m.raw.raw == nil {
		return
	}
	_ = checkError(C.signal_ciphertext_message_destroy(m.raw))
	m.raw.raw = nil
	runtime.SetFinalizer(m, nil)
}

func wrapCiphertextMessage(raw C.SignalMutPointerCiphertextMessage) *CiphertextMessage {
	m := &CiphertextMessage{raw: raw}
	runtime.SetFinalizer(m, func(m *CiphertextMessage) {
		if m.raw.raw == nil {
			return
		}
		_ = checkError(C.signal_ciphertext_message_destroy(m.raw))
		m.raw.raw = nil
	})
	return m
}

// NewSignedPreKeyRecordBlob builds a SignedPreKeyRecord serialization for storage.
func NewSignedPreKeyRecordBlob(id uint32, timestamp uint64, pub *PublicKey, priv *PrivateKey, sig []byte) ([]byte, error) {
	var out C.SignalMutPointerSignedPreKeyRecord
	if err := checkError(C.signal_signed_pre_key_record_new(
		&out,
		C.uint32_t(id),
		C.uint64_t(timestamp),
		pub.constPtr(),
		priv.constPtr(),
		borrowed(sig),
	)); err != nil {
		return nil, err
	}
	keepAlive(sig)
	rec := wrapSignedPreKeyRecord(out)
	return rec.Serialize()
}

// NewKyberPreKeyRecordBlob builds a KyberPreKeyRecord serialization for storage.
func NewKyberPreKeyRecordBlob(id uint32, timestamp uint64, kp *KyberKeyPair, sig []byte) ([]byte, error) {
	var out C.SignalMutPointerKyberPreKeyRecord
	if err := checkError(C.signal_kyber_pre_key_record_new(
		&out,
		C.uint32_t(id),
		C.uint64_t(timestamp),
		kp.constPtr(),
		borrowed(sig),
	)); err != nil {
		return nil, err
	}
	keepAlive(sig)
	rec := wrapKyberPreKeyRecord(out)
	return rec.Serialize()
}
