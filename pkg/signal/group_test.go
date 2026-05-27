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

func TestNormalizePNIServiceID(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "trim and prefix bare uuid", in: " 11111111-2222-3333-4444-555555555555 ", want: "PNI:11111111-2222-3333-4444-555555555555"},
		{name: "already upper prefixed", in: "PNI:11111111-2222-3333-4444-555555555555", want: "PNI:11111111-2222-3333-4444-555555555555"},
		{name: "lower prefixed normalized", in: "pni:11111111-2222-3333-4444-555555555555", want: "PNI:11111111-2222-3333-4444-555555555555"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizePNIServiceID(tc.in); got != tc.want {
				t.Fatalf("normalizePNIServiceID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
