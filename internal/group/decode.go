package group

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
)

// MemberRole mirrors groupspb.Member.Role for the public decode surface.
type MemberRole int32

const (
	MemberRoleUnknown       MemberRole = 0
	MemberRoleDefault       MemberRole = 1
	MemberRoleAdministrator MemberRole = 2
)

// Member is a decrypted group member entry.
type Member struct {
	ACI  string
	Role MemberRole
}

// State is a decrypted Groups v2 group snapshot.
type State struct {
	Title             string
	Description       string
	AvatarURL         string
	Revision          uint32
	Members           []Member
	AnnouncementsOnly bool
	Terminated        bool
}

// Admins returns ACIs with administrator role.
func (s *State) Admins() []string {
	var out []string
	for _, m := range s.Members {
		if m.Role == MemberRoleAdministrator {
			out = append(out, m.ACI)
		}
	}
	return out
}

// IsAdmin reports whether aci has administrator role in the group.
func (s *State) IsAdmin(aci string) bool {
	for _, m := range s.Members {
		if m.ACI == aci && m.Role == MemberRoleAdministrator {
			return true
		}
	}
	return false
}

// DecodeState decrypts a wire-format groupspb.Group using the supplied secret
// params derived from the group's master key.
func DecodeState(secretParams [libsignal.GroupSecretParamsLen]byte, wire *groupspb.Group) (*State, error) {
	if wire == nil {
		return nil, fmt.Errorf("group.DecodeState: nil group")
	}

	title, err := decryptTitle(secretParams, wire.GetTitle())
	if err != nil {
		return nil, fmt.Errorf("group.DecodeState: title: %w", err)
	}
	description, err := decryptDescription(secretParams, wire.GetDescription())
	if err != nil {
		return nil, fmt.Errorf("group.DecodeState: description: %w", err)
	}

	members := make([]Member, 0, len(wire.GetMembers()))
	for i, m := range wire.GetMembers() {
		dec, err := decodeMember(secretParams, m)
		if err != nil {
			return nil, fmt.Errorf("group.DecodeState: member %d: %w", i, err)
		}
		members = append(members, dec)
	}

	return &State{
		Title:             strings.TrimSpace(title),
		Description:       strings.TrimSpace(description),
		AvatarURL:         wire.GetAvatarUrl(),
		Revision:          wire.GetVersion(),
		Members:           members,
		AnnouncementsOnly: wire.GetAnnouncementsOnly(),
		Terminated:        wire.GetTerminated(),
	}, nil
}

func decodeMember(secretParams [libsignal.GroupSecretParamsLen]byte, m *groupspb.Member) (Member, error) {
	if m == nil {
		return Member{}, fmt.Errorf("nil member")
	}
	if len(m.GetPresentation()) > 0 {
		return Member{}, fmt.Errorf("profile-key presentation members not supported in bootstrap")
	}
	aci, err := decryptACI(secretParams, m.GetUserId())
	if err != nil {
		return Member{}, err
	}
	return Member{
		ACI:  aci,
		Role: MemberRole(m.GetRole()),
	}, nil
}

func decryptACI(secretParams [libsignal.GroupSecretParamsLen]byte, ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", fmt.Errorf("empty user id ciphertext")
	}
	id, err := libsignal.GroupSecretParamsDecryptServiceID(secretParams, ciphertext)
	if err != nil {
		return "", err
	}
	s, err := libsignal.ServiceIDString(id)
	if err != nil {
		return "", err
	}
	return s, nil
}

func decryptTitle(secretParams [libsignal.GroupSecretParamsLen]byte, ciphertext []byte) (string, error) {
	blob, err := decryptAttributeBlob(secretParams, ciphertext)
	if err != nil || blob == nil {
		return "", err
	}
	if t, ok := blob.GetContent().(*groupspb.GroupAttributeBlob_Title); ok {
		return t.Title, nil
	}
	return "", nil
}

func decryptDescription(secretParams [libsignal.GroupSecretParamsLen]byte, ciphertext []byte) (string, error) {
	blob, err := decryptAttributeBlob(secretParams, ciphertext)
	if err != nil || blob == nil {
		return "", err
	}
	if d, ok := blob.GetContent().(*groupspb.GroupAttributeBlob_DescriptionText); ok {
		return d.DescriptionText, nil
	}
	return "", nil
}

func decryptAttributeBlob(secretParams [libsignal.GroupSecretParamsLen]byte, ciphertext []byte) (*groupspb.GroupAttributeBlob, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	if len(ciphertext) < 29 {
		return &groupspb.GroupAttributeBlob{}, nil
	}
	plain, err := libsignal.GroupSecretParamsDecryptBlob(secretParams, ciphertext)
	if err != nil {
		return nil, err
	}
	var blob groupspb.GroupAttributeBlob
	if err := proto.Unmarshal(plain, &blob); err != nil {
		return &groupspb.GroupAttributeBlob{}, nil //nolint:nilerr // mirror Signal-Android: treat corrupt blobs as empty
	}
	return &blob, nil
}

// EncryptTitleBlob encrypts a title for test fixtures.
func EncryptTitleBlob(secretParams [libsignal.GroupSecretParamsLen]byte, title string, randomness [libsignal.ZKRandomnessLen]byte) ([]byte, error) {
	blob := &groupspb.GroupAttributeBlob{
		Content: &groupspb.GroupAttributeBlob_Title{Title: title},
	}
	plain, err := proto.Marshal(blob)
	if err != nil {
		return nil, fmt.Errorf("group.EncryptTitleBlob: marshal: %w", err)
	}
	return libsignal.GroupSecretParamsEncryptBlob(secretParams, plain, randomness)
}

// EncryptServiceID encrypts an ACI for test fixtures.
func EncryptServiceID(secretParams [libsignal.GroupSecretParamsLen]byte, aci string) ([]byte, error) {
	id, err := libsignal.ParseServiceIDString(aci)
	if err != nil {
		return nil, err
	}
	out, err := libsignal.GroupSecretParamsEncryptServiceID(secretParams, id)
	if err != nil {
		return nil, err
	}
	return out[:], nil
}
