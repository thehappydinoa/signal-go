package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// DefaultGroupsStorageURL is Signal's production Groups v2 storage endpoint.
const DefaultGroupsStorageURL = "https://storage.signal.org"

// GroupAuthCredential is one day of zkgroup auth credential material returned
// by GET /v1/certificate/auth/group.
type GroupAuthCredential struct {
	Credential     []byte `json:"credential"`
	RedemptionTime int64  `json:"redemptionTime"`
}

// GroupAuthCredentialsResponse is the JSON body of
// GET /v1/certificate/auth/group.
type GroupAuthCredentialsResponse struct {
	Credentials             []GroupAuthCredential `json:"credentials"`
	CallLinkAuthCredentials []GroupAuthCredential `json:"callLinkAuthCredentials"`
	PNI                     string                `json:"pni"`
}

// FetchGroupAuthCredentials issues GET /v1/certificate/auth/group for a
// seven-day window starting at startDaySeconds (UTC midnight epoch seconds).
func (c *Client) FetchGroupAuthCredentials(ctx context.Context, creds Credentials, startDaySeconds int64) (*GroupAuthCredentialsResponse, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.FetchGroupAuthCredentials: credentials required")
	}
	end := startDaySeconds + 7*86400
	q := url.Values{
		"redemptionStartSeconds": {fmt.Sprintf("%d", startDaySeconds)},
		"redemptionEndSeconds":   {fmt.Sprintf("%d", end)},
	}
	var resp GroupAuthCredentialsResponse
	if err := c.Do(ctx, Request{
		Method:      http.MethodGet,
		Path:        "/v1/certificate/auth/group",
		Query:       q,
		Credentials: creds,
		Out:         &resp,
	}); err != nil {
		return nil, fmt.Errorf("web.FetchGroupAuthCredentials: %w", err)
	}
	return &resp, nil
}

// FetchGroupState issues GET /v2/groups/ against the storage service using
// a Groups v2 authorization header (hex public params : hex presentation).
func (c *Client) FetchGroupState(ctx context.Context, groupsAuthHeader string) ([]byte, error) {
	if groupsAuthHeader == "" {
		return nil, errors.New("web.FetchGroupState: empty authorization")
	}
	var raw []byte
	if err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "/v2/groups/",
		Headers: http.Header{
			"Authorization": {groupsAuthHeader},
			"Accept":        {"application/x-protobuf"},
		},
		RawOut: &raw,
	}); err != nil {
		return nil, fmt.Errorf("web.FetchGroupState: %w", err)
	}
	return raw, nil
}

// CurrentDaySeconds returns today's UTC midnight as epoch seconds, matching
// Signal-Android's GroupsV2Authorization.currentDaySeconds().
func CurrentDaySeconds(now time.Time) int64 {
	utc := now.UTC()
	midnight := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return midnight.Unix()
}
