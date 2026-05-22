package signal

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/store"
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
	cache, err := c.groupSendEndorsementCache(masterKeyHex)
	if err != nil {
		return nil, err
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

func (c *Client) groupSendEndorsementCache(masterKeyHex string) (*groupSendEndorsementCache, error) {
	c.groupEndorseMu.Lock()
	cache := c.groupEndorsements[masterKeyHex]
	c.groupEndorseMu.Unlock()
	if cache != nil && len(cache.response) > 0 {
		return cache, nil
	}
	if c.groupEndorseStore == nil {
		return nil, errors.New("signal: no cached group send endorsements (call FetchGroup first)")
	}
	rec, err := c.groupEndorseStore.LoadGroupEndorsements(masterKeyHex)
	if err != nil {
		if errors.Is(err, store.ErrGroupEndorsementNotFound) {
			return nil, errors.New("signal: no cached group send endorsements (call FetchGroup first)")
		}
		return nil, fmt.Errorf("signal: load group endorsements: %w", err)
	}
	if time.Now().After(rec.Expiration) {
		return nil, errors.New("signal: group send endorsements expired")
	}
	cache = &groupSendEndorsementCache{
		response:     append([]byte(nil), rec.Response...),
		expiration:   rec.Expiration,
		endorsements: cloneEndorsementMap(rec.Endorsements),
	}
	c.groupEndorseMu.Lock()
	if c.groupEndorsements == nil {
		c.groupEndorsements = make(map[string]*groupSendEndorsementCache)
	}
	c.groupEndorsements[masterKeyHex] = cache
	c.groupEndorseMu.Unlock()
	return cache, nil
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
	expiration := time.Unix(int64(expSec), 0)

	c.groupEndorseMu.Lock()
	if c.groupEndorsements == nil {
		c.groupEndorsements = make(map[string]*groupSendEndorsementCache)
	}
	if c.groupSecretParams == nil {
		c.groupSecretParams = make(map[string][libsignal.GroupSecretParamsLen]byte)
	}
	c.groupEndorsements[masterKeyHex] = &groupSendEndorsementCache{
		response:     append([]byte(nil), response...),
		expiration:   expiration,
		endorsements: byACI,
	}
	c.groupSecretParams[masterKeyHex] = secretParams
	c.groupEndorseMu.Unlock()

	if c.groupEndorseStore != nil {
		rec := &store.GroupEndorsementRecord{
			Expiration:   expiration,
			Response:     append([]byte(nil), response...),
			Endorsements: cloneEndorsementMap(byACI),
		}
		if err := c.groupEndorseStore.StoreGroupEndorsements(masterKeyHex, rec); err != nil {
			c.log.Warn("persist group send endorsements failed", "group", masterKeyHex, "err", err)
		}
	}
	return nil
}

func (c *Client) deleteGroupSendEndorsements(masterKeyHex string) {
	c.groupEndorseMu.Lock()
	delete(c.groupEndorsements, masterKeyHex)
	delete(c.groupSecretParams, masterKeyHex)
	c.groupEndorseMu.Unlock()
	if c.groupEndorseStore != nil {
		if err := c.groupEndorseStore.DeleteGroupEndorsements(masterKeyHex); err != nil {
			c.log.Warn("delete persisted group endorsements failed", "group", masterKeyHex, "err", err)
		}
	}
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

func cloneEndorsementMap(in map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(in))
	for k, v := range in {
		out[k] = append([]byte(nil), v...)
	}
	return out
}
