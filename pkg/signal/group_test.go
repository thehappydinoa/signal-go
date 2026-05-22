package signal

import "testing"

func TestGroupIsAdmin(t *testing.T) {
	g := &Group{
		Members: []GroupMember{
			{ACI: "aaa", Role: GroupRoleAdministrator},
			{ACI: "bbb", Role: GroupRoleDefault},
		},
	}
	if !g.IsAdmin("aaa") || g.IsAdmin("bbb") {
		t.Fatal("IsAdmin mismatch")
	}
	if admins := g.Admins(); len(admins) != 1 || admins[0] != "aaa" {
		t.Fatalf("admins = %v", admins)
	}
}
