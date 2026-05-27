package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// VersionedProfileResponse is the JSON body of
// GET /v1/profile/{aci}/{profileKeyVersion}. Encrypted fields are
// standard base64; callers decrypt them with the profile key.
type VersionedProfileResponse struct {
	Name                           string `json:"name"`
	About                          string `json:"about"`
	AboutEmoji                     string `json:"aboutEmoji"`
	Avatar                         string `json:"avatar"`
	PaymentAddress                 string `json:"paymentAddress"`
	PhoneNumberShare               string `json:"phoneNumberSharing"`
	UnidentifiedAccess             string `json:"unidentifiedAccess"`
	UnrestrictedUnidentifiedAccess bool   `json:"unrestrictedUnidentifiedAccess"`
}

// FetchVersionedProfile issues GET /v1/profile/{aci}/{profileKeyVersion}
// with the Unidentified-Access-Key header set to base64(uak). The server
// returns encrypted profile fields when the UAK matches the recipient's
// profile key.
func (c *Client) FetchVersionedProfile(
	ctx context.Context,
	aci, profileKeyVersion string,
	uak []byte,
) (*VersionedProfileResponse, error) {
	return c.fetchVersionedProfile(ctx, aci, profileKeyVersion, uak, "")
}

// FetchExpiringProfileKeyCredential issues GET /v1/profile/{aci}/{version}/{request}
// with credentialType=expiringProfileKey and returns the credential response
// bytes consumed by libsignal. Signal currently returns a JSON profile payload
// that includes a base64 `credential` field; older deployments may return raw
// credential bytes directly, so this method supports both shapes.
func (c *Client) FetchExpiringProfileKeyCredential(
	ctx context.Context,
	aci, profileKeyVersion string,
	uak, credentialRequest []byte,
) ([]byte, error) {
	if len(credentialRequest) != 329 {
		return nil, fmt.Errorf("web.FetchExpiringProfileKeyCredential: request length %d, want 329", len(credentialRequest))
	}
	raw, err := c.fetchVersionedProfileRaw(ctx, aci, profileKeyVersion, uak, credentialRequest)
	if err != nil {
		return nil, err
	}

	var profileResp struct {
		Credential []byte `json:"credential"`
	}
	if err := json.Unmarshal(raw, &profileResp); err == nil {
		if len(profileResp.Credential) == 0 {
			return nil, errors.New("web.FetchExpiringProfileKeyCredential: JSON response missing credential field")
		}
		return profileResp.Credential, nil
	}

	// Legacy shape: endpoint returned the raw credential bytes directly.
	return raw, nil
}

func (c *Client) fetchVersionedProfile(
	ctx context.Context,
	aci, profileKeyVersion string,
	uak []byte,
	credentialType string,
) (*VersionedProfileResponse, error) {
	if aci == "" {
		return nil, errors.New("web.FetchVersionedProfile: aci required")
	}
	if profileKeyVersion == "" {
		return nil, errors.New("web.FetchVersionedProfile: profileKeyVersion required")
	}
	if len(uak) != 16 {
		return nil, fmt.Errorf("web.FetchVersionedProfile: uak length %d, want 16", len(uak))
	}
	path := fmt.Sprintf("/v1/profile/%s/%s", aci, profileKeyVersion)
	if credentialType != "" {
		path += "?credentialType=" + credentialType
	}
	var resp VersionedProfileResponse
	if err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   path,
		Headers: http.Header{
			"Unidentified-Access-Key": {base64.StdEncoding.EncodeToString(uak)},
		},
		Out: &resp,
	}); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) fetchVersionedProfileRaw(
	ctx context.Context,
	aci, profileKeyVersion string,
	uak, credentialRequest []byte,
) ([]byte, error) {
	if aci == "" {
		return nil, errors.New("web.FetchExpiringProfileKeyCredential: aci required")
	}
	if profileKeyVersion == "" {
		return nil, errors.New("web.FetchExpiringProfileKeyCredential: profileKeyVersion required")
	}
	if len(uak) != 16 {
		return nil, fmt.Errorf("web.FetchExpiringProfileKeyCredential: uak length %d, want 16", len(uak))
	}
	path := fmt.Sprintf("/v1/profile/%s/%s/%x?credentialType=expiringProfileKey", aci, profileKeyVersion, credentialRequest)
	var raw []byte
	if err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   path,
		Headers: http.Header{
			"Unidentified-Access-Key": {base64.StdEncoding.EncodeToString(uak)},
		},
		RawOut: &raw,
	}); err != nil {
		return nil, err
	}
	return raw, nil
}

// DecodeBase64Field decodes a nullable base64 profile field from the wire
// JSON. Empty strings decode to nil without error.
func DecodeBase64Field(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, nil
	}
	return DecodeBase64(b64)
}
