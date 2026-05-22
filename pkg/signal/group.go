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
	"github.com/thehappydinoa/signal-go/internal/web"
)

// GroupRole is the role of a member in a Groups v2 chat.
type GroupRole = group.MemberRole

const (
	GroupRoleUnknown       = group.MemberRoleUnknown
	GroupRoleDefault       = group.MemberRoleDefault
	GroupRoleAdministrator = group.MemberRoleAdministrator
)

// GroupMember is one decrypted member of a group.
type GroupMember struct {
	ACI  string
	Role GroupRole
}

// Group is a decrypted Groups v2 group snapshot.
type Group struct {
	// ID is the hex-encoded 32-byte group master key. This matches
	// [MessageEvent.GroupID] on inbound group messages.
	ID          string
	Title       string
	Description string
	AvatarURL   string
	Revision    uint32
	Members     []GroupMember
}

// Admins returns ACIs with administrator role.
func (g *Group) Admins() []string {
	if g == nil {
		return nil
	}
	var out []string
	for _, m := range g.Members {
		if m.Role == GroupRoleAdministrator {
			out = append(out, m.ACI)
		}
	}
	return out
}

// IsAdmin reports whether aci has administrator role.
func (g *Group) IsAdmin(aci string) bool {
	if g == nil {
		return false
	}
	for _, m := range g.Members {
		if m.ACI == aci && m.Role == GroupRoleAdministrator {
			return true
		}
	}
	return false
}

// FetchGroup retrieves and decrypts the current group state for the group
// identified by masterKey (32 bytes). masterKey is the same value carried in
// inbound DataMessage.groupV2.masterKey and exposed as [MessageEvent.GroupID]
// (hex-encoded).
func (c *Client) FetchGroup(ctx context.Context, masterKey []byte) (*Group, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return nil, fmt.Errorf("signal.FetchGroup: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	if c.webc == nil || c.storageWebc == nil {
		return nil, errors.New("signal.FetchGroup: Client was opened without send-side dependencies")
	}

	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroup: derive secret params: %w", err)
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroup: public params: %w", err)
	}

	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroup: authorize: %w", err)
	}

	raw, err := c.storageWebc.FetchGroupState(ctx, authHeader)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroup: %w", err)
	}

	var resp groupspb.GroupResponse
	if err := proto.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("signal.FetchGroup: decode response: %w", err)
	}
	if resp.GetGroup() == nil {
		return nil, errors.New("signal.FetchGroup: server returned empty group")
	}

	state, err := group.DecodeState(secretParams, resp.GetGroup())
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroup: decrypt: %w", err)
	}

	members := make([]GroupMember, len(state.Members))
	for i, m := range state.Members {
		members[i] = GroupMember{ACI: m.ACI, Role: m.Role}
	}

	return &Group{
		ID:          hex.EncodeToString(masterKey),
		Title:       state.Title,
		Description: state.Description,
		AvatarURL:   state.AvatarURL,
		Revision:    state.Revision,
		Members:     members,
	}, nil
}

func (c *Client) groupsV2AuthHeader(
	ctx context.Context,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	publicParams [libsignal.GroupPublicParamsLen]byte,
) (string, error) {
	day := web.CurrentDaySeconds(time.Now())

	c.groupAuthMu.Lock()
	responseBytes, ok := c.groupAuthCreds[day]
	c.groupAuthMu.Unlock()
	if !ok {
		resp, err := c.webc.FetchGroupAuthCredentials(ctx, c.credentials(), day)
		if err != nil {
			return "", err
		}
		c.groupAuthMu.Lock()
		if c.groupAuthCreds == nil {
			c.groupAuthCreds = make(map[int64][]byte)
		}
		for _, cred := range resp.Credentials {
			c.groupAuthCreds[cred.RedemptionTime] = append([]byte(nil), cred.Credential...)
		}
		responseBytes = c.groupAuthCreds[day]
		c.groupAuthMu.Unlock()
	}
	if len(responseBytes) == 0 {
		return "", fmt.Errorf("no auth credential for redemption day %d", day)
	}

	serverParams, err := libsignal.ProductionServerPublicParams()
	if err != nil {
		return "", err
	}

	aci, err := libsignal.ParseServiceIDString(c.acct.ACI)
	if err != nil {
		return "", fmt.Errorf("parse ACI: %w", err)
	}
	pni, err := libsignal.ParseServiceIDString(c.acct.PNI)
	if err != nil {
		return "", fmt.Errorf("parse PNI: %w", err)
	}

	authCredential, err := serverParams.ReceiveAuthCredentialWithPni(aci, pni, uint64(day), responseBytes)
	if err != nil {
		c.groupAuthMu.Lock()
		delete(c.groupAuthCreds, day)
		c.groupAuthMu.Unlock()
		return "", fmt.Errorf("receive auth credential: %w", err)
	}

	randomness, err := libsignal.Randomness()
	if err != nil {
		return "", err
	}
	presentation, err := serverParams.CreateAuthCredentialPresentation(secretParams, authCredential, randomness)
	if err != nil {
		return "", fmt.Errorf("create auth presentation: %w", err)
	}

	return libsignal.GroupsV2AuthorizationHeader(publicParams, presentation), nil
}
