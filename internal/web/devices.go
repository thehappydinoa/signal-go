package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/thehappydinoa/signal-go/internal/prekeys"
)

// AccountCapabilities mirrors the JSON keys Signal expects in the
// `capabilities` object of an account-attributes payload. We default to
// the conservative minimum and grow as the higher layers learn to handle
// each capability.
type AccountCapabilities struct {
	DeleteSync                      bool `json:"deleteSync"`
	VersionedExpirationTimer        bool `json:"versionedExpirationTimer"`
	SSRE2                           bool `json:"ssre2"`
	StorageServiceRecordKeyRotation bool `json:"storageServiceRecordKeyRotation"`
	// SPQR and profiles_v2 are required for all new linked devices per
	// Signal-Server DeviceCapability.CAPABILITIES_REQUIRED_FOR_NEW_DEVICES.
	Spqr       bool `json:"spqr"`
	ProfilesV2 bool `json:"profiles_v2"`
}

// DefaultCapabilities is the conservative capability set we advertise at
// link time. Bots and library consumers can override.
func DefaultCapabilities() AccountCapabilities {
	return AccountCapabilities{
		DeleteSync:                      true,
		VersionedExpirationTimer:        true,
		SSRE2:                           true,
		StorageServiceRecordKeyRotation: true,
		Spqr:                            true,
		ProfilesV2:                      true,
	}
}

// AccountAttributes is the JSON sub-object Signal's server stores against
// each device.
type AccountAttributes struct {
	FetchesMessages                bool                `json:"fetchesMessages"`
	RegistrationID                 uint32              `json:"registrationId"`
	PNIRegistrationID              uint32              `json:"pniRegistrationId"`
	Name                           string              `json:"name,omitempty"`
	Capabilities                   AccountCapabilities `json:"capabilities"`
	UnidentifiedAccessKey          string              `json:"unidentifiedAccessKey"`
	UnrestrictedUnidentifiedAccess bool                `json:"unrestrictedUnidentifiedAccess"`
	DiscoverableByPhoneNumber      bool                `json:"discoverableByPhoneNumber"`
}

// SignedECPreKey is the Curve25519 prekey envelope expected by the server.
type SignedECPreKey struct {
	KeyID     uint32 `json:"keyId"`
	PublicKey string `json:"publicKey"` // base64
	Signature string `json:"signature"` // base64
}

// SignedKEMPreKey is the Kyber prekey envelope.
type SignedKEMPreKey struct {
	KeyID     uint32 `json:"keyId"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
}

// LinkDeviceRequest is the body of PUT /v1/devices/link.
type LinkDeviceRequest struct {
	VerificationCode      string            `json:"verificationCode"`
	AccountAttributes     AccountAttributes `json:"accountAttributes"`
	ACISignedPreKey       SignedECPreKey    `json:"aciSignedPreKey"`
	PNISignedPreKey       SignedECPreKey    `json:"pniSignedPreKey"`
	ACIPqLastResortPreKey SignedKEMPreKey   `json:"aciPqLastResortPreKey"`
	PNIPqLastResortPreKey SignedKEMPreKey   `json:"pniPqLastResortPreKey"`
}

// LinkDeviceResponse is the JSON Signal returns on a successful link.
type LinkDeviceResponse struct {
	UUID     string `json:"uuid"` // = ACI
	DeviceID uint32 `json:"deviceId"`
	PNI      string `json:"pni"`
}

// LinkDevice issues PUT /v1/devices/link.
//
// Per upstream's HTTP-auth convention, this call is Basic-authenticated
// with username = the account phone number (E.164) from the ProvisionMessage
// and password = the new device's account password. The signed link token
// goes in req.VerificationCode (ProvisionMessage.provisioningCode); Signal
// validates that token and stores the password for subsequent auth.
func (c *Client) LinkDevice(ctx context.Context, number, password string, req LinkDeviceRequest) (*LinkDeviceResponse, error) {
	if number == "" {
		return nil, errors.New("web.LinkDevice: missing phone number")
	}
	if password == "" {
		return nil, errors.New("web.LinkDevice: missing password")
	}
	if req.VerificationCode == "" {
		return nil, errors.New("web.LinkDevice: missing verification code in request")
	}
	var resp LinkDeviceResponse
	if err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   "/v1/devices/link",
		Credentials: Credentials{
			Username: number,
			Password: password,
		},
		Body: req,
		Out:  &resp,
	}); err != nil {
		return nil, err
	}
	return &resp, nil
}

// SignedECPreKeyFrom translates a [prekeys.SignedPreKey] into the wire
// envelope.
func SignedECPreKeyFrom(p prekeys.SignedPreKey) SignedECPreKey {
	return SignedECPreKey{
		KeyID:     p.ID,
		PublicKey: base64.StdEncoding.EncodeToString(p.PublicKey),
		Signature: base64.StdEncoding.EncodeToString(p.Signature),
	}
}

// SignedKEMPreKeyFrom translates a [prekeys.LastResortKyberPreKey].
func SignedKEMPreKeyFrom(p prekeys.LastResortKyberPreKey) SignedKEMPreKey {
	return SignedKEMPreKey{
		KeyID:     p.ID,
		PublicKey: base64.StdEncoding.EncodeToString(p.PublicKey),
		Signature: base64.StdEncoding.EncodeToString(p.Signature),
	}
}
