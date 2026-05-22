package libsignal

import (
	"errors"
	"fmt"
	"time"
)

func decryptUSMCInner(
	h *StoreHandle,
	usmc *UnidentifiedSenderMessageContent,
	senderAddr, localAddr *Address,
) ([]byte, bool, error) {
	inner, err := usmc.Contents()
	if err != nil {
		return nil, false, err
	}
	msgType, err := usmc.MessageType()
	if err != nil {
		return nil, false, err
	}

	switch msgType {
	case CiphertextWhisper:
		sm, err := DeserializeSignalMessage(inner)
		if err != nil {
			return nil, false, err
		}
		pt, err := DecryptSignalMessage(sm, senderAddr, localAddr, h)
		return pt, false, err
	case CiphertextPreKey:
		pkm, err := DeserializePreKeySignalMessage(inner)
		if err != nil {
			return nil, false, err
		}
		pt, err := DecryptPreKeySignalMessage(pkm, senderAddr, localAddr, h)
		return pt, true, err
	case CiphertextPlaintext:
		pc, err := DeserializePlaintextContent(inner)
		if err != nil {
			return nil, false, err
		}
		pt, err := DecryptPlaintextContent(pc)
		return pt, false, err
	case CiphertextSenderKey:
		pt, err := GroupDecryptMessage(inner, senderAddr, h)
		return pt, false, err
	default:
		return nil, false, fmt.Errorf("libsignal.DecryptSealedSender: unsupported inner type %d", msgType)
	}
}

func validateSenderCert(usmc *UnidentifiedSenderMessageContent, roots []*PublicKey, when time.Time) (string, uint32, error) {
	cert, err := usmc.SenderCert()
	if err != nil {
		return "", 0, fmt.Errorf("libsignal.DecryptSealedSender: sender cert: %w", err)
	}
	ok, err := cert.Validate(roots, when)
	if err != nil {
		return "", 0, fmt.Errorf("libsignal.DecryptSealedSender: validate cert: %w", err)
	}
	if !ok {
		return "", 0, errors.New("libsignal.DecryptSealedSender: sender certificate not trusted")
	}
	senderUUID, err := cert.SenderUUID()
	if err != nil {
		return "", 0, err
	}
	senderDevice, err := cert.SenderDeviceID()
	if err != nil {
		return "", 0, err
	}
	return senderUUID, senderDevice, nil
}
