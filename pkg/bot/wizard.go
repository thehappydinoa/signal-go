package bot

import (
	"context"
	"fmt"
	"strings"
)

// Wizard is a multi-step conversation builder that prefixes stages as
// "name:step" and registers [Bot.OnAnyText] handlers gated by stage.
type Wizard struct {
	bot   *Bot
	name  string
	steps []wizardStep
}

type wizardStep struct {
	stage   string
	handler HandlerFunc
}

// Wizard returns a named multi-step flow builder. Call [Wizard.Step] to
// register handlers, then [Wizard.Register] during bot setup.
func (b *Bot) Wizard(name string) *Wizard {
	if name == "" {
		panic("bot.Wizard: empty name")
	}
	return &Wizard{bot: b, name: name}
}

// Step registers a handler for the named step. The handler runs when the
// conversation stage equals name:step (see [Wizard.Begin]).
func (w *Wizard) Step(step string, fn HandlerFunc) *Wizard {
	if step == "" {
		panic("bot.Wizard.Step: empty step")
	}
	w.steps = append(w.steps, wizardStep{stage: step, handler: fn})
	return w
}

// Register wires all steps onto the bot via stage-gated [OnAnyText] handlers.
func (w *Wizard) Register() {
	for _, s := range w.steps {
		stage := w.stageName(s.stage)
		w.bot.OnAnyText().Stage(stage).Do(s.handler)
	}
}

// Begin starts the wizard on m's conversation at the first step. If step is
// empty, the first registered step is used.
func (w *Wizard) Begin(ctx context.Context, m *Message, step string) error {
	if len(w.steps) == 0 {
		return fmt.Errorf("bot.Wizard.Begin: no steps registered for %q", w.name)
	}
	if step == "" {
		step = w.steps[0].stage
	}
	m.Convo().SetStage(w.stageName(step))
	return nil
}

// Advance moves the conversation to the next wizard step.
func (w *Wizard) Advance(m *Message, step string) {
	m.Convo().SetStage(w.stageName(step))
}

// Clear removes the wizard stage prefix from the conversation.
func (w *Wizard) Clear(m *Message) {
	current := m.Convo().Stage()
	prefix := w.name + ":"
	if strings.HasPrefix(current, prefix) {
		m.Convo().ClearStage()
	}
}

func (w *Wizard) stageName(step string) string {
	return w.name + ":" + step
}
