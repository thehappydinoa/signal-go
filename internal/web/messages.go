package web

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ---------- GET /v2/keys/{serviceID}/{deviceID} ----------

// FetchPreKeyResponse is the JSON Signal returns when we fetch a
// recipient's prekey bundle. The `devices` array contains one entry per
// active device — usually 1 or 2. We always request a single device
// (`*` for all is also supported by the server, but we'd need to wire
// fan-out separately).
//
// Field names match the wire format. Signal returns base64 in every
// `publicKey` / `signature` slot.
type FetchPreKeyResponse struct {
	IdentityKey string `json:"identityKey"`
	Devices     []struct {
		DeviceID       uint32 `json:"deviceId"`
		RegistrationID uint32 `json:"registrationId"`
		SignedPreKey   struct {
			KeyID     uint32 `json:"keyId"`
			PublicKey string `json:"publicKey"`
			Signature string `json:"signature"`
		} `json:"signedPreKey"`
		PqPreKey *struct {
			KeyID     uint32 `json:"keyId"`
			PublicKey string `json:"publicKey"`
			Signature string `json:"signature"`
		} `json:"pqPreKey"`
		PreKey *struct {
			KeyID     uint32 `json:"keyId"`
			PublicKey string `json:"publicKey"`
		} `json:"preKey"`
	} `json:"devices"`
}

// FetchPreKeyBundle issues GET /v2/keys/{serviceID}/{deviceIDOrStar}.
// deviceIDOrStar is "*" (all devices) or a stringified device id.
func (c *Client) FetchPreKeyBundle(ctx context.Context, creds Credentials, serviceID, deviceIDOrStar string) (*FetchPreKeyResponse, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.FetchPreKeyBundle: credentials required")
	}
	if serviceID == "" {
		return nil, errors.New("web.FetchPreKeyBundle: serviceID required")
	}
	if deviceIDOrStar == "" {
		deviceIDOrStar = "*"
	}
	var resp FetchPreKeyResponse
	if err := c.Do(ctx, Request{
		Method:      http.MethodGet,
		Path:        fmt.Sprintf("/v2/keys/%s/%s", serviceID, deviceIDOrStar),
		Credentials: creds,
		Out:         &resp,
	}); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ---------- PUT /v1/messages/{recipientACI} ----------

// CiphertextType mirrors libsignal's `CiphertextMessageType` for the
// envelope JSON the server expects. Only the values we actually emit
// are listed; receive-side dispatch lives in pkg/signal.
type CiphertextType uint8

const (
	// CiphertextTypeWhisper is a regular Double Ratchet message (after
	// session establishment).
	CiphertextTypeWhisper CiphertextType = 1
	// CiphertextTypePreKey is the first message in a new session — it
	// carries the initial X3DH/PQXDH bundle.
	CiphertextTypePreKey CiphertextType = 3
	// CiphertextTypeSenderKey is for group v2 messages (Phase 5).
	CiphertextTypeSenderKey CiphertextType = 7
	// CiphertextTypePlaintext is libsignal's "plaintext content" for
	// retries / sync errors.
	CiphertextTypePlaintext CiphertextType = 8
)

// MessageEnvelope is one device's worth of ciphertext destined for the
// recipient. PUT /v1/messages/{recipientACI} takes an array — one per
// destination device.
type MessageEnvelope struct {
	Type                      CiphertextType `json:"type"`
	DestinationDeviceID       uint32         `json:"destinationDeviceId"`
	DestinationRegistrationID uint32         `json:"destinationRegistrationId"`
	// Content is base64 of the serialized CiphertextMessage from libsignal.
	Content string `json:"content"`
	// Silent suppresses notifications on the recipient. Set true for
	// receipts and other background sync messages.
	Silent bool `json:"silent"`
}

// SendMessageRequest is the body of PUT /v1/messages/{recipientACI}.
type SendMessageRequest struct {
	// Timestamp is the sender's wall clock in milliseconds since epoch.
	// Used as the message's primary identifier across the conversation.
	Timestamp uint64 `json:"timestamp"`
	// Online suppresses storage if the recipient is offline (used by
	// receipts and typing indicators). Default false.
	Online bool `json:"online"`
	// Urgent prompts the recipient's device to wake. Default true for
	// content messages.
	Urgent   bool              `json:"urgent"`
	Messages []MessageEnvelope `json:"messages"`
}

// SendMessageResponse is the success-case body Signal returns.
type SendMessageResponse struct {
	NeedsSync bool `json:"needsSync"`
}

// MismatchedDevicesError is returned for HTTP 409. Either we tried to
// send to a device the recipient no longer has (extraDevices) or we
// missed a device they've since added (missingDevices). The caller
// re-fetches bundles for the missing devices and retries.
type MismatchedDevicesError struct {
	MissingDevices []uint32 `json:"missingDevices"`
	ExtraDevices   []uint32 `json:"extraDevices"`
}

// Error implements error.
func (e *MismatchedDevicesError) Error() string {
	return fmt.Sprintf("web: mismatched devices (missing=%v extra=%v)", e.MissingDevices, e.ExtraDevices)
}

// StaleDevicesError is returned for HTTP 410. The recipient's
// registration id for one of our targeted devices changed since we
// last fetched their bundle — drop the affected sessions and re-fetch.
type StaleDevicesError struct {
	StaleDevices []uint32 `json:"staleDevices"`
}

// Error implements error.
func (e *StaleDevicesError) Error() string {
	return fmt.Sprintf("web: stale devices %v", e.StaleDevices)
}

// SendMessage issues PUT /v1/messages/{recipientACI}.
//
// On HTTP 409 returns [*MismatchedDevicesError]; on 410
// [*StaleDevicesError]. Both wrap the underlying *web.Error so callers
// can still see the raw status. The caller is responsible for handling
// these by re-fetching bundles and retrying.
//
// Phase 4 ships with basic-auth send (the server sees our ACI as the
// sender). Sealed-sender uses a different header (Unidentified-Access-Key)
// and a different endpoint shape; it's a planned Phase 4.1 enhancement.
func (c *Client) SendMessage(ctx context.Context, creds Credentials, recipientServiceID string, req SendMessageRequest) (*SendMessageResponse, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.SendMessage: credentials required")
	}
	if recipientServiceID == "" {
		return nil, errors.New("web.SendMessage: recipient required")
	}
	if len(req.Messages) == 0 {
		return nil, errors.New("web.SendMessage: no messages")
	}
	var resp SendMessageResponse
	err := c.Do(ctx, Request{
		Method:      http.MethodPut,
		Path:        "/v1/messages/" + recipientServiceID,
		Credentials: creds,
		Body:        req,
		Out:         &resp,
	})
	if err != nil {
		return nil, mapSendError(err)
	}
	return &resp, nil
}

// mapSendError converts a generic *web.Error into a typed
// MismatchedDevicesError / StaleDevicesError when the server's JSON
// body matches one of those shapes.
func mapSendError(err error) error {
	var werr *Error
	if !errors.As(err, &werr) {
		return err
	}
	switch werr.StatusCode {
	case http.StatusConflict:
		var mde MismatchedDevicesError
		if jerr := json.Unmarshal(werr.Body, &mde); jerr == nil && (len(mde.MissingDevices) > 0 || len(mde.ExtraDevices) > 0) {
			return &mde
		}
	case http.StatusGone:
		var sde StaleDevicesError
		if jerr := json.Unmarshal(werr.Body, &sde); jerr == nil && len(sde.StaleDevices) > 0 {
			return &sde
		}
	}
	return err
}

// ---------- helpers ----------

// DecodeBase64 decodes a base64 string from a server response. Both
// standard and url-safe encodings appear in Signal payloads.
func DecodeBase64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
