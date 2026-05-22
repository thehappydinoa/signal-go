package signal

import (
	"testing"
)

func TestParseGroupInviteLinkDelegates(t *testing.T) {
	_, _, err := ParseGroupInviteLink("https://example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}
