package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// SendGroup delivers a text message to a Groups v2 chat identified by its
// 32-byte master key.
//
// SendGroup fetches current membership via [FetchGroup], distributes a
// sender-key message (SKDM) to each member over 1:1 sessions, then
// encrypts the group payload with the sender key and delivers it via
// PUT /v1/messages/multi_recipient.
//
// Authorization prefers a cached group send endorsement token from the
// most recent [FetchGroup] (Group-Send-Token header). If endorsements are
// unavailable, SendGroup falls back to the legacy combined UAK (XOR of
// member profile-key UAKs).
func (c *Client) SendGroup(ctx context.Context, masterKey []byte, text string) (Receipt, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return Receipt{}, fmt.Errorf("signal.SendGroup: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	if text == "" {
		return Receipt{}, errors.New("signal.SendGroup: empty body")
	}
	if c.webc == nil || c.stores == nil {
		return Receipt{}, errors.New("signal.SendGroup: Client was opened without send-side dependencies")
	}

	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return Receipt{}, fmt.Errorf("signal.SendGroup: fetch group: %w", err)
	}

	ts := uint64(time.Now().UnixMilli())
	groupIDHex := hex.EncodeToString(masterKey)
	contentBytes, err := buildGroupDataMessageContent(text, ts, masterKey, grp.Revision, c.expireTimerSeconds(groupIDHex))
	if err != nil {
		return Receipt{}, err
	}

	return c.deliverGroupPayload(ctx, masterKey, grp, contentBytes, ts, groupDeliveryOpts{
		online:         false,
		urgent:         true,
		distributeSKDM: true,
	})
}

type groupDeliveryOpts struct {
	online         bool
	urgent         bool
	distributeSKDM bool
}

func (c *Client) deliverGroupPayload(
	ctx context.Context,
	masterKey []byte,
	grp *Group,
	contentBytes []byte,
	ts uint64,
	opts groupDeliveryOpts,
) (Receipt, error) {
	if c.webc == nil || c.stores == nil {
		return Receipt{}, errors.New("signal: Client was opened without send-side dependencies")
	}

	distID, err := c.groupDistributionID(hex.EncodeToString(masterKey))
	if err != nil {
		return Receipt{}, err
	}

	local, err := libsignal.NewAddress(c.acct.ACI, c.acct.DeviceID)
	if err != nil {
		return Receipt{}, err
	}
	h := libsignal.NewStoreHandle(c.stores)
	defer h.Release()

	if opts.distributeSKDM {
		skdm, err := libsignal.CreateSenderKeyDistributionMessage(local, distID, h)
		if err != nil {
			return Receipt{}, fmt.Errorf("signal: create SKDM: %w", err)
		}
		skdmBytes, err := skdm.Serialize()
		if err != nil {
			return Receipt{}, err
		}
		if err := c.distributeSenderKeyToMembers(ctx, grp, skdmBytes); err != nil {
			return Receipt{}, err
		}
	}

	creds := c.credentials()
	payload, auth, err := c.buildGroupMultiRecipientPayload(ctx, creds, grp, masterKey, distID, local, h, padContent(contentBytes))
	if err != nil {
		return Receipt{}, err
	}

	if err := c.webc.SendMultiRecipientMessage(ctx, auth, payload, ts, opts.online, opts.urgent); err != nil {
		return Receipt{}, fmt.Errorf("signal: deliver group payload: %w", err)
	}

	return Receipt{
		Timestamp:    time.UnixMilli(int64(ts)),
		RecipientACI: grp.ID,
	}, nil
}

func (c *Client) distributeSenderKeyToMembers(ctx context.Context, grp *Group, skdmBytes []byte) error {
	for _, m := range grp.Members {
		if m.ACI == c.acct.ACI {
			continue
		}
		if err := c.sendSenderKeyDistribution(ctx, m.ACI, skdmBytes); err != nil {
			return fmt.Errorf("signal.SendGroup: distribute sender key to %s: %w", m.ACI, err)
		}
	}
	return nil
}

func (c *Client) buildGroupMultiRecipientPayload(
	ctx context.Context,
	creds web.Credentials,
	grp *Group,
	masterKey []byte,
	distID string,
	local *libsignal.Address,
	h *libsignal.StoreHandle,
	padded []byte,
) (payload []byte, auth web.MultiRecipientAuth, err error) {
	cipher, err := libsignal.GroupEncryptMessage(padded, local, distID, h)
	if err != nil {
		return nil, web.MultiRecipientAuth{}, fmt.Errorf("signal.SendGroup: encrypt: %w", err)
	}

	cert, err := c.cachedSenderCert(ctx, creds)
	if err != nil {
		return nil, web.MultiRecipientAuth{}, err
	}
	usmc, err := libsignal.NewUSMCForGroup(cipher, cert, masterKey)
	if err != nil {
		return nil, web.MultiRecipientAuth{}, fmt.Errorf("signal.SendGroup: build USMC: %w", err)
	}

	recipients, sessions, memberACIs, err := c.groupRecipientSessions(ctx, creds, grp)
	if err != nil {
		return nil, web.MultiRecipientAuth{}, err
	}
	if len(recipients) == 0 {
		return nil, web.MultiRecipientAuth{}, errors.New("signal.SendGroup: no recipient devices")
	}

	combinedUAK, uakErr := c.combinedMemberUAKs(memberACIs)
	var groupToken []byte
	if token, tokErr := c.groupSendTokenForRecipients(grp.ID, memberACIs); tokErr == nil {
		groupToken = token
	} else if uakErr != nil {
		return nil, web.MultiRecipientAuth{}, fmt.Errorf("signal.SendGroup: auth: endorsement: %w; combined UAK: %w", tokErr, uakErr)
	}

	payload, err = libsignal.MultiRecipientEncrypt(libsignal.MultiRecipientEncryptParams{
		Recipients:        recipients,
		RecipientSessions: sessions,
		Content:           usmc,
		Stores:            h,
	})
	if err != nil {
		return nil, web.MultiRecipientAuth{}, fmt.Errorf("signal.SendGroup: multi-recipient encrypt: %w", err)
	}

	if len(groupToken) > 0 {
		return payload, web.MultiRecipientAuth{GroupSendToken: groupToken}, nil
	}
	return payload, web.MultiRecipientAuth{CombinedUAK: combinedUAK}, nil
}

func (c *Client) groupDistributionID(groupIDHex string) (string, error) {
	c.groupDistMu.Lock()
	defer c.groupDistMu.Unlock()
	if c.groupDistID == nil {
		c.groupDistID = make(map[string]string)
	}
	if id, ok := c.groupDistID[groupIDHex]; ok {
		return id, nil
	}
	if c.groupDistStore != nil {
		if id, err := c.groupDistStore.LoadGroupDistributionID(groupIDHex); err == nil {
			c.groupDistID[groupIDHex] = id
			return id, nil
		}
	}
	id, err := libsignal.NewRandomUUID()
	if err != nil {
		return "", err
	}
	c.groupDistID[groupIDHex] = id
	if c.groupDistStore != nil {
		if err := c.groupDistStore.StoreGroupDistributionID(groupIDHex, id); err != nil {
			c.log.Warn("failed to persist group distribution id", "group", groupIDHex, "err", err)
		}
	}
	return id, nil
}

func (c *Client) sendSenderKeyDistribution(ctx context.Context, recipientACI string, skdm []byte) error {
	content := &sspb.Content{SenderKeyDistributionMessage: skdm}
	contentBytes, err := proto.Marshal(content)
	if err != nil {
		return err
	}
	ts := uint64(time.Now().UnixMilli())
	_, err = c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: true})
	return err
}

func buildGroupDataMessageContent(text string, tsMillis uint64, masterKey []byte, revision uint32, expireTimer uint32) ([]byte, error) {
	body := text
	timestamp := tsMillis
	rev := revision
	dm := &sspb.DataMessage{
		Body:      &body,
		Timestamp: &timestamp,
		GroupV2: &sspb.GroupContextV2{
			MasterKey: masterKey,
			Revision:  &rev,
		},
	}
	if expireTimer != 0 {
		dm.ExpireTimer = &expireTimer
	}
	content := &sspb.Content{
		Content: &sspb.Content_DataMessage{DataMessage: dm},
	}
	return proto.Marshal(content)
}

// groupRecipientSessions returns libsignal addresses and session records
// for every device of every group member except the local device.
func (c *Client) groupRecipientSessions(
	ctx context.Context,
	creds web.Credentials,
	grp *Group,
) ([]*libsignal.Address, []*libsignal.SessionRecord, []string, error) {
	var recipients []*libsignal.Address
	var sessions []*libsignal.SessionRecord
	var memberACIs []string

	for _, m := range grp.Members {
		if m.ACI == c.acct.ACI {
			continue
		}
		memberACIs = append(memberACIs, m.ACI)
		addrs, err := c.discoverAndEnsureSessions(ctx, creds, m.ACI)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, addr := range addrs {
			sessBlob, err := c.stores.LoadSession(addr)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("load session for %s: %w", addr, err)
			}
			sess, err := libsignal.DeserializeSessionRecord(sessBlob)
			if err != nil {
				return nil, nil, nil, err
			}
			libAddr, err := libsignal.NewAddress(addr.ServiceID, addr.DeviceID)
			if err != nil {
				return nil, nil, nil, err
			}
			recipients = append(recipients, libAddr)
			sessions = append(sessions, sess)
		}
	}
	return recipients, sessions, memberACIs, nil
}

func (c *Client) combinedMemberUAKs(memberACIs []string) ([]byte, error) {
	if len(memberACIs) == 0 {
		return nil, errors.New("signal.SendGroup: no members to combine UAKs for")
	}
	var combined [libsignal.AccessKeyLen]byte
	for _, aci := range memberACIs {
		c.ensureRecipientUAK(aci)
		c.mu.Lock()
		uak := c.knownUAKs[aci]
		c.mu.Unlock()
		if len(uak) != libsignal.AccessKeyLen {
			return nil, fmt.Errorf("signal.SendGroup: missing UAK for member %s (set profile key or fetch profile)", aci)
		}
		for i := 0; i < libsignal.AccessKeyLen; i++ {
			combined[i] ^= uak[i]
		}
	}
	return combined[:], nil
}
