package group

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

func TestBuildNewGroupMessage(t *testing.T) {
	masterKey, secret, err := libsignal.GenerateGroupMasterKey()
	if err != nil {
		t.Fatalf("GenerateGroupMasterKey: %v", err)
	}
	_ = masterKey
	public, err := libsignal.GroupSecretParamsPublicParams(secret)
	if err != nil {
		t.Fatal(err)
	}

	wire, err := BuildNewGroupMessage(NewGroupMessageParams{
		SecretParams:     secret,
		PublicParams:     public,
		Title:            "Hello",
		Description:      "Desc",
		SelfPresentation: []byte("self-presentation"),
		Members:          []NewGroupMember{{Presentation: []byte("member-presentation"), Role: MemberRoleDefault}},
		PendingMembers:   []NewGroupPendingMember{{TargetACI: "64656667-6869-6a6b-6c6d-6e6f70717273", Role: MemberRoleDefault}},
	})
	if err != nil {
		t.Fatalf("BuildNewGroupMessage: %v", err)
	}
	var parsed groupspb.Group
	if err := proto.Unmarshal(wire, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.GetVersion() != 0 {
		t.Fatalf("version = %d", parsed.GetVersion())
	}
	if string(parsed.GetPublicKey()) != string(public[:]) {
		t.Fatal("public key mismatch")
	}
	if len(parsed.GetMembers()) != 2 {
		t.Fatalf("members = %d", len(parsed.GetMembers()))
	}
	if parsed.GetMembers()[0].GetRole() != groupspb.Member_ADMINISTRATOR {
		t.Fatalf("self role = %v", parsed.GetMembers()[0].GetRole())
	}
	if len(parsed.GetMembersPendingProfileKey()) != 1 {
		t.Fatalf("pending = %d", len(parsed.GetMembersPendingProfileKey()))
	}
	if parsed.GetAccessControl().GetAttributes() != groupspb.AccessControl_MEMBER {
		t.Fatalf("attributes access = %v", parsed.GetAccessControl().GetAttributes())
	}
}

func TestFormatInviteLinkURLRoundTrip(t *testing.T) {
	master, _, err := libsignal.GenerateGroupMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	password, err := GenerateInviteLinkPassword()
	if err != nil {
		t.Fatal(err)
	}
	url, err := FormatInviteLinkURL(master, password)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseInviteLinkURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if string(parsed.MasterKey) != string(master) {
		t.Fatal("master key mismatch")
	}
	if string(parsed.InviteLinkPassword) != string(password) {
		t.Fatal("password mismatch")
	}
}
