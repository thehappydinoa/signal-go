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

// NewUSMC wraps a per-device Double Ratchet ciphertext in a sealed-sender
// UnidentifiedSenderMessageContent. The result can be serialized and used
// as the envelope payload in an UNIDENTIFIED_SENDER PUT.
//
// For 1:1 messages the groupID must be nil. content_hint is set to 0
// (DEFAULT — the server stores and re-delivers the message if the recipient
// is offline).
func NewUSMC(msg *CiphertextMessage, cert *SenderCertificate) (*UnidentifiedSenderMessageContent, error) {
	return newUSMC(msg, cert, nil)
}

// NewUSMCForGroup wraps a sender-key ciphertext for multi-recipient group
// delivery. groupMasterKey is the 32-byte Groups v2 master key.
func NewUSMCForGroup(msg *CiphertextMessage, cert *SenderCertificate, groupMasterKey []byte) (*UnidentifiedSenderMessageContent, error) {
	if len(groupMasterKey) == 0 {
		return nil, errors.New("libsignal.NewUSMCForGroup: empty group master key")
	}
	return newUSMC(msg, cert, groupMasterKey)
}

func newUSMC(msg *CiphertextMessage, cert *SenderCertificate, groupMasterKey []byte) (*UnidentifiedSenderMessageContent, error) {
	if msg == nil {
		return nil, errors.New("libsignal.NewUSMC: nil ciphertext message")
	}
	if cert == nil {
		return nil, errors.New("libsignal.NewUSMC: nil sender certificate")
	}
	var groupBuf C.SignalBorrowedBuffer
	if len(groupMasterKey) > 0 {
		groupBuf = borrowed(groupMasterKey)
		keepAlive(groupMasterKey)
	}
	var out C.SignalMutPointerUnidentifiedSenderMessageContent
	err := checkError(C.signal_unidentified_sender_message_content_new(
		&out,
		msg.constPtr(),
		cert.constPtr(),
		0, // content_hint: DEFAULT
		groupBuf,
	))
	runtime.KeepAlive(cert) // prevent finalizer from destroying cert during FFI
	if err != nil {
		return nil, err
	}
	return wrapUSMC(out), nil
}

// MultiRecipientMessageForSingleRecipient extracts the per-device portion
// of a multi-recipient sealed-sender payload. When given a single-recipient
// payload, libsignal returns an error and callers should decrypt the input
// directly.
func MultiRecipientMessageForSingleRecipient(encoded []byte) ([]byte, error) {
	if len(encoded) == 0 {
		return nil, errors.New("libsignal.MultiRecipientMessageForSingleRecipient: empty input")
	}
	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_sealed_sender_multi_recipient_message_for_single_recipient(
		&buf,
		borrowed(encoded),
	))
	keepAlive(encoded)
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// MultiRecipientEncryptParams holds inputs for sealed-sender v2 fan-out.
type MultiRecipientEncryptParams struct {
	Recipients        []*Address
	RecipientSessions []*SessionRecord
	ExcludedServiceID []byte // optional; empty for normal group sends
	Content           *UnidentifiedSenderMessageContent
	Stores            *StoreHandle
}

// MultiRecipientEncrypt builds a SealedSenderMultiRecipientMessage wire blob
// suitable for PUT /v1/messages/multi_recipient.
func MultiRecipientEncrypt(p MultiRecipientEncryptParams) ([]byte, error) {
	if len(p.Recipients) == 0 {
		return nil, errors.New("libsignal.MultiRecipientEncrypt: no recipients")
	}
	if len(p.Recipients) != len(p.RecipientSessions) {
		return nil, errors.New("libsignal.MultiRecipientEncrypt: recipients/sessions length mismatch")
	}
	if p.Content == nil {
		return nil, errors.New("libsignal.MultiRecipientEncrypt: nil content")
	}
	if p.Stores == nil {
		return nil, errors.New("libsignal.MultiRecipientEncrypt: nil store handle")
	}

	addrPtrs := make([]C.SignalConstPointerProtocolAddress, len(p.Recipients))
	sessPtrs := make([]C.SignalConstPointerSessionRecord, len(p.RecipientSessions))
	var pinner runtime.Pinner
	defer pinner.Unpin()
	for i, addr := range p.Recipients {
		pinner.Pin(addr)
		addrPtrs[i] = addr.constPtr()
	}
	for i, sess := range p.RecipientSessions {
		pinner.Pin(sess)
		sessPtrs[i] = sess.constPtr()
	}
	p.Stores.pinForFFI(&pinner)
	pinner.Pin(p.Content)

	recipientSlice := C.SignalBorrowedSliceOfConstPointerProtocolAddress{
		base:   (*C.SignalConstPointerProtocolAddress)(unsafe.Pointer(&addrPtrs[0])),
		length: C.size_t(len(addrPtrs)),
	}
	sessionSlice := C.SignalBorrowedSliceOfConstPointerSessionRecord{
		base:   (*C.SignalConstPointerSessionRecord)(unsafe.Pointer(&sessPtrs[0])),
		length: C.size_t(len(sessPtrs)),
	}

	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_sealed_sender_multi_recipient_encrypt(
		&buf,
		recipientSlice,
		sessionSlice,
		borrowed(p.ExcludedServiceID),
		p.Content.constPtr(),
		p.Stores.IdentityKeyStoreStruct(),
	))
	keepAlive(p.ExcludedServiceID)
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// Serialize returns the wire encoding of the USMC. This is the bytes placed
// into a sealed-sender MessageEnvelope's Content field.
func (u *UnidentifiedSenderMessageContent) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_unidentified_sender_message_content_serialize(&buf, u.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
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

// DeserializeSenderCertificate parses the wire encoding returned by
// GET /v1/certificate/delivery.
func DeserializeSenderCertificate(data []byte) (*SenderCertificate, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeSenderCertificate: empty input")
	}
	var out C.SignalMutPointerSenderCertificate
	err := checkError(C.signal_sender_certificate_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapSenderCertificate(out), nil
}

// SenderUUID returns the sender's ACI/PNI string.
func (c *SenderCertificate) SenderUUID() (string, error) {
	var cstr C.SignalCStringPtr
	if err := checkError(C.signal_sender_certificate_get_sender_uuid(&cstr, c.constPtr())); err != nil {
		return "", err
	}
	s := C.GoString((*C.char)(cstr))
	C.signal_free_string((*C.char)(cstr))
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

// Expiration returns when this certificate expires (milliseconds since epoch,
// converted to a [time.Time]).
func (c *SenderCertificate) Expiration() (time.Time, error) {
	var ms C.uint64_t
	if err := checkError(C.signal_sender_certificate_get_expiration(&ms, c.constPtr())); err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(int64(ms)), nil
}

// Serialize returns the canonical wire encoding of the certificate.
func (c *SenderCertificate) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_sender_certificate_get_serialized(&buf, c.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
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
