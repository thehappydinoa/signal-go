package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// PatchGroup issues PATCH /v2/groups/ with serialized GroupChange.Actions.
// The response body is a GroupChangeResponse protobuf.
func (c *Client) PatchGroup(ctx context.Context, groupsAuthHeader string, actions []byte) ([]byte, error) {
	if groupsAuthHeader == "" {
		return nil, errors.New("web.PatchGroup: empty authorization")
	}
	if len(actions) == 0 {
		return nil, errors.New("web.PatchGroup: empty actions")
	}
	var raw []byte
	if err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Path:   "/v2/groups/",
		Headers: http.Header{
			"Authorization": {groupsAuthHeader},
			"Content-Type":  {"application/x-protobuf"},
			"Accept":        {"application/x-protobuf"},
		},
		RawBody: actions,
		RawOut:  &raw,
	}); err != nil {
		return nil, fmt.Errorf("web.PatchGroup: %w", err)
	}
	return raw, nil
}
