package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

// SendGroupReaction sends a reaction (or removes one) in a Groups v2 chat.
func (c *Client) SendGroupReaction(
	ctx context.Context,
	masterKey []byte,
	emoji, targetAuthorACI string,
	targetTimestamp time.Time,
	remove bool,
) (Receipt, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return Receipt{}, fmt.Errorf("signal.SendGroupReaction: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	if targetAuthorACI == "" {
		return Receipt{}, errors.New("signal.SendGroupReaction: empty target author")
	}
	if targetTimestamp.IsZero() {
		return Receipt{}, errors.New("signal.SendGroupReaction: zero target timestamp")
	}
	if !remove && emoji == "" {
		return Receipt{}, errors.New("signal.SendGroupReaction: emoji required when not removing")
	}

	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return Receipt{}, fmt.Errorf("signal.SendGroupReaction: fetch group: %w", err)
	}

	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildGroupReactionContent(emoji, targetAuthorACI, targetTimestamp, remove, ts, masterKey, grp.Revision)
	if err != nil {
		return Receipt{}, err
	}

	return c.deliverGroupPayload(ctx, masterKey, grp, contentBytes, ts, groupDeliveryOpts{
		online:         false,
		urgent:         true,
		distributeSKDM: true,
	})
}

// SendGroupTyping sends a typing indicator in a Groups v2 chat.
func (c *Client) SendGroupTyping(ctx context.Context, masterKey []byte, action TypingAction) (Receipt, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return Receipt{}, fmt.Errorf("signal.SendGroupTyping: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}

	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return Receipt{}, fmt.Errorf("signal.SendGroupTyping: fetch group: %w", err)
	}

	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildGroupTypingContent(action, ts, masterKey)
	if err != nil {
		return Receipt{}, err
	}

	return c.deliverGroupPayload(ctx, masterKey, grp, contentBytes, ts, groupDeliveryOpts{
		online:         true,
		urgent:         false,
		distributeSKDM: true,
	})
}

func buildGroupReactionContent(
	emoji, targetAuthorACI string,
	targetTS time.Time,
	remove bool,
	tsMillis uint64,
	masterKey []byte,
	revision uint32,
) ([]byte, error) {
	base, err := buildReactionContent(emoji, targetAuthorACI, targetTS, remove, tsMillis)
	if err != nil {
		return nil, err
	}
	return attachGroupV2ToDataMessage(base, masterKey, revision)
}

func buildGroupTypingContent(action TypingAction, tsMillis uint64, masterKey []byte) ([]byte, error) {
	groupID, err := libsignal.GroupIdentifierFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	return buildTypingContent(action, tsMillis, groupID[:])
}

func attachGroupV2ToDataMessage(contentBytes []byte, masterKey []byte, revision uint32) ([]byte, error) {
	var content sspb.Content
	if err := proto.Unmarshal(contentBytes, &content); err != nil {
		return nil, fmt.Errorf("signal: decode content: %w", err)
	}
	dm := content.GetDataMessage()
	if dm == nil {
		return nil, errors.New("signal: content has no DataMessage")
	}
	rev := revision
	dm.GroupV2 = &sspb.GroupContextV2{
		MasterKey: masterKey,
		Revision:  &rev,
	}
	return proto.Marshal(&content)
}
