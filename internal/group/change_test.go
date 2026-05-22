package group

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

func TestBuildLeaveActions(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	for i := range master {
		master[i] = byte(i)
	}
	secret, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}
	const aci = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	actions, err := BuildLeaveActions(secret, aci, 3)
	if err != nil {
		t.Fatal(err)
	}
	var parsed groupspb.GroupChange_Actions
	if err := proto.Unmarshal(actions, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.GetVersion() != 4 {
		t.Fatalf("version = %d, want 4", parsed.GetVersion())
	}
	if len(parsed.GetDeleteMembers()) != 1 {
		t.Fatalf("deleteMembers = %d", len(parsed.GetDeleteMembers()))
	}
	if len(parsed.GetGroupId()) != 0 {
		t.Fatal("group_id must be empty on request")
	}
}

func TestBuildModifyRoleActions(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	secret, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}
	const actor = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	const target = "64656667-6869-6a6b-6c6d-6e6f70717273"
	actions, err := BuildModifyRoleActions(secret, actor, target, MemberRoleAdministrator, 1)
	if err != nil {
		t.Fatal(err)
	}
	var parsed groupspb.GroupChange_Actions
	if err := proto.Unmarshal(actions, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.GetVersion() != 2 {
		t.Fatalf("version = %d", parsed.GetVersion())
	}
	if len(parsed.GetModifyMemberRoles()) != 1 {
		t.Fatalf("modifyMemberRoles = %d", len(parsed.GetModifyMemberRoles()))
	}
	if parsed.GetModifyMemberRoles()[0].GetRole() != groupspb.Member_ADMINISTRATOR {
		t.Fatalf("role = %v", parsed.GetModifyMemberRoles()[0].GetRole())
	}
}
