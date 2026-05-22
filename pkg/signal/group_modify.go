package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/group"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

// LeaveGroup removes the local device from a Groups v2 chat.
func (c *Client) LeaveGroup(ctx context.Context, masterKey []byte) error {
	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return fmt.Errorf("signal.LeaveGroup: %w", err)
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return err
	}
	actions, err := group.BuildLeaveActions(secretParams, c.acct.ACI, grp.Revision)
	if err != nil {
		return err
	}
	if _, err := c.patchGroup(ctx, secretParams, actions); err != nil {
		return err
	}
	masterKeyHex := hex.EncodeToString(masterKey)
	c.deleteGroupSendEndorsements(masterKeyHex)
	c.groupDistMu.Lock()
	delete(c.groupDistID, masterKeyHex)
	c.groupDistMu.Unlock()
	return nil
}

// SetMemberRole changes a member's role (promote/demote). The local user must
// be a group administrator.
func (c *Client) SetMemberRole(ctx context.Context, masterKey []byte, memberACI string, role GroupRole) (*Group, error) {
	if memberACI == "" {
		return nil, errors.New("signal.SetMemberRole: empty member ACI")
	}
	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, fmt.Errorf("signal.SetMemberRole: %w", err)
	}
	if !grp.IsAdmin(c.acct.ACI) {
		return nil, errors.New("signal.SetMemberRole: local user is not an administrator")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	actions, err := group.BuildModifyRoleActions(secretParams, c.acct.ACI, memberACI, role, grp.Revision)
	if err != nil {
		return nil, err
	}
	resp, err := c.patchGroup(ctx, secretParams, actions)
	if err != nil {
		return nil, err
	}
	updated, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, err
	}
	changeBytes, err := proto.Marshal(resp.GetGroupChange())
	if err != nil {
		return nil, err
	}
	if err := c.sendGroupChangeNotification(ctx, masterKey, updated, changeBytes); err != nil {
		c.log.Warn("group change notification failed", "err", err)
	}
	return updated, nil
}

// PromoteMember grants administrator role to memberACI.
func (c *Client) PromoteMember(ctx context.Context, masterKey []byte, memberACI string) (*Group, error) {
	return c.SetMemberRole(ctx, masterKey, memberACI, GroupRoleAdministrator)
}

// DemoteMember revokes administrator role from memberACI.
func (c *Client) DemoteMember(ctx context.Context, masterKey []byte, memberACI string) (*Group, error) {
	return c.SetMemberRole(ctx, masterKey, memberACI, GroupRoleDefault)
}

// RemoveMember removes memberACI from the group. The local user must be an
// administrator and cannot remove themselves (use [LeaveGroup]).
func (c *Client) RemoveMember(ctx context.Context, masterKey []byte, memberACI string) (*Group, error) {
	if memberACI == "" {
		return nil, errors.New("signal.RemoveMember: empty member ACI")
	}
	if memberACI == c.acct.ACI {
		return nil, errors.New("signal.RemoveMember: use LeaveGroup to remove the local user")
	}
	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, fmt.Errorf("signal.RemoveMember: %w", err)
	}
	if !grp.IsAdmin(c.acct.ACI) {
		return nil, errors.New("signal.RemoveMember: local user is not an administrator")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	actions, err := group.BuildRemoveMemberActions(secretParams, c.acct.ACI, memberACI, grp.Revision)
	if err != nil {
		return nil, err
	}
	resp, err := c.patchGroup(ctx, secretParams, actions)
	if err != nil {
		return nil, err
	}
	updated, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, err
	}
	changeBytes, err := proto.Marshal(resp.GetGroupChange())
	if err != nil {
		return nil, err
	}
	if err := c.sendGroupChangeNotification(ctx, masterKey, updated, changeBytes); err != nil {
		c.log.Warn("group change notification failed", "err", err)
	}
	return updated, nil
}

// AddMember adds memberACI to the group using their profile key (fetches an
// expiring profile key credential from the chat service). The local user must
// be an administrator.
func (c *Client) AddMember(ctx context.Context, masterKey []byte, memberACI string, profileKey []byte, role GroupRole) (*Group, error) {
	if memberACI == "" {
		return nil, errors.New("signal.AddMember: empty member ACI")
	}
	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, fmt.Errorf("signal.AddMember: %w", err)
	}
	if !grp.IsAdmin(c.acct.ACI) {
		return nil, errors.New("signal.AddMember: local user is not an administrator")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	presentation, err := c.memberPresentationForAdd(ctx, secretParams, memberACI, profileKey)
	if err != nil {
		return nil, err
	}
	actions, err := group.BuildAddMemberActions(secretParams, c.acct.ACI, presentation, role, grp.Revision)
	if err != nil {
		return nil, err
	}
	resp, err := c.patchGroup(ctx, secretParams, actions)
	if err != nil {
		return nil, err
	}
	updated, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, err
	}
	changeBytes, err := proto.Marshal(resp.GetGroupChange())
	if err != nil {
		return nil, err
	}
	if err := c.sendGroupChangeNotification(ctx, masterKey, updated, changeBytes); err != nil {
		c.log.Warn("group change notification failed", "err", err)
	}
	return updated, nil
}

func (c *Client) patchGroup(
	ctx context.Context,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actions []byte,
) (*groupspb.GroupChangeResponse, error) {
	if c.storageWebc == nil {
		return nil, errors.New("signal: Client was opened without groups storage")
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return nil, err
	}
	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return nil, err
	}

	raw, err := c.storageWebc.PatchGroup(ctx, authHeader, actions)
	if err != nil {
		return nil, fmt.Errorf("signal: patch group: %w", err)
	}

	var resp groupspb.GroupChangeResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("signal: decode patch response: %w", err)
	}
	if resp.GetGroupChange() == nil {
		return nil, errors.New("signal: patch returned empty group change")
	}
	return &resp, nil
}

func (c *Client) patchGroupWithInvite(
	ctx context.Context,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	inviteLinkPassword []byte,
	actions []byte,
) (*groupspb.GroupChangeResponse, error) {
	if c.storageWebc == nil {
		return nil, errors.New("signal: Client was opened without groups storage")
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return nil, err
	}
	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return nil, err
	}
	passB64 := group.InviteLinkPasswordBase64(inviteLinkPassword)
	raw, err := c.storageWebc.PatchGroupWithInvite(ctx, authHeader, passB64, actions)
	if err != nil {
		return nil, fmt.Errorf("signal: patch group with invite: %w", err)
	}
	var resp groupspb.GroupChangeResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("signal: decode patch response: %w", err)
	}
	if resp.GetGroupChange() == nil {
		return nil, errors.New("signal: patch returned empty group change")
	}
	return &resp, nil
}

func (c *Client) sendGroupChangeNotification(ctx context.Context, masterKey []byte, grp *Group, groupChange []byte) error {
	if grp == nil {
		return errors.New("signal: nil group")
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildGroupUpdateContent(masterKey, grp.Revision, groupChange, ts)
	if err != nil {
		return err
	}
	_, err = c.sendGroupContent(ctx, masterKey, grp, contentBytes, ts)
	return err
}

func buildGroupUpdateContent(masterKey []byte, revision uint32, groupChange []byte, tsMillis uint64) ([]byte, error) {
	timestamp := tsMillis
	rev := revision
	gv2 := &sspb.GroupContextV2{
		MasterKey: masterKey,
		Revision:  &rev,
	}
	if len(groupChange) > 0 {
		gv2.GroupChange = groupChange
	}
	content := &sspb.Content{
		Content: &sspb.Content_DataMessage{
			DataMessage: &sspb.DataMessage{
				Timestamp: &timestamp,
				GroupV2:   gv2,
			},
		},
	}
	return proto.Marshal(content)
}

// sendGroupContent delivers pre-built group Content bytes via multi-recipient.
func (c *Client) sendGroupContent(ctx context.Context, masterKey []byte, grp *Group, contentBytes []byte, ts uint64) (Receipt, error) {
	return c.deliverGroupPayload(ctx, masterKey, grp, contentBytes, ts, groupDeliveryOpts{
		online:         false,
		urgent:         true,
		distributeSKDM: false,
	})
}
