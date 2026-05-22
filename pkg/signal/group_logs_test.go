package signal

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/group"
)

func TestGroupFromDecodedState(t *testing.T) {
	state := &group.State{
		Title:       "hello",
		Description: "desc",
		AvatarURL:   "cdn://avatar",
		Revision:    7,
		Members: []group.Member{
			{ACI: "00000000-0000-4000-8000-000000000001", Role: group.MemberRoleAdministrator},
		},
	}
	grp, acis, err := groupFromDecodedState("abc123", state)
	if err != nil {
		t.Fatal(err)
	}
	if grp.ID != "abc123" || grp.Revision != 7 || grp.Title != "hello" {
		t.Fatalf("grp = %+v", grp)
	}
	if len(acis) != 1 || acis[0] != state.Members[0].ACI {
		t.Fatalf("acis = %v", acis)
	}
	if !grp.IsAdmin(state.Members[0].ACI) {
		t.Fatal("expected admin")
	}
}
