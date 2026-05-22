package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// FetchExpiringProfileKeyCredential retrieves a zkgroup expiring profile key
// credential for memberACI using their 32-byte profile key. The profile key
// may come from an inbound message, [SetRecipientProfileKey], or [FetchProfile].
func (c *Client) FetchExpiringProfileKeyCredential(ctx context.Context, memberACI string, profileKey []byte) ([libsignal.ExpiringProfileKeyCredentialLen]byte, error) {
	var out [libsignal.ExpiringProfileKeyCredentialLen]byte
	if memberACI == "" {
		return out, errors.New("signal.FetchExpiringProfileKeyCredential: empty member ACI")
	}
	if len(profileKey) == 0 {
		c.mu.Lock()
		profileKey = append([]byte(nil), c.knownProfileKeys[memberACI]...)
		c.mu.Unlock()
	}
	if err := libsignal.ValidateProfileKey(profileKey); err != nil {
		return out, fmt.Errorf("signal.FetchExpiringProfileKeyCredential: %w", err)
	}
	if c.webc == nil {
		return out, errors.New("signal.FetchExpiringProfileKeyCredential: Client was opened without send-side dependencies")
	}

	serverParams, err := libsignal.ProductionServerPublicParams()
	if err != nil {
		return out, err
	}
	user, err := libsignal.ParseServiceIDString(memberACI)
	if err != nil {
		return out, err
	}
	randomness, err := libsignal.Randomness()
	if err != nil {
		return out, err
	}
	reqCtx, err := libsignal.CreateProfileKeyCredentialRequestContext(serverParams, user, profileKey, randomness)
	if err != nil {
		return out, err
	}
	request, err := libsignal.ProfileKeyCredentialRequestFromContext(reqCtx)
	if err != nil {
		return out, err
	}
	uak, err := libsignal.DeriveAccessKey(profileKey)
	if err != nil {
		return out, err
	}
	version, err := libsignal.ProfileKeyVersion(profileKey, memberACI)
	if err != nil {
		return out, err
	}

	raw, err := c.webc.FetchExpiringProfileKeyCredential(ctx, memberACI, version, uak[:], request[:])
	if err != nil {
		return out, fmt.Errorf("signal.FetchExpiringProfileKeyCredential: fetch: %w", err)
	}
	credential, err := libsignal.ReceiveExpiringProfileKeyCredential(serverParams, reqCtx, raw, time.Now())
	if err != nil {
		return out, fmt.Errorf("signal.FetchExpiringProfileKeyCredential: receive: %w", err)
	}
	c.storeRecipientProfileKey(memberACI, profileKey)
	return credential, nil
}

// memberPresentationForAdd builds a profile-key credential presentation for
// adding memberACI to a group.
func (c *Client) memberPresentationForAdd(
	ctx context.Context,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	memberACI string,
	profileKey []byte,
) ([]byte, error) {
	credential, err := c.FetchExpiringProfileKeyCredential(ctx, memberACI, profileKey)
	if err != nil {
		return nil, err
	}
	serverParams, err := libsignal.ProductionServerPublicParams()
	if err != nil {
		return nil, err
	}
	randomness, err := libsignal.Randomness()
	if err != nil {
		return nil, err
	}
	return libsignal.CreateExpiringProfileKeyCredentialPresentation(serverParams, secretParams, credential, randomness)
}
