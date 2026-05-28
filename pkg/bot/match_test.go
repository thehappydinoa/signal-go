package bot

import (
	"testing"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// TestInGroupsDMBypass documents and enforces the security contract:
// InGroups alone does not block DMs; .Group().InGroups(...) does.
func TestInGroupsDMBypass(t *testing.T) {
	const allowed = "aabbcc"
	const other = "ddeeff"

	m := matcher{
		kind:            matchAnyText,
		allowedGroupIDs: map[string]struct{}{allowed: {}},
	}

	cases := []struct {
		name    string
		groupID string
		want    bool
	}{
		{"DM passes (empty GroupID)", "", true},
		{"listed group passes", allowed, true},
		{"unlisted group blocked", other, false},
	}
	for _, tc := range cases {
		ev := &signal.MessageEvent{Sender: "alice", GroupID: tc.groupID, Body: "hi"}
		msg := &Message{event: ev}
		_, got := m.match(ev, msg)
		if got != tc.want {
			t.Errorf("InGroups(%q) with GroupID=%q: got %v, want %v", allowed, tc.groupID, got, tc.want)
		}
	}
}

// TestInGroupsWithGroupFilter verifies that .Group().InGroups(...) blocks DMs.
func TestInGroupsWithGroupFilter(t *testing.T) {
	const allowed = "aabbcc"

	m := matcher{
		kind:            matchAnyText,
		groupOnly:       true,
		allowedGroupIDs: map[string]struct{}{allowed: {}},
	}

	cases := []struct {
		name    string
		groupID string
		want    bool
	}{
		{"DM blocked by groupOnly", "", false},
		{"listed group passes", allowed, true},
		{"unlisted group blocked", "ddeeff", false},
	}
	for _, tc := range cases {
		ev := &signal.MessageEvent{Sender: "alice", GroupID: tc.groupID, Body: "hi"}
		msg := &Message{event: ev}
		_, got := m.match(ev, msg)
		if got != tc.want {
			t.Errorf("Group().InGroups(%q) with GroupID=%q: got %v, want %v", allowed, tc.groupID, got, tc.want)
		}
	}
}

// TestReactionInGroupsDMBypass mirrors the message test for ReactionMatch.
func TestReactionInGroupsDMBypass(t *testing.T) {
	const allowed = "aabbcc"

	m := reactionMatcher{
		allowedGroupIDs: map[string]struct{}{allowed: {}},
	}

	cases := []struct {
		name    string
		groupID string
		want    bool
	}{
		{"DM reaction passes", "", true},
		{"listed group passes", allowed, true},
		{"unlisted group blocked", "ddeeff", false},
	}
	for _, tc := range cases {
		ev := &signal.ReactionEvent{Sender: "alice", GroupID: tc.groupID, Emoji: "👍"}
		got := m.match(ev)
		if got != tc.want {
			t.Errorf("ReactionMatch.InGroups(%q) with GroupID=%q: got %v, want %v", allowed, tc.groupID, got, tc.want)
		}
	}
}

// TestEditInGroupsDMBypass mirrors the message test for EditMatch.
func TestEditInGroupsDMBypass(t *testing.T) {
	const allowed = "aabbcc"

	m := editMatcher{
		allowedGroupIDs: map[string]struct{}{allowed: {}},
	}

	cases := []struct {
		name    string
		groupID string
		want    bool
	}{
		{"DM edit passes", "", true},
		{"listed group passes", allowed, true},
		{"unlisted group blocked", "ddeeff", false},
	}
	for _, tc := range cases {
		ev := &signal.EditMessageEvent{Sender: "alice", GroupID: tc.groupID}
		got := m.match(ev)
		if got != tc.want {
			t.Errorf("EditMatch.InGroups(%q) with GroupID=%q: got %v, want %v", allowed, tc.groupID, got, tc.want)
		}
	}
}
