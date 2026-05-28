package group

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

// NewGroupMember is one member entry for the initial PUT /v2/groups/ payload.
type NewGroupMember struct {
	Presentation []byte
	Role         MemberRole
}

// NewGroupPendingMember is a member awaiting profile-key credential before full
// membership, matching Signal-Android's invitee() path.
type NewGroupPendingMember struct {
	TargetACI string
	Role      MemberRole
}

// NewGroupMessageParams holds inputs for the initial encrypted Group protobuf.
type NewGroupMessageParams struct {
	SecretParams                [libsignal.GroupSecretParamsLen]byte
	PublicParams                [libsignal.GroupPublicParamsLen]byte
	Title                       string
	Description                 string
	DisappearingMessagesSeconds int
	SelfPresentation            []byte
	Members                     []NewGroupMember
	PendingMembers              []NewGroupPendingMember
}

// BuildNewGroupMessage constructs the encrypted Group protobuf for PUT
// /v2/groups/, matching Signal-Android's GroupsV2Operations.createNewGroup.
func BuildNewGroupMessage(p NewGroupMessageParams) ([]byte, error) {
	if len(p.SelfPresentation) == 0 {
		return nil, errors.New("group.BuildNewGroupMessage: empty self presentation")
	}
	randomness, err := libsignal.Randomness()
	if err != nil {
		return nil, err
	}
	titleCT, err := EncryptTitleBlob(p.SecretParams, p.Title, randomness)
	if err != nil {
		return nil, fmt.Errorf("group.BuildNewGroupMessage: title: %w", err)
	}
	timerCT, err := EncryptTimerBlob(p.SecretParams, p.DisappearingMessagesSeconds, randomness)
	if err != nil {
		return nil, fmt.Errorf("group.BuildNewGroupMessage: timer: %w", err)
	}
	var descCT []byte
	if p.Description != "" {
		descCT, err = EncryptDescriptionBlob(p.SecretParams, p.Description, randomness)
		if err != nil {
			return nil, fmt.Errorf("group.BuildNewGroupMessage: description: %w", err)
		}
	}

	members := make([]*groupspb.Member, 0, 1+len(p.Members))
	members = append(members, &groupspb.Member{
		Presentation: p.SelfPresentation,
		Role:         groupspb.Member_ADMINISTRATOR,
	})
	for _, m := range p.Members {
		if len(m.Presentation) == 0 {
			return nil, errors.New("group.BuildNewGroupMessage: member missing presentation")
		}
		members = append(members, &groupspb.Member{
			Presentation: m.Presentation,
			Role:         groupspb.Member_Role(m.Role),
		})
	}

	pending := make([]*groupspb.MemberPendingProfileKey, 0, len(p.PendingMembers))
	for _, pm := range p.PendingMembers {
		targetID, err := libsignal.ParseServiceIDString(pm.TargetACI)
		if err != nil {
			return nil, fmt.Errorf("group.BuildNewGroupMessage: pending member: %w", err)
		}
		encTarget, err := libsignal.GroupSecretParamsEncryptServiceID(p.SecretParams, targetID)
		if err != nil {
			return nil, err
		}
		pending = append(pending, &groupspb.MemberPendingProfileKey{
			Member: &groupspb.Member{
				UserId: encTarget[:],
				Role:   groupspb.Member_Role(pm.Role),
			},
		})
	}

	wire := &groupspb.Group{
		Version:                   0,
		PublicKey:                 append([]byte(nil), p.PublicParams[:]...),
		Title:                     titleCT,
		Description:               descCT,
		DisappearingMessagesTimer: timerCT,
		AccessControl: &groupspb.AccessControl{
			Attributes: groupspb.AccessControl_MEMBER,
			Members:    groupspb.AccessControl_MEMBER,
		},
		Members:                  members,
		MembersPendingProfileKey: pending,
	}
	return proto.Marshal(wire)
}
