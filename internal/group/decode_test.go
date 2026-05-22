package group

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

func TestDecodeState(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	for i := range master {
		master[i] = byte(i + 10)
	}
	secret, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatalf("derive secret: %v", err)
	}

	var randomness [libsignal.ZKRandomnessLen]byte
	for i := range randomness {
		randomness[i] = byte(i + 20)
	}

	titleCT, err := EncryptTitleBlob(secret, "Test Group", randomness)
	if err != nil {
		t.Fatalf("encrypt title: %v", err)
	}

	const adminACI = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	const memberACI = "64656667-6869-6a6b-6c6d-6e6f70717273"
	adminCT, err := EncryptServiceID(secret, adminACI)
	if err != nil {
		t.Fatalf("encrypt admin: %v", err)
	}
	memberCT, err := EncryptServiceID(secret, memberACI)
	if err != nil {
		t.Fatalf("encrypt member: %v", err)
	}

	wire := &groupspb.Group{
		Title:   titleCT,
		Version: 3,
		Members: []*groupspb.Member{
			{UserId: adminCT, Role: groupspb.Member_ADMINISTRATOR},
			{UserId: memberCT, Role: groupspb.Member_DEFAULT},
		},
	}

	state, err := DecodeState(secret, wire)
	if err != nil {
		t.Fatalf("DecodeState: %v", err)
	}
	if state.Title != "Test Group" {
		t.Fatalf("title = %q", state.Title)
	}
	if state.Revision != 3 {
		t.Fatalf("revision = %d", state.Revision)
	}
	if len(state.Members) != 2 {
		t.Fatalf("members = %d", len(state.Members))
	}
	if !state.IsAdmin(adminACI) {
		t.Fatal("expected admin")
	}
	if state.IsAdmin(memberACI) {
		t.Fatal("member should not be admin")
	}
	admins := state.Admins()
	if len(admins) != 1 || admins[0] != adminACI {
		t.Fatalf("admins = %v", admins)
	}
}
