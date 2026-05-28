package group

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

// BuildLeaveActions constructs GroupChange.Actions for the local user to leave.
func BuildLeaveActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	localACI string,
	currentRevision uint32,
) ([]byte, error) {
	localID, err := libsignal.ParseServiceIDString(localACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildLeaveActions: local aci: %w", err)
	}
	encLocal, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, localID)
	if err != nil {
		return nil, err
	}
	encSource, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, localID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encSource[:],
		Version:      version,
		DeleteMembers: []*groupspb.GroupChange_Actions_DeleteMemberAction{
			{DeletedUserId: encLocal[:]},
		},
	}
	return proto.Marshal(actions)
}

// BuildModifyRoleActions constructs GroupChange.Actions to change one member's role.
func BuildModifyRoleActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actorACI string,
	targetACI string,
	role MemberRole,
	currentRevision uint32,
) ([]byte, error) {
	actorID, err := libsignal.ParseServiceIDString(actorACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildModifyRoleActions: actor: %w", err)
	}
	targetID, err := libsignal.ParseServiceIDString(targetACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildModifyRoleActions: target: %w", err)
	}
	encActor, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, actorID)
	if err != nil {
		return nil, err
	}
	encTarget, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, targetID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	r := groupspb.Member_Role(role)
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encActor[:],
		Version:      version,
		ModifyMemberRoles: []*groupspb.GroupChange_Actions_ModifyMemberRoleAction{
			{UserId: encTarget[:], Role: r},
		},
	}
	return proto.Marshal(actions)
}

// BuildRemoveMemberActions constructs GroupChange.Actions for an admin to
// remove another member from the group.
func BuildRemoveMemberActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actorACI string,
	targetACI string,
	currentRevision uint32,
) ([]byte, error) {
	actorID, err := libsignal.ParseServiceIDString(actorACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildRemoveMemberActions: actor: %w", err)
	}
	targetID, err := libsignal.ParseServiceIDString(targetACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildRemoveMemberActions: target: %w", err)
	}
	encActor, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, actorID)
	if err != nil {
		return nil, err
	}
	encTarget, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, targetID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encActor[:],
		Version:      version,
		DeleteMembers: []*groupspb.GroupChange_Actions_DeleteMemberAction{
			{DeletedUserId: encTarget[:]},
		},
	}
	return proto.Marshal(actions)
}

// BuildAddMemberActions constructs GroupChange.Actions to add a member using
// a profile-key credential presentation.
func BuildAddMemberActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actorACI string,
	presentation []byte,
	role MemberRole,
	currentRevision uint32,
) ([]byte, error) {
	if len(presentation) == 0 {
		return nil, fmt.Errorf("group.BuildAddMemberActions: empty presentation")
	}
	actorID, err := libsignal.ParseServiceIDString(actorACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildAddMemberActions: actor: %w", err)
	}
	encActor, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, actorID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	r := groupspb.Member_Role(role)
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encActor[:],
		Version:      version,
		AddMembers: []*groupspb.GroupChange_Actions_AddMemberAction{
			{
				Added: &groupspb.Member{
					Presentation: presentation,
					Role:         r,
				},
			},
		},
	}
	return proto.Marshal(actions)
}

// BuildModifyTitleActions constructs GroupChange.Actions to rename a group.
func BuildModifyTitleActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actorACI string,
	title string,
	currentRevision uint32,
) ([]byte, error) {
	actorID, err := libsignal.ParseServiceIDString(actorACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildModifyTitleActions: actor: %w", err)
	}
	encActor, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, actorID)
	if err != nil {
		return nil, err
	}
	randomness, err := libsignal.Randomness()
	if err != nil {
		return nil, err
	}
	titleCT, err := EncryptTitleBlob(secretParams, title, randomness)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encActor[:],
		Version:      version,
		ModifyTitle: &groupspb.GroupChange_Actions_ModifyTitleAction{
			Title: titleCT,
		},
	}
	return proto.Marshal(actions)
}

// BuildModifyDescriptionActions constructs GroupChange.Actions to change a
// group's description.
func BuildModifyDescriptionActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actorACI string,
	description string,
	currentRevision uint32,
) ([]byte, error) {
	actorID, err := libsignal.ParseServiceIDString(actorACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildModifyDescriptionActions: actor: %w", err)
	}
	encActor, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, actorID)
	if err != nil {
		return nil, err
	}
	randomness, err := libsignal.Randomness()
	if err != nil {
		return nil, err
	}
	descCT, err := EncryptDescriptionBlob(secretParams, description, randomness)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encActor[:],
		Version:      version,
		ModifyDescription: &groupspb.GroupChange_Actions_ModifyDescriptionAction{
			Description: descCT,
		},
	}
	return proto.Marshal(actions)
}

// BuildEnableInviteLinkActions constructs GroupChange.Actions to set an invite
// link password and who may join via the link.
func BuildEnableInviteLinkActions(
	secretParams [libsignal.GroupSecretParamsLen]byte,
	actorACI string,
	inviteLinkPassword []byte,
	access groupspb.AccessControl_AccessRequired,
	currentRevision uint32,
) ([]byte, error) {
	if len(inviteLinkPassword) == 0 {
		return nil, errors.New("group.BuildEnableInviteLinkActions: empty invite password")
	}
	actorID, err := libsignal.ParseServiceIDString(actorACI)
	if err != nil {
		return nil, fmt.Errorf("group.BuildEnableInviteLinkActions: actor: %w", err)
	}
	encActor, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, actorID)
	if err != nil {
		return nil, err
	}
	version := currentRevision + 1
	actions := &groupspb.GroupChange_Actions{
		SourceUserId: encActor[:],
		Version:      version,
		ModifyInviteLinkPassword: &groupspb.GroupChange_Actions_ModifyInviteLinkPasswordAction{
			InviteLinkPassword: append([]byte(nil), inviteLinkPassword...),
		},
		ModifyAddFromInviteLinkAccess: &groupspb.GroupChange_Actions_ModifyAddFromInviteLinkAccessControlAction{
			AddFromInviteLinkAccess: access,
		},
	}
	return proto.Marshal(actions)
}
