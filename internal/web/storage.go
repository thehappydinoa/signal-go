package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"google.golang.org/protobuf/proto"

	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

// StorageAuthCredentials are short-lived HTTP Basic credentials for the
// storage service, returned by GET /v1/storage/auth on the chat service.
type StorageAuthCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// StorageManifestResult is the response from GET /v1/storage/manifest.
type StorageManifestResult struct {
	Manifest  *storagepb.StorageManifest
	Unchanged bool // HTTP 204 — localVersion is current
	Missing   bool // HTTP 404 — account has no manifest yet
}

// FetchStorageAuth issues GET /v1/storage/auth on the chat service.
func (c *Client) FetchStorageAuth(ctx context.Context, creds Credentials) (*StorageAuthCredentials, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.FetchStorageAuth: credentials required")
	}
	var out StorageAuthCredentials
	if err := c.Do(ctx, Request{
		Method:      http.MethodGet,
		Path:        "/v1/storage/auth",
		Credentials: creds,
		Out:         &out,
	}); err != nil {
		return nil, fmt.Errorf("web.FetchStorageAuth: %w", err)
	}
	if out.Username == "" || out.Password == "" {
		return nil, errors.New("web.FetchStorageAuth: empty storage credentials in response")
	}
	return &out, nil
}

// FetchStorageManifest issues GET /v1/storage/manifest or
// GET /v1/storage/manifest/version/{localVersion} on the storage service.
// greaterThanVersion selects the conditional endpoint; pass 0 for the latest
// manifest unconditionally.
func (c *Client) FetchStorageManifest(
	ctx context.Context,
	storageCreds Credentials,
	greaterThanVersion uint64,
) (*StorageManifestResult, error) {
	if storageCreds.Username == "" || storageCreds.Password == "" {
		return nil, errors.New("web.FetchStorageManifest: credentials required")
	}
	path := "/v1/storage/manifest"
	if greaterThanVersion > 0 {
		path = fmt.Sprintf("/v1/storage/manifest/version/%d", greaterThanVersion)
	}

	full, err := c.buildURL(path, nil)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("web.FetchStorageManifest: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", c.UserAgent)
	httpReq.Header.Set("X-Signal-Agent", c.UserAgent)
	httpReq.Header.Set("Authorization", storageCreds.Header())
	httpReq.Header.Set("Accept", "application/x-protobuf")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("web.FetchStorageManifest: do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("web.FetchStorageManifest: read body: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusNoContent:
		return &StorageManifestResult{Unchanged: true}, nil
	case http.StatusNotFound:
		return &StorageManifestResult{Missing: true}, nil
	case http.StatusOK:
		var manifest storagepb.StorageManifest
		if err := proto.Unmarshal(body, &manifest); err != nil {
			return nil, fmt.Errorf("web.FetchStorageManifest: unmarshal: %w", err)
		}
		return &StorageManifestResult{Manifest: &manifest}, nil
	default:
		return nil, &Error{StatusCode: resp.StatusCode, Status: resp.Status, Body: body}
	}
}

// ReadStorageRecords issues PUT /v1/storage/read with a ReadOperation
// protobuf body.
func (c *Client) ReadStorageRecords(
	ctx context.Context,
	storageCreds Credentials,
	readKeys [][]byte,
) (*storagepb.StorageItems, error) {
	if storageCreds.Username == "" || storageCreds.Password == "" {
		return nil, errors.New("web.ReadStorageRecords: credentials required")
	}
	if len(readKeys) == 0 {
		return &storagepb.StorageItems{}, nil
	}
	req := &storagepb.ReadOperation{ReadKey: readKeys}
	raw, err := proto.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("web.ReadStorageRecords: marshal: %w", err)
	}
	var respBody []byte
	if err := c.Do(ctx, Request{
		Method:      http.MethodPut,
		Path:        "/v1/storage/read",
		Credentials: storageCreds,
		RawBody:     raw,
		Headers: http.Header{
			"Content-Type": {"application/x-protobuf"},
			"Accept":       {"application/x-protobuf"},
		},
		RawOut: &respBody,
	}); err != nil {
		return nil, fmt.Errorf("web.ReadStorageRecords: %w", err)
	}
	var items storagepb.StorageItems
	if err := proto.Unmarshal(respBody, &items); err != nil {
		return nil, fmt.Errorf("web.ReadStorageRecords: unmarshal: %w", err)
	}
	return &items, nil
}
