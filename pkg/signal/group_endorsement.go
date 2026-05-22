package signal

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// groupSendEndorsementCache holds processed GSE material for one group.
type groupSendEndorsementCache struct {
	response     []byte
	expiration   time.Time
	endorsements map[string][]byte // member ACI → endorsement blob
}

// groupSendTokenForRecipients returns a GroupSendFullToken authorizing delivery
// to the given member ACIs. Falls back to error if endorsements are missing or
// expired — callers may retry FetchGroup to refresh.
func (c *Client) groupSendTokenForRecipients(
	masterKeyHex string,
	recipientACIs []string,
) ([]byte, error) {
	c.groupEndorseMu.Lock()
	cache := c.groupEndorsements[masterKeyHex]
	c.groupEndorseMu.Unlock()
	if cache == nil || len(cache.response) == 0 {
		return nil, errors.New("signal: no cached group send endorsements (call FetchGroup first)")
	}
	if time.Now().After(cache.expiration) {
		return nil, errors.New("signal: group send endorsements expired")
	}

	var toCombine [][]byte
	for _, aci := range recipientACIs {
		endorsement, ok := cache.endorsements[aci]
		if !ok || len(endorsement) == 0 {
			return nil, fmt.Errorf("signal: no send endorsement for member %s", aci)
		}
		toCombine = append(toCombine, endorsement)
	}
	combined, err := libsignal.CombineGroupSendEndorsements(toCombine...)
	if err != nil {
		return nil, err
	}

	secretParams, err := c.groupSecretParamsForHexID(masterKeyHex)
	if err != nil {
		return nil, err
	}
	return libsignal.GroupSendEndorsementToFullToken(combined, secretParams, cache.expiration)
}

func (c *Client) storeGroupSendEndorsements(
	masterKeyHex string,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	response []byte,
	memberACIs []string,
) error {
	if len(response) == 0 || len(memberACIs) == 0 {
		return nil
	}
	serverParams, err := libsignal.ProductionServerPublicParams()
	if err != nil {
		return err
	}
	localUser, err := libsignal.ParseServiceIDString(c.acct.ACI)
	if err != nil {
		return err
	}

	memberIDs := make([]libsignal.ServiceIDFixedWidth, len(memberACIs))
	for i, aci := range memberACIs {
		memberIDs[i], err = libsignal.ParseServiceIDString(aci)
		if err != nil {
			return fmt.Errorf("parse member %s: %w", aci, err)
		}
	}

	expSec, err := libsignal.GroupSendEndorsementsResponseExpiration(response)
	if err != nil {
		return err
	}
	endorsements, err := libsignal.ReceiveGroupSendEndorsementsWithServiceIDs(
		response,
		memberIDs,
		localUser,
		secretParams,
		serverParams,
		time.Now(),
	)
	if err != nil {
		return err
	}
	if len(endorsements) != len(memberACIs) {
		return fmt.Errorf("signal: endorsement count %d != member count %d", len(endorsements), len(memberACIs))
	}

	byACI := make(map[string][]byte, len(memberACIs))
	for i, aci := range memberACIs {
		byACI[aci] = append([]byte(nil), endorsements[i]...)
	}

	c.groupEndorseMu.Lock()
	if c.groupEndorsements == nil {
		c.groupEndorsements = make(map[string]*groupSendEndorsementCache)
	}
	if c.groupSecretParams == nil {
		c.groupSecretParams = make(map[string][libsignal.GroupSecretParamsLen]byte)
	}
	c.groupEndorsements[masterKeyHex] = &groupSendEndorsementCache{
		response:     append([]byte(nil), response...),
		expiration:   time.Unix(int64(expSec), 0),
		endorsements: byACI,
	}
	c.groupSecretParams[masterKeyHex] = secretParams
	c.groupEndorseMu.Unlock()
	return nil
}

func (c *Client) groupSecretParamsForHexID(masterKeyHex string) ([libsignal.GroupSecretParamsLen]byte, error) {
	c.groupEndorseMu.Lock()
	defer c.groupEndorseMu.Unlock()
	if p, ok := c.groupSecretParams[masterKeyHex]; ok {
		return p, nil
	}
	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return [libsignal.GroupSecretParamsLen]byte{}, err
	}
	return libsignal.GroupSecretParamsFromMasterKey(masterKey)
}

// GroupSendTokenHeader returns the base64-encoded Group-Send-Token value for
// HTTP headers.
func GroupSendTokenHeader(fullToken []byte) string {
	return base64.StdEncoding.EncodeToString(fullToken)
}
