package web

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
)

// ReserveUsernameHashResult is what the server returns from
// PUT /v1/accounts/username_hash/reserve: the single 32-byte hash it
// picked from the caller's priority-ordered list.
type ReserveUsernameHashResult struct {
	UsernameHash []byte
}

type reserveUsernameRequestBody struct {
	UsernameHashes []string `json:"usernameHashes"`
}

type reserveUsernameResponseBody struct {
	UsernameHash string `json:"usernameHash"`
}

// ReserveUsernameHash reserves one of hashes for creds's account. The
// server picks the first available entry and holds the reservation for
// about five minutes; call [ConfirmUsernameHash] within that window to
// finalize it. 409 means every candidate was already taken; 429 means
// rate-limited (Error.RetryAfter is populated).
func (c *Client) ReserveUsernameHash(
	ctx context.Context,
	creds Credentials,
	hashes [][]byte,
) (*ReserveUsernameHashResult, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.ReserveUsernameHash: credentials required")
	}
	if n := len(hashes); n == 0 || n > 20 {
		return nil, fmt.Errorf("web.ReserveUsernameHash: hash count out of range (%d; want 1..=20)", n)
	}
	body := reserveUsernameRequestBody{UsernameHashes: make([]string, len(hashes))}
	for i, h := range hashes {
		if len(h) != 32 {
			return nil, fmt.Errorf("web.ReserveUsernameHash: hash %d must be 32 bytes, got %d", i, len(h))
		}
		body.UsernameHashes[i] = base64.RawURLEncoding.EncodeToString(h)
	}
	var out reserveUsernameResponseBody
	if err := c.Do(ctx, Request{
		Method:      http.MethodPut,
		Path:        "/v1/accounts/username_hash/reserve",
		Credentials: creds,
		Body:        body,
		Out:         &out,
	}); err != nil {
		return nil, fmt.Errorf("web.ReserveUsernameHash: %w", err)
	}
	picked, err := base64.RawURLEncoding.DecodeString(out.UsernameHash)
	if err != nil {
		return nil, fmt.Errorf("web.ReserveUsernameHash: decode reserved hash: %w", err)
	}
	return &ReserveUsernameHashResult{UsernameHash: picked}, nil
}

// ConfirmUsernameHashResult is the body of the 200 response from
// PUT /v1/accounts/username_hash/confirm.
type ConfirmUsernameHashResult struct {
	UsernameHash       []byte
	UsernameLinkHandle string // UUID string
}

type confirmUsernameRequestBody struct {
	UsernameHash      string `json:"usernameHash"`
	ZKProof           string `json:"zkProof"`
	EncryptedUsername string `json:"encryptedUsername"`
}

type confirmUsernameResponseBody struct {
	UsernameHash       string `json:"usernameHash"`
	UsernameLinkHandle string `json:"usernameLinkHandle"`
}

// ConfirmUsernameHash finalizes the reservation created by
// [ReserveUsernameHash]. hash is the 32-byte hash the server picked;
// proof is libsignal's zero-knowledge proof of ownership; encryptedLink
// is the ciphertext half of a fresh username link (the entropy half
// stays client-side). The server also publishes the link and returns
// its UUID handle in one round trip.
//
// 409 = reservation lapsed or was never made; 410 = the username was
// taken by someone else in the gap between reserve and confirm; 422 =
// request body rejected (usually a bad zkProof).
func (c *Client) ConfirmUsernameHash(
	ctx context.Context,
	creds Credentials,
	hash, proof, encryptedLink []byte,
) (*ConfirmUsernameHashResult, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.ConfirmUsernameHash: credentials required")
	}
	if len(hash) != 32 {
		return nil, fmt.Errorf("web.ConfirmUsernameHash: hash must be 32 bytes, got %d", len(hash))
	}
	if len(proof) == 0 {
		return nil, errors.New("web.ConfirmUsernameHash: empty proof")
	}
	if len(encryptedLink) == 0 {
		return nil, errors.New("web.ConfirmUsernameHash: empty encrypted link")
	}
	body := confirmUsernameRequestBody{
		UsernameHash:      base64.RawURLEncoding.EncodeToString(hash),
		ZKProof:           base64.RawURLEncoding.EncodeToString(proof),
		EncryptedUsername: base64.RawURLEncoding.EncodeToString(encryptedLink),
	}
	var out confirmUsernameResponseBody
	if err := c.Do(ctx, Request{
		Method:      http.MethodPut,
		Path:        "/v1/accounts/username_hash/confirm",
		Credentials: creds,
		Body:        body,
		Out:         &out,
	}); err != nil {
		return nil, fmt.Errorf("web.ConfirmUsernameHash: %w", err)
	}
	returnedHash, err := base64.RawURLEncoding.DecodeString(out.UsernameHash)
	if err != nil {
		return nil, fmt.Errorf("web.ConfirmUsernameHash: decode hash: %w", err)
	}
	if out.UsernameLinkHandle == "" {
		return nil, errors.New("web.ConfirmUsernameHash: empty username link handle")
	}
	return &ConfirmUsernameHashResult{
		UsernameHash:       returnedHash,
		UsernameLinkHandle: out.UsernameLinkHandle,
	}, nil
}

// DeleteUsernameHash clears any username (and its link) from creds's
// account. The server returns 204 on success; this call returns nil in
// that case.
func (c *Client) DeleteUsernameHash(ctx context.Context, creds Credentials) error {
	if creds.Username == "" || creds.Password == "" {
		return errors.New("web.DeleteUsernameHash: credentials required")
	}
	if err := c.Do(ctx, Request{
		Method:      http.MethodDelete,
		Path:        "/v1/accounts/username_hash",
		Credentials: creds,
	}); err != nil {
		return fmt.Errorf("web.DeleteUsernameHash: %w", err)
	}
	return nil
}

// SetUsernameLinkResult is the response from PUT /v1/accounts/username_link.
type SetUsernameLinkResult struct {
	UsernameLinkHandle string // UUID string
}

type setUsernameLinkRequestBody struct {
	UsernameLinkEncryptedValue string `json:"usernameLinkEncryptedValue"`
	KeepLinkHandle             bool   `json:"keepLinkHandle"`
}

type setUsernameLinkResponseBody struct {
	UsernameLinkHandle string `json:"usernameLinkHandle"`
}

// SetUsernameLink publishes encryptedLink and returns the UUID handle
// under which the server stored it. keepHandle=true preserves the
// existing handle when rotating, so the shareable URL stays stable;
// false issues a fresh handle (useful after a compromise).
//
// 409 = the account doesn't have a username yet (call
// ConfirmUsernameHash first).
func (c *Client) SetUsernameLink(
	ctx context.Context,
	creds Credentials,
	encryptedLink []byte,
	keepHandle bool,
) (*SetUsernameLinkResult, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.SetUsernameLink: credentials required")
	}
	if len(encryptedLink) == 0 {
		return nil, errors.New("web.SetUsernameLink: empty encrypted link")
	}
	body := setUsernameLinkRequestBody{
		UsernameLinkEncryptedValue: base64.RawURLEncoding.EncodeToString(encryptedLink),
		KeepLinkHandle:             keepHandle,
	}
	var out setUsernameLinkResponseBody
	if err := c.Do(ctx, Request{
		Method:      http.MethodPut,
		Path:        "/v1/accounts/username_link",
		Credentials: creds,
		Body:        body,
		Out:         &out,
	}); err != nil {
		return nil, fmt.Errorf("web.SetUsernameLink: %w", err)
	}
	if out.UsernameLinkHandle == "" {
		return nil, errors.New("web.SetUsernameLink: empty username link handle")
	}
	return &SetUsernameLinkResult{UsernameLinkHandle: out.UsernameLinkHandle}, nil
}

// DeleteUsernameLink removes the account's published link but keeps the
// username itself. 204 on success.
func (c *Client) DeleteUsernameLink(ctx context.Context, creds Credentials) error {
	if creds.Username == "" || creds.Password == "" {
		return errors.New("web.DeleteUsernameLink: credentials required")
	}
	if err := c.Do(ctx, Request{
		Method:      http.MethodDelete,
		Path:        "/v1/accounts/username_link",
		Credentials: creds,
	}); err != nil {
		return fmt.Errorf("web.DeleteUsernameLink: %w", err)
	}
	return nil
}

// LookupUsernameHashResult is the response from
// GET /v1/accounts/username_hash/{b64url}.
type LookupUsernameHashResult struct {
	ACI string
}

type lookupUsernameHashResponseBody struct {
	UUID string `json:"uuid"`
}

// LookupUsernameHash resolves a 32-byte hash to the ACI UUID of the
// account that confirmed it, or returns a 404 Error if no account owns
// it. The server rejects this endpoint with 400 if the request carries
// an Authorization header, so it uses no credentials.
func (c *Client) LookupUsernameHash(ctx context.Context, hash []byte) (*LookupUsernameHashResult, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("web.LookupUsernameHash: hash must be 32 bytes, got %d", len(hash))
	}
	path := "/v1/accounts/username_hash/" + base64.RawURLEncoding.EncodeToString(hash)
	var out lookupUsernameHashResponseBody
	if err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   path,
		Out:    &out,
	}); err != nil {
		return nil, fmt.Errorf("web.LookupUsernameHash: %w", err)
	}
	if out.UUID == "" {
		return nil, errors.New("web.LookupUsernameHash: empty uuid in response")
	}
	return &LookupUsernameHashResult{ACI: out.UUID}, nil
}

// LookupUsernameLinkResult is the response from
// GET /v1/accounts/username_link/{uuid}: the ciphertext half of the
// link, which the caller decrypts with the entropy half shipped out of
// band (in the shareable URL).
type LookupUsernameLinkResult struct {
	UsernameLinkEncryptedValue []byte
}

type lookupUsernameLinkResponseBody struct {
	UsernameLinkEncryptedValue string `json:"usernameLinkEncryptedValue"`
}

// LookupUsernameLink fetches the ciphertext stored under a link handle.
// Enforced-unauthenticated like LookupUsernameHash. handle is a UUID
// string as returned by [ConfirmUsernameHash] or [SetUsernameLink].
func (c *Client) LookupUsernameLink(ctx context.Context, handle string) (*LookupUsernameLinkResult, error) {
	if handle == "" {
		return nil, errors.New("web.LookupUsernameLink: empty handle")
	}
	path := "/v1/accounts/username_link/" + handle
	var out lookupUsernameLinkResponseBody
	if err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   path,
		Out:    &out,
	}); err != nil {
		return nil, fmt.Errorf("web.LookupUsernameLink: %w", err)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(out.UsernameLinkEncryptedValue)
	if err != nil {
		return nil, fmt.Errorf("web.LookupUsernameLink: decode ciphertext: %w", err)
	}
	return &LookupUsernameLinkResult{UsernameLinkEncryptedValue: ciphertext}, nil
}
