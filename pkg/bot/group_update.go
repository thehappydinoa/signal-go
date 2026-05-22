package bot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/pkg/signal"
)

// GroupUpdateHandlerFunc is invoked for inbound group membership or
// metadata changes delivered as DataMessage.groupV2.groupChange.
type GroupUpdateHandlerFunc func(ctx context.Context, u *GroupUpdate) error

// GroupUpdate is the per-event handle for [Bot.OnGroupUpdate] handlers.
type GroupUpdate struct {
	event *signal.GroupUpdateEvent
	bot   *Bot
}

// Sender returns the ACI of the member who authored the change.
func (u *GroupUpdate) Sender() string { return u.event.Sender }

// GroupID returns the hex-encoded group v2 master key.
func (u *GroupUpdate) GroupID() string { return u.event.GroupID }

// Revision returns the post-change revision advertised in the update.
func (u *GroupUpdate) Revision() uint32 { return u.event.Revision }

// Timestamp returns the update message timestamp.
func (u *GroupUpdate) Timestamp() time.Time { return u.event.Timestamp }

// GroupChange returns the signed change blob from the update.
func (u *GroupUpdate) GroupChange() []byte { return u.event.GroupChange }

// Event returns the wrapped typed event.
func (u *GroupUpdate) Event() *signal.GroupUpdateEvent { return u.event }

// Sync applies the change by syncing group logs from the cached local
// revision via [signal.Client.SyncGroup].
func (u *GroupUpdate) Sync(ctx context.Context) (*signal.Group, error) {
	syncer, ok := u.bot.cli.(groupSyncer)
	if !ok {
		return nil, errors.New("bot.GroupUpdate.Sync: client does not support SyncGroup")
	}
	masterKey, err := decodeGroupMasterKey(u.event.GroupID)
	if err != nil {
		return nil, err
	}
	from := u.event.Revision
	if from > 0 {
		from--
	}
	grp, err := syncer.SyncGroup(ctx, masterKey, from)
	if err != nil {
		return nil, fmt.Errorf("bot.GroupUpdate.Sync: %w", err)
	}
	return grp, nil
}

type groupSyncer interface {
	SyncGroup(ctx context.Context, masterKey []byte, fromRevision uint32) (*signal.Group, error)
}

type groupUpdateHandler struct {
	run GroupUpdateHandlerFunc
}
