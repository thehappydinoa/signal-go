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
)

// matcher describes one registered pattern. Only the relevant fields
// for the kind are populated. Scope filters (dmOnly, groupOnly, fromACI)
// are evaluated before the pattern match.
type matcher struct {
	kind      matchKind
	text      string
	re        *regexp.Regexp
	dmOnly    bool
	groupOnly bool
	fromACI   string
}

// match evaluates the matcher against an inbound message event. Scope
// filters are tested first; then the pattern match runs. Returns
// (capture-groups-or-args, true) on match, (nil, false) otherwise.
func (m matcher) match(ev *signal.MessageEvent, msg *Message) ([]string, bool) {
	if m.dmOnly && msg.IsGroup() {
		return nil, false
	}
	if m.groupOnly && !msg.IsGroup() {
		return nil, false
	}
	if m.fromACI != "" && ev.Sender != m.fromACI {
		return nil, false
	}
	body := ev.Body
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
		groups := m.re.FindStringSubmatch(body)
		if groups != nil {
			return groups, true
		}
	case matchCommand:
		args, ok := parseCommand(body, m.text)
		if ok {
			return args, true
		}
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
