package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/thehappydinoa/signal-go/internal/prekeys"
)

// AccountCapabilities mirrors the JSON keys Signal expects in the
// `capabilities` object of an account-attributes payload. The set matches
// what signal-cli / libsignal-service-java serialize
// (AccountAttributes.Capabilities).
//
// AttachmentBackfill and Spqr are REQUIRED for new linked devices per
// Signal-Server DeviceCapability.requireForNewDevices — omitting either makes
// PUT /v1/devices/link fail with HTTP 422 "Missing device capabilities".
type AccountCapabilities struct {
	Storage                  bool `json:"storage"`
	VersionedExpirationTimer bool `json:"versionedExpirationTimer"`
	AttachmentBackfill       bool `json:"attachmentBackfill"`
	Spqr                     bool `json:"spqr"`
}

// DefaultCapabilities is the capability set we advertise at link time,
// matching signal-cli. Bots and library consumers can override.
func DefaultCapabilities() AccountCapabilities {
	return AccountCapabilities{
		Storage:                  true,
		VersionedExpirationTimer: true,
		AttachmentBackfill:       true,
		Spqr:                     true,
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

// LinkDevice issues PUT /v1/devices/link over REST, matching signal-cli /
// libsignal-service-java (PushServiceSocket.finishNewDeviceRegistration).
//
// The request is Basic-authenticated with username = the account's e164
// number (from the ProvisionMessage) and password = our freshly generated
// account password. Upstream deliberately uses the e164 here and NOT the
// ACI/UUID — the server rejects a UUID identifier with HTTP 400. The server
// identifies which account to attach to from req.VerificationCode (the
// provisioning code, validated as a device-linking token) and stores the
// password for all subsequent device authentication.
func (c *Client) LinkDevice(ctx context.Context, number, password string, req LinkDeviceRequest) (*LinkDeviceResponse, error) {
	if number == "" {
		return nil, errors.New("web.LinkDevice: missing account number")
	}
	if password == "" {
		return nil, errors.New("web.LinkDevice: missing password")
	}
	if req.VerificationCode == "" {
		return nil, errors.New("web.LinkDevice: missing verification code")
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
