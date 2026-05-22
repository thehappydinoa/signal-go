package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// DirectoryAuthCredentials are short-lived HTTP Basic credentials for CDSI,
// returned by GET /v2/directory/auth on the chat service.
type DirectoryAuthCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// FetchDirectoryAuth issues GET /v2/directory/auth on the chat service.
func (c *Client) FetchDirectoryAuth(ctx context.Context, creds Credentials) (*DirectoryAuthCredentials, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.FetchDirectoryAuth: credentials required")
	}
	var out DirectoryAuthCredentials
	if err := c.Do(ctx, Request{
		Method:      http.MethodGet,
		Path:        "/v2/directory/auth",
		Credentials: creds,
		Out:         &out,
	}); err != nil {
		return nil, fmt.Errorf("web.FetchDirectoryAuth: %w", err)
	}
	if out.Username == "" || out.Password == "" {
		return nil, errors.New("web.FetchDirectoryAuth: empty credentials in response")
	}
	return &out, nil
}
