package bot

import (
	"regexp"
	"strings"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// matchKind discriminates the four built-in matcher shapes.
type matchKind uint8

const (
	matchExact matchKind = iota
	matchPrefix
	matchRegex
	matchCommand
	matchAnyText
)

// matcher describes one registered pattern. Only the relevant fields
// for the kind are populated. Scope filters (dmOnly, groupOnly, fromACI,
// stage) are evaluated before the pattern match.
type matcher struct {
	kind      matchKind
	text      string
	re        *regexp.Regexp
	dmOnly    bool
	groupOnly bool
	fromACI   string
	// allowedGroupIDs, when non-nil, restricts group messages to those
	// whose GroupID is present in the map. DM messages (empty GroupID)
	// are unaffected: they pass the filter regardless.
	allowedGroupIDs map[string]struct{}
	// stage, when non-empty, requires the conversation's current
	// stage (as written via [Convo.SetStage]) to match exactly.
	// stageAny, when true, matches any non-empty stage.
	stage    string
	stageAny bool
}

// match evaluates the matcher against an inbound message event. Scope
// filters are tested first; then the pattern match runs. Returns
// (capture-groups-or-args, true) on match, (nil, false) otherwise.
func (m matcher) match(ev *signal.MessageEvent, msg *Message) ([]string, bool) {
	if !m.scopeOK(ev, msg) {
		return nil, false
	}
	return m.bodyMatch(ev.Body)
}

// scopeOK evaluates the non-pattern scope filters: dmOnly, groupOnly,
// fromACI, allowedGroupIDs, stage, stageAny. Pulled out of [matcher.match]
// to keep cyclomatic complexity down.
func (m matcher) scopeOK(ev *signal.MessageEvent, msg *Message) bool {
	if m.dmOnly && msg.IsGroup() {
		return false
	}
	if m.groupOnly && !msg.IsGroup() {
		return false
	}
	if m.fromACI != "" && ev.Sender != m.fromACI {
		return false
	}
	if m.allowedGroupIDs != nil && ev.GroupID != "" {
		if _, ok := m.allowedGroupIDs[ev.GroupID]; !ok {
			return false
		}
	}
	if m.stageAny || m.stage != "" {
		current := msg.Convo().Stage()
		if m.stageAny && current == "" {
			return false
		}
		if m.stage != "" && current != m.stage {
			return false
		}
	}
	return true
}

// bodyMatch runs the pattern test against body and returns the captured
// arguments on match. Caller is responsible for scope filtering.
func (m matcher) bodyMatch(body string) ([]string, bool) {
	switch m.kind {
	case matchExact:
		if body == m.text {
			return nil, true
		}
	case matchPrefix:
		if strings.HasPrefix(strings.ToLower(body), strings.ToLower(m.text)) {
			return nil, true
		}
	case matchRegex:
		if m.re == nil {
			return nil, false
		}
		if groups := m.re.FindStringSubmatch(body); groups != nil {
			return groups, true
		}
	case matchCommand:
		if args, ok := parseCommand(body, m.text); ok {
			return args, true
		}
	case matchAnyText:
		return nil, true
	}
	return nil, false
}

// parseCommand checks whether body starts with "/<name>" (case-
// insensitive) and is followed either by end-of-string or whitespace
// then arguments. Returns the whitespace-split arguments (excluding
// the command itself) on success.
func parseCommand(body, name string) ([]string, bool) {
	body = strings.TrimLeft(body, " \t")
	if len(body) < 1+len(name) || body[0] != '/' {
		return nil, false
	}
	rest := body[1:]
	lower := strings.ToLower(rest)
	if !strings.HasPrefix(lower, strings.ToLower(name)) {
		return nil, false
	}
	after := rest[len(name):]
	if len(after) == 0 {
		return []string{}, true
	}
	if after[0] != ' ' && after[0] != '\t' {
		// /foobar should not match "/foo".
		return nil, false
	}
	fields := strings.Fields(after)
	return fields, true
}
