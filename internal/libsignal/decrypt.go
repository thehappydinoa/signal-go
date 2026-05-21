package libsignal

/*
#include "signal_ffi.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/thehappydinoa/signal-go/internal/store"
)

// DecryptParams groups everything needed for one inbound decrypt.
type DecryptParams struct {
	Stores         store.SignalStores
	LocalServiceID string
	LocalDeviceID  uint32
	LocalE164      string // optional; used for self-send detection
	TrustRoots     []*PublicKey
	ValidationTime time.Time
}

// DecryptResult is the outcome of decrypting one inner ciphertext.
type DecryptResult struct {
	Plaintext    []byte
	SenderUUID   string
	SenderDevice uint32
}

// DecryptSignalMessage decrypts a normal Double-Ratchet message.
func DecryptSignalMessage(
	msg *SignalMessage,
	sender *Address,
	local *Address,
	h *StoreHandle,
) ([]byte, error) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(msg)
	pinner.Pin(sender)
	pinner.Pin(local)
	h.pinForFFI(&pinner)
	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_decrypt_message(
		&buf,
		msg.constPtr(),
		sender.constPtr(),
		local.constPtr(),
		h.SessionStoreStruct(),
		h.IdentityKeyStoreStruct(),
	))
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// DecryptPreKeySignalMessage decrypts an X3DH/PQXDH prekey message.
func DecryptPreKeySignalMessage(
	msg *PreKeySignalMessage,
	sender *Address,
	local *Address,
	h *StoreHandle,
) ([]byte, error) {
	var pinner runtime.Pinner
	defer pinner.Unpin()
	pinner.Pin(msg)
	pinner.Pin(sender)
	pinner.Pin(local)
	h.pinForFFI(&pinner)
	var buf C.SignalOwnedBuffer
	err := checkError(C.signal_decrypt_pre_key_message(
		&buf,
		msg.constPtr(),
		sender.constPtr(),
		local.constPtr(),
		h.SessionStoreStruct(),
		h.IdentityKeyStoreStruct(),
		h.PreKeyStoreStruct(),
		h.SignedPreKeyStoreStruct(),
		h.KyberPreKeyStoreStruct(),
	))
	if err != nil {
		return nil, err
	}
	return goBytesFromOwnedBuffer(buf), nil
}

// DecryptPlaintextContent returns the body of a plaintext content message.
func DecryptPlaintextContent(msg *PlaintextContent) ([]byte, error) {
	return msg.Body()
}

// DecryptSealedSender follows libsignal's sealed_sender_decrypt path:
// unwrap → validate sender cert → decrypt inner whisper/prekey payload.
func DecryptSealedSender(ctext []byte, p DecryptParams) (*DecryptResult, error) {
	if p.Stores == nil {
		return nil, errors.New("libsignal.DecryptSealedSender: nil stores")
	}
	if p.LocalServiceID == "" {
		return nil, errors.New("libsignal.DecryptSealedSender: empty local service id")
	}
	roots := p.TrustRoots
	if len(roots) == 0 {
		var err error
		roots, err = ProductionTrustRoots()
		if err != nil {
			return nil, err
		}
	}
	when := p.ValidationTime
	if when.IsZero() {
		when = time.Now()
	}

	h := NewStoreHandle(p.Stores)
	defer h.Release()

	usmc, err := DecryptSealedSenderToUSMC(ctext, h)
	if err != nil {
		return nil, fmt.Errorf("libsignal.DecryptSealedSender: unwrap: %w", err)
	}

	cert, err := usmc.SenderCert()
	if err != nil {
		return nil, fmt.Errorf("libsignal.DecryptSealedSender: sender cert: %w", err)
	}
	ok, err := cert.Validate(roots, when)
	if err != nil {
		return nil, fmt.Errorf("libsignal.DecryptSealedSender: validate cert: %w", err)
	}
	if !ok {
		return nil, errors.New("libsignal.DecryptSealedSender: sender certificate not trusted")
	}

	senderUUID, err := cert.SenderUUID()
	if err != nil {
		return nil, err
	}
	senderDevice, err := cert.SenderDeviceID()
	if err != nil {
		return nil, err
	}

	if senderUUID == p.LocalServiceID && senderDevice == p.LocalDeviceID {
		return nil, &Error{Code: ErrorCode(C.SignalErrorCodeSealedSenderSelfSend), Message: "sealed sender self-send"}
	}

	inner, err := usmc.Contents()
	if err != nil {
		return nil, err
	}
	msgType, err := usmc.MessageType()
	if err != nil {
		return nil, err
	}

	senderAddr, err := NewAddress(senderUUID, senderDevice)
	if err != nil {
		return nil, err
	}
	localAddr, err := NewAddress(p.LocalServiceID, p.LocalDeviceID)
	if err != nil {
		return nil, err
	}

	var plaintext []byte
	switch msgType {
	case CiphertextWhisper:
		sm, err := DeserializeSignalMessage(inner)
		if err != nil {
			return nil, err
		}
		plaintext, err = DecryptSignalMessage(sm, senderAddr, localAddr, h)
	case CiphertextPreKey:
		pkm, err := DeserializePreKeySignalMessage(inner)
		if err != nil {
			return nil, err
		}
		plaintext, err = DecryptPreKeySignalMessage(pkm, senderAddr, localAddr, h)
	case CiphertextPlaintext:
		pc, err := DeserializePlaintextContent(inner)
		if err != nil {
			return nil, err
		}
		plaintext, err = DecryptPlaintextContent(pc)
	default:
		return nil, fmt.Errorf("libsignal.DecryptSealedSender: unsupported inner type %d", msgType)
	}
	if err != nil {
		return nil, fmt.Errorf("libsignal.DecryptSealedSender: inner decrypt: %w", err)
	}

	return &DecryptResult{
		Plaintext:    plaintext,
		SenderUUID:   senderUUID,
		SenderDevice: senderDevice,
	}, nil
}

// StripVersionByte removes the leading version byte from envelope content.
// Signal prefixes DOUBLE_RATCHET and PREKEY_MESSAGE payloads with one
// version byte before the serialized ciphertext.
func StripVersionByte(content []byte) ([]byte, bool) {
	if len(content) <= 1 {
		return content, false
	}
	return content[1:], true
}
