package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"time"
	"unsafe"
)

// Production sealed-sender trust root (Signal server signing key).
// See libsignal KNOWN_SERVER_CERTIFICATES id=3.
var productionTrustRootBytes = mustDecodeTrustRoot(
	"BUkY0I+9+oPgDCn4+Ac6Iu813yvqkDr/ga8DzLxFxuk6",
)

func mustDecodeTrustRoot(b64 string) []byte {
	b, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic("libsignal: invalid trust root: " + err.Error())
	}
	return b
}

// ProductionTrustRoots returns the well-known Signal production trust roots
// as deserialized [PublicKey] values. Callers must not mutate the slice.
func ProductionTrustRoots() ([]*PublicKey, error) {
	pk, err := DeserializePublicKey(productionTrustRootBytes)
	if err != nil {
		return nil, fmt.Errorf("libsignal.ProductionTrustRoots: %w", err)
	}
	return []*PublicKey{pk}, nil
}

// UnidentifiedSenderMessageContent is the inner payload of a sealed-sender
// envelope after the outer KEM unwrap.
type UnidentifiedSenderMessageContent struct {
	raw C.SignalMutPointerUnidentifiedSenderMessageContent
}

func (u *UnidentifiedSenderMessageContent) constPtr() C.SignalConstPointerUnidentifiedSenderMessageContent {
	return C.SignalConstPointerUnidentifiedSenderMessageContent{raw: u.raw.raw}
}

func wrapUSMC(raw C.SignalMutPointerUnidentifiedSenderMessageContent) *UnidentifiedSenderMessageContent {
	u := &UnidentifiedSenderMessageContent{raw: raw}
	runtime.SetFinalizer(u, func(u *UnidentifiedSenderMessageContent) {
		_ = checkError(C.signal_unidentified_sender_message_content_destroy(u.raw))
	})
	return u
}

// DecryptSealedSenderToUSMC unwraps a sealed-sender ciphertext to USMC
// using the local identity in identityStore.
func DecryptSealedSenderToUSMC(ctext []byte, h *StoreHandle) (*UnidentifiedSenderMessageContent, error) {
	if len(ctext) == 0 {
		return nil, errors.New("libsignal.DecryptSealedSenderToUSMC: empty ciphertext")
	}
	var pinner runtime.Pinner
	defer pinner.Unpin()
	h.pinForFFI(&pinner)
	var out C.SignalMutPointerUnidentifiedSenderMessageContent
	err := checkError(C.signal_sealed_session_cipher_decrypt_to_usmc(
		&out,
		borrowed(ctext),
		h.IdentityKeyStoreStruct(),
	))
	keepAlive(ctext)
	if err != nil {
		return nil, err
	}
	return wrapUSMC(out), nil
}

// MessageType returns the inner ciphertext type (whisper, prekey, etc.).
func (u *UnidentifiedSenderMessageContent) MessageType() (CiphertextMessageType, error) {
	var t C.uint8_t
	if err := checkError(C.signal_unidentified_sender_message_content_get_msg_type(&t, u.constPtr())); err != nil {
		return 0, err
	}
	return CiphertextMessageType(t), nil
}

// Contents returns the serialized inner ciphertext bytes.
func (u *UnidentifiedSenderMessageContent) Contents() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_unidentified_sender_message_content_get_contents(&buf, u.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// SenderCertificate is the authenticated sender embedded in USMC.
type SenderCertificate struct {
	raw C.SignalMutPointerSenderCertificate
}

func (c *SenderCertificate) constPtr() C.SignalConstPointerSenderCertificate {
	return C.SignalConstPointerSenderCertificate{raw: c.raw.raw}
}

func wrapSenderCertificate(raw C.SignalMutPointerSenderCertificate) *SenderCertificate {
	c := &SenderCertificate{raw: raw}
	runtime.SetFinalizer(c, func(c *SenderCertificate) {
		_ = checkError(C.signal_sender_certificate_destroy(c.raw))
	})
	return c
}

// SenderCert extracts the sender certificate from USMC.
func (u *UnidentifiedSenderMessageContent) SenderCert() (*SenderCertificate, error) {
	var out C.SignalMutPointerSenderCertificate
	if err := checkError(C.signal_unidentified_sender_message_content_get_sender_cert(&out, u.constPtr())); err != nil {
		return nil, err
	}
	return wrapSenderCertificate(out), nil
}

// SenderUUID returns the sender's ACI/PNI string.
func (c *SenderCertificate) SenderUUID() (string, error) {
	var cstr *C.char
	if err := checkError(C.signal_sender_certificate_get_sender_uuid(&cstr, c.constPtr())); err != nil {
		return "", err
	}
	s := C.GoString(cstr)
	C.signal_free_string(cstr)
	return s, nil
}

// SenderDeviceID returns the sender device number.
func (c *SenderCertificate) SenderDeviceID() (uint32, error) {
	var out C.uint32_t
	if err := checkError(C.signal_sender_certificate_get_device_id(&out, c.constPtr())); err != nil {
		return 0, err
	}
	return uint32(out), nil
}

// Validate checks the sender certificate against trustRoots at validationTime.
func (c *SenderCertificate) Validate(trustRoots []*PublicKey, validationTime time.Time) (bool, error) {
	if len(trustRoots) == 0 {
		return false, errors.New("libsignal.SenderCertificate.Validate: no trust roots")
	}
	ptrs := make([]C.SignalConstPointerPublicKey, len(trustRoots))
	for i, pk := range trustRoots {
		ptrs[i] = pk.constPtr()
	}
	slice := C.SignalBorrowedSliceOfConstPointerPublicKey{
		base:   (*C.SignalConstPointerPublicKey)(unsafe.Pointer(&ptrs[0])),
		length: C.size_t(len(ptrs)),
	}
	var ok C.bool
	ms := uint64(validationTime.UnixMilli())
	if err := checkError(C.signal_sender_certificate_validate(&ok, c.constPtr(), slice, C.uint64_t(ms))); err != nil {
		return false, err
	}
	return bool(ok), nil
}
