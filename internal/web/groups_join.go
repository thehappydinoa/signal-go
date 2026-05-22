package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// FetchGroupJoinInfo issues GET /v2/groups/join/{inviteLinkPassword} against
// the storage service. inviteLinkPasswordBase64 is the standard Base64-encoded
// 16-byte invite password (see [group.InviteLinkPasswordBase64]).
func (c *Client) FetchGroupJoinInfo(ctx context.Context, groupsAuthHeader, inviteLinkPasswordBase64 string) ([]byte, error) {
	if groupsAuthHeader == "" {
		return nil, errors.New("web.FetchGroupJoinInfo: empty authorization")
	}
	path := "/v2/groups/join/"
	if inviteLinkPasswordBase64 != "" {
		path += inviteLinkPasswordBase64
	}
	var raw []byte
	if err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   path,
		Headers: http.Header{
			"Authorization": {groupsAuthHeader},
			"Accept":        {"application/x-protobuf"},
		},
		RawOut: &raw,
	}); err != nil {
		return nil, fmt.Errorf("web.FetchGroupJoinInfo: %w", err)
	}
	return raw, nil
}

// PatchGroupWithInvite issues PATCH /v2/groups/?inviteLinkPassword=... with
// serialized GroupChange.Actions. Used when joining via invite link.
func (c *Client) PatchGroupWithInvite(ctx context.Context, groupsAuthHeader, inviteLinkPasswordBase64 string, actions []byte) ([]byte, error) {
	if groupsAuthHeader == "" {
		return nil, errors.New("web.PatchGroupWithInvite: empty authorization")
	}
	if len(actions) == 0 {
		return nil, errors.New("web.PatchGroupWithInvite: empty actions")
	}
	req := Request{
		Method: http.MethodPatch,
		Path:   "/v2/groups/",
		Headers: http.Header{
			"Authorization": {groupsAuthHeader},
			"Content-Type":  {"application/x-protobuf"},
			"Accept":        {"application/x-protobuf"},
		},
		RawBody: actions,
		RawOut:  new([]byte),
	}
	if inviteLinkPasswordBase64 != "" {
		req.Query = url.Values{
			"inviteLinkPassword": {inviteLinkPasswordBase64},
		}
	}
	var raw []byte
	req.RawOut = &raw
	if err := c.Do(ctx, req); err != nil {
		return nil, fmt.Errorf("web.PatchGroupWithInvite: %w", err)
	}
	return raw, nil
}
