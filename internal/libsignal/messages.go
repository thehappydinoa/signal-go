package libsignal

/*
#include "signal_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"runtime"
)

// SignalMessage is an encrypted Double-Ratchet payload.
type SignalMessage struct {
	raw C.SignalMutPointerSignalMessage
}

// DeserializeSignalMessage parses the wire encoding of a SignalMessage.
func DeserializeSignalMessage(data []byte) (*SignalMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializeSignalMessage: empty input")
	}
	var out C.SignalMutPointerSignalMessage
	err := checkError(C.signal_message_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapSignalMessage(out), nil
}

func (m *SignalMessage) constPtr() C.SignalConstPointerSignalMessage {
	return C.SignalConstPointerSignalMessage{raw: m.raw.raw}
}

func wrapSignalMessage(raw C.SignalMutPointerSignalMessage) *SignalMessage {
	m := &SignalMessage{raw: raw}
	runtime.SetFinalizer(m, func(m *SignalMessage) {
		_ = checkError(C.signal_message_destroy(m.raw))
	})
	return m
}

// PreKeySignalMessage is the first message in an X3DH/PQXDH session.
type PreKeySignalMessage struct {
	raw C.SignalMutPointerPreKeySignalMessage
}

// DeserializePreKeySignalMessage parses the wire encoding.
func DeserializePreKeySignalMessage(data []byte) (*PreKeySignalMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializePreKeySignalMessage: empty input")
	}
	var out C.SignalMutPointerPreKeySignalMessage
	err := checkError(C.signal_pre_key_signal_message_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapPreKeySignalMessage(out), nil
}

func (m *PreKeySignalMessage) constPtr() C.SignalConstPointerPreKeySignalMessage {
	return C.SignalConstPointerPreKeySignalMessage{raw: m.raw.raw}
}

func wrapPreKeySignalMessage(raw C.SignalMutPointerPreKeySignalMessage) *PreKeySignalMessage {
	m := &PreKeySignalMessage{raw: raw}
	runtime.SetFinalizer(m, func(m *PreKeySignalMessage) {
		_ = checkError(C.signal_pre_key_signal_message_destroy(m.raw))
	})
	return m
}

// PlaintextContent wraps a plaintext payload (e.g. decryption-error receipts).
type PlaintextContent struct {
	raw C.SignalMutPointerPlaintextContent
}

// DeserializePlaintextContent parses the wire encoding.
func DeserializePlaintextContent(data []byte) (*PlaintextContent, error) {
	if len(data) == 0 {
		return nil, errors.New("libsignal.DeserializePlaintextContent: empty input")
	}
	var out C.SignalMutPointerPlaintextContent
	err := checkError(C.signal_plaintext_content_deserialize(&out, borrowed(data)))
	keepAlive(data)
	if err != nil {
		return nil, err
	}
	return wrapPlaintextContent(out), nil
}

// Body returns the inner plaintext bytes.
func (p *PlaintextContent) Body() ([]byte, error) {
	var buf C.SignalOwnedBuffer
	if err := checkError(C.signal_plaintext_content_get_body(&buf, p.constPtr())); err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

func (p *PlaintextContent) constPtr() C.SignalConstPointerPlaintextContent {
	return C.SignalConstPointerPlaintextContent{raw: p.raw.raw}
}

func wrapPlaintextContent(raw C.SignalMutPointerPlaintextContent) *PlaintextContent {
	p := &PlaintextContent{raw: raw}
	runtime.SetFinalizer(p, func(p *PlaintextContent) {
		_ = checkError(C.signal_plaintext_content_destroy(p.raw))
	})
	return p
}

// CiphertextMessageType mirrors libsignal's message-type enum.
type CiphertextMessageType uint8

const (
	CiphertextWhisper   CiphertextMessageType = C.SignalCiphertextMessageTypeWhisper
	CiphertextPreKey    CiphertextMessageType = C.SignalCiphertextMessageTypePreKey
	CiphertextSenderKey CiphertextMessageType = C.SignalCiphertextMessageTypeSenderKey
	CiphertextPlaintext CiphertextMessageType = C.SignalCiphertextMessageTypePlaintext
)
