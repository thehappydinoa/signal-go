package bot

import (
	"sync"
)

// ConvoKey identifies a single conversation. For 1:1 messages, only
// Sender is populated; for group messages, both fields are set so that
// state in different groups for the same sender does not collide.
type ConvoKey struct {
	// Sender is the ACI of the user the conversation is with.
	Sender string
	// GroupID is the group v2 master key (hex) for group threads, or
	// the empty string for 1:1 DMs.
	GroupID string
}

// ConvoStore persists per-conversation key/value state. Implementations
// must be safe for concurrent use from multiple goroutines.
//
// The default implementation is [MemoryConvoStore]; bots that want
// state to survive process restarts can supply a backing store via
// [Options.ConvoStore].
//
// Values are opaque strings; callers that need richer types should
// JSON-encode before storing and decode after [ConvoStore.Get].
type ConvoStore interface {
	// Get returns the value stored at (key, field) and whether it was
	// present.
	Get(key ConvoKey, field string) (value string, ok bool)
	// Set writes value at (key, field), overwriting any previous
	// value.
	Set(key ConvoKey, field, value string)
	// Delete removes a single field from key. No-op if absent.
	Delete(key ConvoKey, field string)
	// Clear removes every field for key. No-op if no state exists.
	Clear(key ConvoKey)
	// All returns a copy of every field stored under key. The returned
	// map is safe to mutate; it is decoupled from the store.
	All(key ConvoKey) map[string]string
}

// MemoryConvoStore is the default in-memory [ConvoStore]. It is safe
// for concurrent use; state is lost when the process exits.
type MemoryConvoStore struct {
	mu    sync.RWMutex
	state map[ConvoKey]map[string]string
}

// NewMemoryConvoStore returns an empty [MemoryConvoStore].
func NewMemoryConvoStore() *MemoryConvoStore {
	return &MemoryConvoStore{state: make(map[ConvoKey]map[string]string)}
}

// Get implements [ConvoStore].
func (s *MemoryConvoStore) Get(key ConvoKey, field string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.state[key]
	if !ok {
		return "", false
	}
	v, ok := m[field]
	return v, ok
}

// Set implements [ConvoStore].
func (s *MemoryConvoStore) Set(key ConvoKey, field, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.state[key]
	if !ok {
		m = make(map[string]string)
		s.state[key] = m
	}
	m[field] = value
}

// Delete implements [ConvoStore].
func (s *MemoryConvoStore) Delete(key ConvoKey, field string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.state[key]
	if !ok {
		return
	}
	delete(m, field)
	if len(m) == 0 {
		delete(s.state, key)
	}
}

// Clear implements [ConvoStore].
func (s *MemoryConvoStore) Clear(key ConvoKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state, key)
}

// All implements [ConvoStore].
func (s *MemoryConvoStore) All(key ConvoKey) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.state[key]
	if !ok {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// stageField is the well-known field name used by [Convo.SetStage] /
// [Convo.Stage] / [Match.Stage]. Defining the constant here keeps the
// matcher and the helpers in lock-step.
const stageField = "_stage"

// Convo is a per-key handle into a [ConvoStore]. The bot returns one
// from [Bot.Convo] (general-purpose) and from [Message.Convo] (scoped
// to the current message's sender + group).
type Convo struct {
	store ConvoStore
	key   ConvoKey
}

// Key returns the [ConvoKey] this Convo is bound to.
func (c *Convo) Key() ConvoKey { return c.key }

// Get returns the value stored at field and whether it was present.
func (c *Convo) Get(field string) (string, bool) {
	if c == nil || c.store == nil {
		return "", false
	}
	return c.store.Get(c.key, field)
}

// Set writes value at field.
func (c *Convo) Set(field, value string) {
	if c == nil || c.store == nil {
		return
	}
	c.store.Set(c.key, field, value)
}

// Delete removes a single field.
func (c *Convo) Delete(field string) {
	if c == nil || c.store == nil {
		return
	}
	c.store.Delete(c.key, field)
}

// Clear removes every field for this conversation.
func (c *Convo) Clear() {
	if c == nil || c.store == nil {
		return
	}
	c.store.Clear(c.key)
}

// All returns a copy of every field for this conversation.
func (c *Convo) All() map[string]string {
	if c == nil || c.store == nil {
		return map[string]string{}
	}
	return c.store.All(c.key)
}

// Stage returns the conversation's current stage, or the empty string
// if none is set. Stages are a convenience for FSM-style flows; pair
// with [Match.Stage] to gate handlers on the current stage.
func (c *Convo) Stage() string {
	v, _ := c.Get(stageField)
	return v
}

// SetStage writes the conversation's current stage. Passing an empty
// stage clears it (equivalent to [Convo.ClearStage]).
func (c *Convo) SetStage(stage string) {
	if stage == "" {
		c.ClearStage()
		return
	}
	c.Set(stageField, stage)
}

// ClearStage removes any stored stage, returning the conversation to
// the implicit "no stage" state.
func (c *Convo) ClearStage() {
	c.Delete(stageField)
}

// Conversations is the bot-wide handle returned by [Bot.Convo]. Use
// [Conversations.For] to obtain a per-key [Convo].
type Conversations struct {
	store ConvoStore
}

// For returns a [Convo] scoped to the given key.
func (c *Conversations) For(key ConvoKey) *Convo {
	if c == nil {
		return &Convo{key: key}
	}
	return &Convo{store: c.store, key: key}
}

// Store returns the underlying [ConvoStore] for callers that want
// to query or mutate state outside the per-key helpers.
func (c *Conversations) Store() ConvoStore {
	if c == nil {
		return nil
	}
	return c.store
}
