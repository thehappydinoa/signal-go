package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// PutGroup issues PUT /v2/groups/ with a serialized initial Group protobuf.
// The response body is a GroupResponse protobuf.
func (c *Client) PutGroup(ctx context.Context, groupsAuthHeader string, groupWire []byte) ([]byte, error) {
	if groupsAuthHeader == "" {
		return nil, errors.New("web.PutGroup: empty authorization")
	}
	if len(groupWire) == 0 {
		return nil, errors.New("web.PutGroup: empty group")
	}
	var raw []byte
	if err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   "/v2/groups/",
		Headers: http.Header{
			"Authorization": {groupsAuthHeader},
			"Content-Type":  {"application/x-protobuf"},
			"Accept":        {"application/x-protobuf"},
		},
		RawBody: groupWire,
		RawOut:  &raw,
	}); err != nil {
		return nil, fmt.Errorf("web.PutGroup: %w", err)
	}
	return raw, nil
}
