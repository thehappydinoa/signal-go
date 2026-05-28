package group

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

// EncryptDescriptionBlob encrypts a group description for the wire format.
func EncryptDescriptionBlob(secretParams [libsignal.GroupSecretParamsLen]byte, description string, randomness [libsignal.ZKRandomnessLen]byte) ([]byte, error) {
	if description == "" {
		return nil, nil
	}
	blob := &groupspb.GroupAttributeBlob{
		Content: &groupspb.GroupAttributeBlob_DescriptionText{DescriptionText: description},
	}
	plain, err := proto.Marshal(blob)
	if err != nil {
		return nil, fmt.Errorf("group.EncryptDescriptionBlob: marshal: %w", err)
	}
	return libsignal.GroupSecretParamsEncryptBlob(secretParams, plain, randomness)
}

// EncryptTimerBlob encrypts a disappearing-messages timer duration in seconds.
func EncryptTimerBlob(secretParams [libsignal.GroupSecretParamsLen]byte, seconds int, randomness [libsignal.ZKRandomnessLen]byte) ([]byte, error) {
	if seconds < 0 {
		return nil, fmt.Errorf("group.EncryptTimerBlob: negative duration %d", seconds)
	}
	blob := &groupspb.GroupAttributeBlob{
		Content: &groupspb.GroupAttributeBlob_DisappearingMessagesDuration{
			DisappearingMessagesDuration: uint32(seconds),
		},
	}
	plain, err := proto.Marshal(blob)
	if err != nil {
		return nil, fmt.Errorf("group.EncryptTimerBlob: marshal: %w", err)
	}
	return libsignal.GroupSecretParamsEncryptBlob(secretParams, plain, randomness)
}
