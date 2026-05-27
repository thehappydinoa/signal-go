// Command poll-bot demonstrates group workflow scripts using reactions.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/thehappydinoa/signal-go/examples/internal/botexample"
	"github.com/thehappydinoa/signal-go/pkg/bot"
)

func main() {
	os.Exit(botexample.Run(os.Args[1:], ".signal-poll-bot", setup))
}

func setup(b *bot.Bot) error {
	b.OnCommand("poll").Group().Do(func(ctx context.Context, m *bot.Message, args []string) error {
		if len(args) == 0 {
			return m.Reply(ctx, "usage: /poll open <question> | /poll status | /poll close")
		}
		state := pollState(b, m.GroupID())
		switch strings.ToLower(args[0]) {
		case "open":
			if len(args) < 2 {
				return m.Reply(ctx, "usage: /poll open <question>")
			}
			question := strings.TrimSpace(strings.Join(args[1:], " "))
			state.Set("open", "true")
			state.Set("question", question)
			state.Set("yes", "0")
			state.Set("no", "0")
			clearVotes(state)
			return m.Reply(ctx, fmt.Sprintf("Poll opened: %s\nReact with 👍 or 👎.", question))
		case "status":
			return m.Reply(ctx, pollSummary(state))
		case "close":
			state.Set("open", "false")
			return m.Reply(ctx, "Poll closed.\n"+pollSummary(state))
		default:
			return m.Reply(ctx, "usage: /poll open <question> | /poll status | /poll close")
		}
	})

	b.OnReaction("👍").Group().Do(func(ctx context.Context, r *bot.Reaction) error {
		return applyVote(b, r, "yes")
	})
	b.OnReaction("👎").Group().Do(func(ctx context.Context, r *bot.Reaction) error {
		return applyVote(b, r, "no")
	})

	return nil
}

func pollState(b *bot.Bot, groupID string) *bot.Convo {
	return b.Convo().For(bot.ConvoKey{Sender: "_poll_", GroupID: groupID})
}

func clearVotes(state *bot.Convo) {
	all := state.All()
	for key := range all {
		if strings.HasPrefix(key, "vote:") {
			state.Delete(key)
		}
	}
}

func pollSummary(state *bot.Convo) string {
	question, _ := state.Get("question")
	yes := parseInt(state, "yes")
	no := parseInt(state, "no")
	open := "closed"
	if v, _ := state.Get("open"); v == "true" {
		open = "open"
	}
	if question == "" {
		question = "(none)"
	}
	return fmt.Sprintf("Poll is %s\nQuestion: %s\n👍 %d\n👎 %d", open, question, yes, no)
}

func applyVote(b *bot.Bot, r *bot.Reaction, vote string) error {
	state := pollState(b, r.GroupID())
	if open, _ := state.Get("open"); open != "true" {
		return nil
	}
	field := "vote:" + r.Sender()
	prev, hadPrev := state.Get(field)
	if hadPrev && prev == vote {
		return nil
	}
	if hadPrev {
		decrement(state, prev)
	}
	increment(state, vote)
	state.Set(field, vote)
	return nil
}

func increment(state *bot.Convo, vote string) {
	value := parseInt(state, vote)
	state.Set(vote, strconv.Itoa(value+1))
}

func decrement(state *bot.Convo, vote string) {
	value := parseInt(state, vote)
	if value > 0 {
		state.Set(vote, strconv.Itoa(value-1))
	}
}

func parseInt(c *bot.Convo, field string) int {
	raw, _ := c.Get(field)
	v, _ := strconv.Atoi(raw)
	return v
}
