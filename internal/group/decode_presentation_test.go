package group

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

func TestDecodeStatePresentationMember(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	for i := range master {
		master[i] = byte(i + 10)
	}
	secret, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}

	profileKey := make([]byte, libsignal.ProfileKeyLen)
	for i := range profileKey {
		profileKey[i] = byte(i + 11)
	}
	const memberACI = "64656667-6869-6a6b-6c6d-6e6f70717273"
	presentation, err := libsignal.TestingProfileKeyPresentationRoundTrip(memberACI, profileKey, master)
	if err != nil {
		t.Fatal(err)
	}

	wire := &groupspb.Group{
		Version: 1,
		Members: []*groupspb.Member{
			{
				Presentation: presentation,
				Role:         groupspb.Member_DEFAULT,
			},
		},
	}

	state, err := DecodeState(secret, wire)
	if err != nil {
		t.Fatalf("DecodeState: %v", err)
	}
	if len(state.Members) != 1 {
		t.Fatalf("members = %d", len(state.Members))
	}
	if state.Members[0].ACI != memberACI {
		t.Fatalf("aci = %q", state.Members[0].ACI)
	}
	if len(state.Members[0].ProfileKey) != libsignal.ProfileKeyLen {
		t.Fatalf("profile key length = %d", len(state.Members[0].ProfileKey))
	}
}
