package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/group"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

// CreateGroupMember is one initial member besides the local administrator.
// ProfileKey may be empty to use a cached key from [Client.SetRecipientProfileKey]
// or prior inbound messages; without a profile key the member is added as a
// pending invite until their credential is available.
type CreateGroupMember struct {
	ACI        string
	ProfileKey []byte
}

// CreateGroupOptions configures [Client.CreateGroup].
type CreateGroupOptions struct {
	Title                       string
	Description                 string
	Members                     []CreateGroupMember
	DisappearingMessagesSeconds int
}

// CreateGroupResult is the outcome of [Client.CreateGroup].
type CreateGroupResult struct {
	Group     *Group
	MasterKey []byte // 32-byte master key; [Group.ID] is its hex encoding
}

// CreateGroup creates a new Groups v2 chat with the linked account as the sole
// administrator. Additional members are added when their profile keys are
// available; otherwise they are placed on the pending-members list.
func (c *Client) CreateGroup(ctx context.Context, opts CreateGroupOptions) (*CreateGroupResult, error) {
	if c.storageWebc == nil {
		return nil, errors.New("signal.CreateGroup: Client was opened without groups storage")
	}
	if len(c.acct.ProfileKey) == 0 {
		return nil, errors.New("signal.CreateGroup: linked account has no profile key")
	}

	masterKey, secretParams, err := libsignal.GenerateGroupMasterKey()
	if err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: %w", err)
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return nil, err
	}

	selfPresentation, err := c.memberPresentationForAdd(ctx, secretParams, c.acct.ACI, c.acct.ProfileKey)
	if err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: self presentation: %w", err)
	}

	members, pending, err := c.buildCreateGroupMembers(ctx, secretParams, opts.Members)
	if err != nil {
		return nil, err
	}

	groupWire, err := group.BuildNewGroupMessage(group.NewGroupMessageParams{
		SecretParams:                secretParams,
		PublicParams:                publicParams,
		Title:                       opts.Title,
		Description:                 opts.Description,
		DisappearingMessagesSeconds: opts.DisappearingMessagesSeconds,
		SelfPresentation:            selfPresentation,
		Members:                     members,
		PendingMembers:              pending,
	})
	if err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: %w", err)
	}

	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: authorize: %w", err)
	}
	raw, err := c.storageWebc.PutGroup(ctx, authHeader, groupWire)
	if err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: put group: %w", err)
	}

	var resp groupspb.GroupResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: decode response: %w", err)
	}
	if resp.GetGroup() == nil {
		return nil, errors.New("signal.CreateGroup: server returned empty group")
	}

	masterKeyHex := hex.EncodeToString(masterKey)
	state, err := group.DecodeState(secretParams, resp.GetGroup())
	if err != nil {
		return nil, fmt.Errorf("signal.CreateGroup: decrypt: %w", err)
	}
	grp, memberACIs, err := groupFromDecodedState(masterKeyHex, state)
	if err != nil {
		return nil, err
	}
	if gse := resp.GetGroupSendEndorsementsResponse(); len(gse) > 0 {
		if err := c.storeGroupSendEndorsements(masterKeyHex, secretParams, gse, memberACIs); err != nil {
			c.log.Warn("group send endorsements unavailable", "group", masterKeyHex, "err", err)
		}
	}
	c.storeGroupRevision(masterKeyHex, grp.Revision)

	if err := c.sendGroupChangeNotification(ctx, masterKey, grp, nil); err != nil {
		c.log.Warn("group create notification failed", "err", err)
	}

	return &CreateGroupResult{
		Group:     grp,
		MasterKey: masterKey,
	}, nil
}

// FormatGroupInviteLink builds a https://signal.group invite URL from a master
// key and invite-link password.
func FormatGroupInviteLink(masterKey, inviteLinkPassword []byte) (string, error) {
	url, err := group.FormatInviteLinkURL(masterKey, inviteLinkPassword)
	if err != nil {
		return "", fmt.Errorf("signal.FormatGroupInviteLink: %w", err)
	}
	return url, nil
}

// GroupInviteLinkAccess controls who may join via an invite link.
type GroupInviteLinkAccess = groupspb.AccessControl_AccessRequired

const (
	GroupInviteLinkAccessAny           = groupspb.AccessControl_ANY
	GroupInviteLinkAccessMember        = groupspb.AccessControl_MEMBER
	GroupInviteLinkAccessAdministrator = groupspb.AccessControl_ADMINISTRATOR
)

// EnableGroupInviteLink sets a fresh invite-link password and join policy on a
// group. The local user must be an administrator. Returns the invite URL and
// updated group state.
func (c *Client) EnableGroupInviteLink(ctx context.Context, masterKey []byte, access GroupInviteLinkAccess) (inviteURL string, grp *Group, err error) {
	grp, err = c.FetchGroup(ctx, masterKey)
	if err != nil {
		return "", nil, fmt.Errorf("signal.EnableGroupInviteLink: %w", err)
	}
	if !grp.IsAdmin(c.acct.ACI) {
		return "", nil, errors.New("signal.EnableGroupInviteLink: local user is not an administrator")
	}
	password, err := group.GenerateInviteLinkPassword()
	if err != nil {
		return "", nil, err
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return "", nil, err
	}
	actions, err := group.BuildEnableInviteLinkActions(secretParams, c.acct.ACI, password, access, grp.Revision)
	if err != nil {
		return "", nil, err
	}
	resp, err := c.patchGroup(ctx, secretParams, actions)
	if err != nil {
		return "", nil, err
	}
	updated, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return "", nil, err
	}
	changeBytes, err := proto.Marshal(resp.GetGroupChange())
	if err != nil {
		return "", nil, err
	}
	if err := c.sendGroupChangeNotification(ctx, masterKey, updated, changeBytes); err != nil {
		c.log.Warn("group change notification failed", "err", err)
	}
	url, err := FormatGroupInviteLink(masterKey, password)
	if err != nil {
		return "", updated, err
	}
	return url, updated, nil
}

// GroupInviteLinkURL returns the invite URL for a group that already has an
// invite link enabled.
func (c *Client) GroupInviteLinkURL(ctx context.Context, masterKey []byte) (string, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return "", fmt.Errorf("signal.GroupInviteLinkURL: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	if c.storageWebc == nil {
		return "", errors.New("signal.GroupInviteLinkURL: Client was opened without groups storage")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return "", err
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return "", err
	}
	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return "", fmt.Errorf("signal.GroupInviteLinkURL: authorize: %w", err)
	}
	raw, err := c.storageWebc.FetchGroupState(ctx, authHeader)
	if err != nil {
		return "", fmt.Errorf("signal.GroupInviteLinkURL: %w", err)
	}
	var resp groupspb.GroupResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("signal.GroupInviteLinkURL: decode: %w", err)
	}
	password := resp.GetGroup().GetInviteLinkPassword()
	if len(password) == 0 {
		return "", errors.New("signal.GroupInviteLinkURL: group has no invite link")
	}
	return FormatGroupInviteLink(masterKey, password)
}

// SetGroupTitle renames a group. The local user must be allowed to edit group
// attributes (typically any member when using default access control).
func (c *Client) SetGroupTitle(ctx context.Context, masterKey []byte, title string) (*Group, error) {
	return c.modifyGroupAttribute(ctx, masterKey, func(secretParams [libsignal.GroupSecretParamsLen]byte, revision uint32) ([]byte, error) {
		return group.BuildModifyTitleActions(secretParams, c.acct.ACI, title, revision)
	})
}

// SetGroupDescription changes a group's description.
func (c *Client) SetGroupDescription(ctx context.Context, masterKey []byte, description string) (*Group, error) {
	return c.modifyGroupAttribute(ctx, masterKey, func(secretParams [libsignal.GroupSecretParamsLen]byte, revision uint32) ([]byte, error) {
		return group.BuildModifyDescriptionActions(secretParams, c.acct.ACI, description, revision)
	})
}

func (c *Client) modifyGroupAttribute(
	ctx context.Context,
	masterKey []byte,
	build func(secretParams [libsignal.GroupSecretParamsLen]byte, revision uint32) ([]byte, error),
) (*Group, error) {
	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return nil, err
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	actions, err := build(secretParams, grp.Revision)
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

func (c *Client) buildCreateGroupMembers(
	ctx context.Context,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	inputs []CreateGroupMember,
) ([]group.NewGroupMember, []group.NewGroupPendingMember, error) {
	members := make([]group.NewGroupMember, 0, len(inputs))
	pending := make([]group.NewGroupPendingMember, 0, len(inputs))
	for _, m := range inputs {
		if m.ACI == "" {
			return nil, nil, errors.New("signal.CreateGroup: empty member ACI")
		}
		if m.ACI == c.acct.ACI {
			return nil, nil, errors.New("signal.CreateGroup: members must not include the local account")
		}
		profileKey := m.ProfileKey
		if len(profileKey) == 0 {
			c.mu.Lock()
			profileKey = append([]byte(nil), c.knownProfileKeys[m.ACI]...)
			c.mu.Unlock()
		}
		if len(profileKey) > 0 {
			presentation, err := c.memberPresentationForAdd(ctx, secretParams, m.ACI, profileKey)
			if err != nil {
				return nil, nil, fmt.Errorf("signal.CreateGroup: member %s: %w", m.ACI, err)
			}
			members = append(members, group.NewGroupMember{
				Presentation: presentation,
				Role:         group.MemberRoleDefault,
			})
			continue
		}
		pending = append(pending, group.NewGroupPendingMember{
			TargetACI: m.ACI,
			Role:      group.MemberRoleDefault,
		})
	}
	return members, pending, nil
}
