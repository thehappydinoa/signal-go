package group

import (
	"encoding/base64"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

func TestParseInviteLinkURLRoundTrip(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	password := []byte("0123456789abcdef")
	for i := range master {
		master[i] = byte(i)
	}
	link := &groupspb.GroupInviteLink{
		Contents: &groupspb.GroupInviteLink_ContentsV1{
			ContentsV1: &groupspb.GroupInviteLink_GroupInviteLinkContentsV1{
				GroupMasterKey:     master,
				InviteLinkPassword: password,
			},
		},
	}
	raw, err := proto.Marshal(link)
	if err != nil {
		t.Fatal(err)
	}
	url := "https://signal.group/#" + base64.RawURLEncoding.EncodeToString(raw)

	got, err := ParseInviteLinkURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if string(got.MasterKey) != string(master) {
		t.Fatalf("master key mismatch")
	}
	if string(got.InviteLinkPassword) != string(password) {
		t.Fatalf("password mismatch")
	}
}

func TestParseInviteLinkURLRejectsBadHost(t *testing.T) {
	if _, err := ParseInviteLinkURL("https://example.com/#abc"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildJoinViaInviteLinkActions(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := BuildJoinViaInviteLinkActions(secretParams, "00000000-0000-4000-8000-000000000001", []byte{1, 2, 3}, 4)
	if err != nil {
		t.Fatal(err)
	}
	var wire groupspb.GroupChange_Actions
	if err := proto.Unmarshal(actions, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.GetVersion() != 5 {
		t.Fatalf("version = %d", wire.GetVersion())
	}
	if len(wire.GetAddMembers()) != 1 {
		t.Fatalf("add members = %+v", wire.GetAddMembers())
	}
	if wire.GetAddMembers()[0].GetJoinFromInviteLink() {
		t.Fatalf("joinFromInviteLink unexpectedly true")
	}
	if got := wire.GetAddMembers()[0].GetAdded().GetPresentation(); string(got) != string([]byte{1, 2, 3}) {
		t.Fatalf("presentation = %v", got)
	}
	if len(wire.GetSourceUserId()) == 0 {
		t.Fatal("sourceUserId is empty")
	}
}

func TestBuildJoinRequestActions(t *testing.T) {
	master := make([]byte, libsignal.GroupMasterKeyLen)
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := BuildJoinRequestActions(secretParams, "00000000-0000-4000-8000-000000000001", []byte{4, 5, 6}, 9)
	if err != nil {
		t.Fatal(err)
	}
	var wire groupspb.GroupChange_Actions
	if err := proto.Unmarshal(actions, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.GetVersion() != 10 {
		t.Fatalf("version = %d", wire.GetVersion())
	}
	if len(wire.GetAddMembersPendingAdminApproval()) != 1 {
		t.Fatalf("pending admin actions = %+v", wire.GetAddMembersPendingAdminApproval())
	}
	added := wire.GetAddMembersPendingAdminApproval()[0].GetAdded()
	if added == nil {
		t.Fatal("pending admin added is nil")
	}
	if got := added.GetPresentation(); string(got) != string([]byte{4, 5, 6}) {
		t.Fatalf("presentation = %v", got)
	}
	if len(added.GetUserId()) != 0 {
		t.Fatalf("userId should be empty, got len=%d", len(added.GetUserId()))
	}
	if added.GetTimestamp() != 0 {
		t.Fatalf("timestamp should be zero, got %d", added.GetTimestamp())
	}
	if len(wire.GetSourceUserId()) == 0 {
		t.Fatal("sourceUserId is empty")
	}
}

func TestInviteLinkPasswordBase64(t *testing.T) {
	got := InviteLinkPasswordBase64([]byte{0xff, 0x00})
	if got != "/wA=" {
		t.Fatalf("got %q", got)
	}
}
