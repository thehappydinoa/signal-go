package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"runtime"
)

// SenderKeyDistributionMessage distributes a sender-key chain to group
// members so they can decrypt subsequent group ciphertexts.
type SenderKeyDistributionMessage struct {
	raw C.SignalMutPointerSenderKeyDistributionMessage
}

func (m *SenderKeyDistributionMessage) constPtr() C.SignalConstPointerSenderKeyDistributionMessage {
	return C.SignalConstPointerSenderKeyDistributionMessage{raw: m.raw.raw}
}

func wrapSenderKeyDistributionMessage(raw C.SignalMutPointerSenderKeyDistributionMessage) *SenderKeyDistributionMessage {
	m := &SenderKeyDistributionMessage{raw: raw}
	runtime.SetFinalizer(m, func(m *SenderKeyDistributionMessage) {
		_ = checkError(C.signal_sender_key_distribution_message_destroy(m.raw))
	})
	return m
}

// CreateSenderKeyDistributionMessage builds a new SKDM for the given
// distribution UUID. The sender address is the local device that will
// encrypt group messages with this chain.
func CreateSenderKeyDistributionMessage(
	sender *Address,
	distributionID string,
	h *StoreHandle,
) (*SenderKeyDistributionMessage, error) {
	uuid, err := ParseUUIDString(distributionID)
	if err != nil {
		return nil, err
	}
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(sender)
	h.pinForFFI(&pinner)
	var out C.SignalMutPointerSenderKeyDistributionMessage
	err = checkError(C.signal_sender_key_distribution_message_create(
		&out,
		sender.constPtr(),
		uuidToC(uuid),
		h.SenderKeyStoreStruct(),
	))
	if err != nil {
		return nil, err
	}
	return wrapSenderKeyDistributionMessage(out), nil
}

// DeserializeSenderKeyDistributionMessage parses SKDM wire bytes.
func DeserializeSenderKeyDistributionMessage(data []byte) (*SenderKeyDistributionMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeSenderKeyDistributionMessage: empty input")
	}
	var out C.SignalMutPointerSenderKeyDistributionMessage
	err := checkError(C.signal_sender_key_distribution_message_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapSenderKeyDistributionMessage(out), nil
}

// Serialize returns the wire encoding of the SKDM.
func (m *SenderKeyDistributionMessage) Serialize() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_sender_key_distribution_message_serialize(&buf, m.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// ProcessSenderKeyDistributionMessage stores the sender's key material
// from an inbound SKDM.
func ProcessSenderKeyDistributionMessage(
	sender *Address,
	msg *SenderKeyDistributionMessage,
	h *StoreHandle,
) error {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(sender)
	pinner.Pin(msg)
	h.pinForFFI(&pinner)
	return checkError(C.signal_process_sender_key_distribution_message(
		sender.constPtr(),
		msg.constPtr(),
		h.SenderKeyStoreStruct(),
	))
}

// GroupEncryptMessage encrypts plaintext with the local sender-key chain
// identified by distributionID.
func GroupEncryptMessage(
	ptext []byte,
	sender *Address,
	distributionID string,
	h *StoreHandle,
) (*CiphertextMessage, error) {
	uuid, err := ParseUUIDString(distributionID)
	if err != nil {
		return nil, err
	}
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(sender)
	h.pinForFFI(&pinner)
	var out C.SignalMutPointerCiphertextMessage
	err = checkError(C.signal_group_encrypt_message(
		&out,
		sender.constPtr(),
		uuidToC(uuid),
		borrowed(ptext),
		h.SenderKeyStoreStruct(),
	))
	keepAlive(ptext)
	if err != nil {
		return nil, err
	}
	return wrapCiphertextMessage(out), nil
}

// GroupDecryptMessage decrypts a serialized SenderKeyMessage from sender.
func GroupDecryptMessage(
	ciphertext []byte,
	sender *Address,
	h *StoreHandle,
) ([]byte, error) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(sender)
	h.pinForFFI(&pinner)
	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_group_decrypt_message(
		&buf,
		sender.constPtr(),
		borrowed(ciphertext),
		h.SenderKeyStoreStruct(),
	))
	keepAlive(ciphertext)
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}
