package signal

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/group"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

// ParseGroupInviteLink parses a signal.group invite URL into master key and
// password bytes.
func ParseGroupInviteLink(rawURL string) (masterKey, password []byte, err error) {
	parsed, err := group.ParseInviteLinkURL(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("signal.ParseGroupInviteLink: %w", err)
	}
	return parsed.MasterKey, parsed.InviteLinkPassword, nil
}

// GroupJoinPreview is a decrypted preview of a group joinable via invite link.
type GroupJoinPreview struct {
	Title                 string
	Description           string
	AvatarURL             string
	MemberCount           uint32
	Revision              uint32
	RequiresAdminApproval bool
}

// PreviewGroupJoin fetches and decrypts join metadata for an invite link URL.
func (c *Client) PreviewGroupJoin(ctx context.Context, inviteURL string) (*GroupJoinPreview, error) {
	parsed, err := group.ParseInviteLinkURL(inviteURL)
	if err != nil {
		return nil, fmt.Errorf("signal.PreviewGroupJoin: %w", err)
	}
	if c.storageWebc == nil {
		return nil, errors.New("signal.PreviewGroupJoin: Client was opened without groups storage")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(parsed.MasterKey)
	if err != nil {
		return nil, err
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return nil, err
	}
	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return nil, fmt.Errorf("signal.PreviewGroupJoin: authorize: %w", err)
	}
	passB64 := group.InviteLinkPasswordBase64(parsed.InviteLinkPassword)
	raw, err := c.storageWebc.FetchGroupJoinInfo(ctx, authHeader, passB64)
	if err != nil {
		return nil, fmt.Errorf("signal.PreviewGroupJoin: fetch: %w", err)
	}
	var wire groupspb.GroupJoinInfo
	if err := proto.Unmarshal(raw, &wire); err != nil {
		return nil, fmt.Errorf("signal.PreviewGroupJoin: decode: %w", err)
	}
	info, err := group.DecodeJoinInfo(secretParams, &wire)
	if err != nil {
		return nil, err
	}
	return &GroupJoinPreview{
		Title:                 info.Title,
		Description:           info.Description,
		AvatarURL:             info.AvatarURL,
		MemberCount:           info.MemberCount,
		Revision:              info.Revision,
		RequiresAdminApproval: info.AddFromInviteLink == groupspb.AccessControl_ADMINISTRATOR || info.PendingAdminApproval,
	}, nil
}

// JoinGroupViaInviteLink joins the group described by inviteURL using the
// linked account's profile key. When admin approval is required the local user
// is added to the pending list instead of full membership.
func (c *Client) JoinGroupViaInviteLink(ctx context.Context, inviteURL string) (*Group, error) {
	parsed, err := group.ParseInviteLinkURL(inviteURL)
	if err != nil {
		return nil, fmt.Errorf("signal.JoinGroupViaInviteLink: %w", err)
	}
	preview, err := c.PreviewGroupJoin(ctx, inviteURL)
	if err != nil {
		return nil, err
	}
	if len(c.acct.ProfileKey) == 0 {
		return nil, errors.New("signal.JoinGroupViaInviteLink: linked account has no profile key")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(parsed.MasterKey)
	if err != nil {
		return nil, err
	}
	presentation, err := c.memberPresentationForAdd(ctx, secretParams, c.acct.ACI, c.acct.ProfileKey)
	if err != nil {
		return nil, fmt.Errorf("signal.JoinGroupViaInviteLink: presentation: %w", err)
	}
	var actions []byte
	if preview.RequiresAdminApproval {
		actions, err = group.BuildJoinRequestActions(secretParams, c.acct.ACI, presentation, preview.Revision)
	} else {
		actions, err = group.BuildJoinViaInviteLinkActions(secretParams, c.acct.ACI, presentation, preview.Revision)
	}
	if err != nil {
		return nil, err
	}
	if _, err := c.patchGroupWithInvite(ctx, secretParams, parsed.InviteLinkPassword, actions); err != nil {
		return nil, err
	}
	return c.FetchGroup(ctx, parsed.MasterKey)
}
