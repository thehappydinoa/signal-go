package group

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

const groupInviteHost = "signal.group"

// ParsedInviteLink holds the master key and invite password from a
// signal.group URL.
type ParsedInviteLink struct {
	MasterKey          []byte
	InviteLinkPassword []byte
}

// ParseInviteLinkURL extracts group master key and invite password from a
// https://signal.group/#... or sgnl://signal.group/#... URL.
func ParseInviteLinkURL(rawURL string) (*ParsedInviteLink, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("group.ParseInviteLinkURL: parse url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "sgnl" {
		return nil, errors.New("group.ParseInviteLinkURL: not a signal.group link")
	}
	if !strings.EqualFold(u.Host, groupInviteHost) {
		return nil, errors.New("group.ParseInviteLinkURL: not a signal.group link")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, errors.New("group.ParseInviteLinkURL: unexpected path in link")
	}
	frag := u.Fragment
	if frag == "" {
		// Some clients append tracking query params before the fragment; retry
		// with the substring after '#'.
		if i := strings.Index(rawURL, "#"); i >= 0 {
			frag = rawURL[i+1:]
		}
	}
	if frag == "" {
		return nil, errors.New("group.ParseInviteLinkURL: missing link payload")
	}
	raw, err := base64.RawURLEncoding.DecodeString(frag)
	if err != nil {
		return nil, fmt.Errorf("group.ParseInviteLinkURL: decode fragment: %w", err)
	}
	var link groupspb.GroupInviteLink
	if err := proto.Unmarshal(raw, &link); err != nil {
		return nil, fmt.Errorf("group.ParseInviteLinkURL: unmarshal: %w", err)
	}
	v1 := link.GetContentsV1()
	if v1 == nil {
		return nil, errors.New("group.ParseInviteLinkURL: unsupported link version")
	}
	if len(v1.GetGroupMasterKey()) != libsignal.GroupMasterKeyLen {
		return nil, fmt.Errorf("group.ParseInviteLinkURL: master key length %d", len(v1.GetGroupMasterKey()))
	}
	if len(v1.GetInviteLinkPassword()) == 0 {
		return nil, errors.New("group.ParseInviteLinkURL: empty invite password")
	}
	return &ParsedInviteLink{
		MasterKey:          append([]byte(nil), v1.GetGroupMasterKey()...),
		InviteLinkPassword: append([]byte(nil), v1.GetInviteLinkPassword()...),
	}, nil
}

// JoinInfo is a decrypted preview of a group reachable via invite link.
type JoinInfo struct {
	Title                string
	Description          string
	AvatarURL            string
	MemberCount          uint32
	Revision             uint32
	AddFromInviteLink    groupspb.AccessControl_AccessRequired
	PendingAdminApproval bool
}

// DecodeJoinInfo decrypts a wire-format GroupJoinInfo using secret params
// derived from the group's master key.
func DecodeJoinInfo(secretParams [libsignal.GroupSecretParamsLen]byte, wire *groupspb.GroupJoinInfo) (*JoinInfo, error) {
	if wire == nil {
		return nil, errors.New("group.DecodeJoinInfo: nil join info")
	}
	title, err := decryptTitle(secretParams, wire.GetTitle())
	if err != nil {
		return nil, fmt.Errorf("group.DecodeJoinInfo: title: %w", err)
	}
	description, err := decryptDescription(secretParams, wire.GetDescription())
	if err != nil {
		return nil, fmt.Errorf("group.DecodeJoinInfo: description: %w", err)
	}
	return &JoinInfo{
		Title:                strings.TrimSpace(title),
		Description:          strings.TrimSpace(description),
		AvatarURL:            wire.GetAvatar(),
		MemberCount:          wire.GetMemberCount(),
		Revision:             wire.GetVersion(),
		AddFromInviteLink:    wire.GetAddFromInviteLink(),
		PendingAdminApproval: wire.GetPendingAdminApproval(),
	}, nil
}

// BuildJoinViaInviteLinkActions constructs GroupChange.Actions for the local
// user to join a group directly via invite link.
func BuildJoinViaInviteLinkActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	localACI string,
	presentation []byte,
	currentRevision uint32,
) ([]byte, error) {
	if len(presentation) == 0 {
		return nil, errors.New("group.BuildJoinViaInviteLinkActions: empty presentation")
	}
	localID, err := libsignal.ParseServiceIDString(localACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildJoinViaInviteLinkActions: local aci: %w", err)
	}
	encLocal, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, localID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encLocal[:],
		Version:      version,
		AddMembers: []*groupspb.GroupChange_Actions_AddMemberAction{
			{
				Added: &groupspb.Member{
					Presentation: presentation,
					Role:         groupspb.Member_DEFAULT,
				},
				JoinFromInviteLink: true,
			},
		},
	}
	return proto.Marshal(actions)
}

// BuildJoinRequestActions constructs GroupChange.Actions to request membership
// when the invite link requires administrator approval.
func BuildJoinRequestActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	localACI string,
	presentation []byte,
	currentRevision uint32,
) ([]byte, error) {
	if len(presentation) == 0 {
		return nil, errors.New("group.BuildJoinRequestActions: empty presentation")
	}
	localID, err := libsignal.ParseServiceIDString(localACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildJoinRequestActions: local aci: %w", err)
	}
	encLocal, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, localID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	now := uint64(time.Now().UnixMilli())
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encLocal[:],
		Version:      version,
		AddMembersPendingAdminApproval: []*groupspb.GroupChange_Actions_AddMemberPendingAdminApprovalAction{
			{
				Added: &groupspb.MemberPendingAdminApproval{
					UserId:       encLocal[:],
					Presentation: presentation,
					Timestamp:    now,
				},
			},
		},
	}
	return proto.Marshal(actions)
}

// InviteLinkPasswordBase64 returns the standard Base64 encoding used by the
// storage service for invite link passwords in HTTP paths and query params.
func InviteLinkPasswordBase64(password []byte) string {
	return base64.StdEncoding.EncodeToString(password)
}
