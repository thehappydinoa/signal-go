package signal

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// Send delivers a 1:1 text message to the recipient identified by their
// ACI (UUID string).
//
// On the first send to a recipient, Send fetches the full device list via
// GET /v2/keys/{aci}/* and establishes a Double Ratchet session with every
// device that does not already have one. Subsequent sends reuse the cached
// device list and existing sessions.
//
// If the server replies with HTTP 409 (mismatched devices) or 410 (stale
// devices), Send transparently refreshes the affected sessions and retries
// once. If the retry also fails the error is returned to the caller.
func (c *Client) Send(ctx context.Context, recipientACI, text string) (Receipt, error) {
	if c.webc == nil || c.stores == nil {
		return Receipt{}, errors.New("signal.Send: Client was opened without send-side dependencies")
	}
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.Send: empty recipient")
	}
	if text == "" {
		return Receipt{}, errors.New("signal.Send: empty body")
	}

	ts := time.Now().UnixMilli()
	contentBytes, err := buildDataMessageContent(text, uint64(ts))
	if err != nil {
		return Receipt{}, err
	}
	padded := padContent(contentBytes)
	creds := c.credentials()

	addrs, err := c.discoverAndEnsureSessions(ctx, creds, recipientACI)
	if err != nil {
		return Receipt{}, err
	}

	req, err := c.buildSendRequest(uint64(ts), addrs, padded)
	if err != nil {
		return Receipt{}, err
	}

	if _, err := c.webc.SendMessage(ctx, creds, recipientACI, req); err != nil {
		addrs, req, err = c.handleSendError(ctx, creds, recipientACI, addrs, padded, uint64(ts), err)
		if err != nil {
			return Receipt{}, err
		}
		if _, err := c.webc.SendMessage(ctx, creds, recipientACI, req); err != nil {
			return Receipt{}, fmt.Errorf("signal.Send: %w", err)
		}
	}

	return Receipt{Timestamp: time.UnixMilli(ts), RecipientACI: recipientACI}, nil
}

// Receipt is what [Client.Send] returns on success. The timestamp is
// also the message's primary identifier across the conversation — the
// recipient will quote it in delivery / read receipts.
type Receipt struct {
	Timestamp    time.Time
	RecipientACI string
}

// credentials returns basic-auth credentials for the local device.
func (c *Client) credentials() web.Credentials {
	return web.Credentials{
		Username: fmt.Sprintf("%s.%d", c.acct.ACI, c.acct.DeviceID),
		Password: c.acct.Password,
	}
}

// discoverAndEnsureSessions returns the current device address list for
// the recipient, establishing sessions for any devices that do not yet
// have one.
//
// On the first call for a given ACI it fetches GET /v2/keys/{aci}/* and
// caches the device ID set. Subsequent calls use the in-memory cache and
// only fetch individual bundles for devices whose sessions have been
// deleted (e.g. by a prior 410 response).
func (c *Client) discoverAndEnsureSessions(ctx context.Context, creds web.Credentials, recipientACI string) ([]store.Address, error) {
	c.mu.Lock()
	deviceIDs := c.knownDevices[recipientACI]
	c.mu.Unlock()

	if deviceIDs == nil {
		// First send to this recipient: discover the full device set.
		resp, err := c.webc.FetchPreKeyBundle(ctx, creds, recipientACI, "*")
		if err != nil {
			return nil, fmt.Errorf("signal.Send: discover devices: %w", err)
		}
		if len(resp.Devices) == 0 {
			return nil, fmt.Errorf("signal.Send: no devices for %s", recipientACI)
		}
		deviceIDs = make([]uint32, 0, len(resp.Devices))
		for _, dev := range resp.Devices {
			addr := store.Address{ServiceID: recipientACI, DeviceID: dev.DeviceID}
			if _, serr := c.stores.LoadSession(addr); serr != nil {
				if !errors.Is(serr, store.ErrRecordNotFound) {
					return nil, fmt.Errorf("signal.Send: load session: %w", serr)
				}
				if err := c.processBundle(resp.IdentityKey, dev, addr); err != nil {
					return nil, err
				}
			}
			deviceIDs = append(deviceIDs, dev.DeviceID)
		}
		c.mu.Lock()
		if c.knownDevices == nil {
			c.knownDevices = make(map[string][]uint32)
		}
		c.knownDevices[recipientACI] = deviceIDs
		c.mu.Unlock()
	} else {
		// Subsequent send: only fetch bundles for devices whose sessions
		// were deleted (e.g. because the last send returned HTTP 410).
		for _, devID := range deviceIDs {
			addr := store.Address{ServiceID: recipientACI, DeviceID: devID}
			if _, serr := c.stores.LoadSession(addr); serr != nil {
				if !errors.Is(serr, store.ErrRecordNotFound) {
					return nil, fmt.Errorf("signal.Send: load session: %w", serr)
				}
				if err := c.fetchAndProcessBundle(ctx, creds, recipientACI, devID); err != nil {
					return nil, err
				}
			}
		}
	}

	addrs := make([]store.Address, len(deviceIDs))
	for i, id := range deviceIDs {
		addrs[i] = store.Address{ServiceID: recipientACI, DeviceID: id}
	}
	return addrs, nil
}

// handleSendError interprets an HTTP 409 or 410 response, fixes up the
// session state and address list, and returns the corrected values ready
// for a single retry. Non-device-set errors are returned unchanged.
func (c *Client) handleSendError(
	ctx context.Context,
	creds web.Credentials,
	recipientACI string,
	addrs []store.Address,
	padded []byte,
	ts uint64,
	sendErr error,
) ([]store.Address, web.SendMessageRequest, error) {
	var mde *web.MismatchedDevicesError
	var sde *web.StaleDevicesError
	switch {
	case errors.As(sendErr, &mde):
		// Drop sessions for devices Bob no longer has.
		for _, devID := range mde.ExtraDevices {
			if err := c.stores.DeleteSession(store.Address{ServiceID: recipientACI, DeviceID: devID}); err != nil {
				return nil, web.SendMessageRequest{}, fmt.Errorf("signal.Send: delete extra device %d: %w", devID, err)
			}
		}
		addrs = filterOutDevices(addrs, mde.ExtraDevices)

		// Establish sessions for Bob's new devices.
		for _, devID := range mde.MissingDevices {
			if err := c.fetchAndProcessBundle(ctx, creds, recipientACI, devID); err != nil {
				return nil, web.SendMessageRequest{}, fmt.Errorf("signal.Send: missing device %d: %w", devID, err)
			}
			addrs = append(addrs, store.Address{ServiceID: recipientACI, DeviceID: devID})
		}

		// Update the cache to reflect the corrected device set.
		c.mu.Lock()
		c.knownDevices[recipientACI] = addressDeviceIDs(addrs)
		c.mu.Unlock()

	case errors.As(sendErr, &sde):
		// Registration IDs changed: drop sessions and re-fetch bundles.
		for _, devID := range sde.StaleDevices {
			addr := store.Address{ServiceID: recipientACI, DeviceID: devID}
			if err := c.stores.DeleteSession(addr); err != nil {
				return nil, web.SendMessageRequest{}, fmt.Errorf("signal.Send: delete stale session %d: %w", devID, err)
			}
			if err := c.fetchAndProcessBundle(ctx, creds, recipientACI, devID); err != nil {
				return nil, web.SendMessageRequest{}, fmt.Errorf("signal.Send: stale device %d: %w", devID, err)
			}
		}
		// addrs is unchanged — same set of devices, new sessions.

	default:
		return nil, web.SendMessageRequest{}, sendErr
	}

	req, err := c.buildSendRequest(ts, addrs, padded)
	if err != nil {
		return nil, web.SendMessageRequest{}, err
	}
	return addrs, req, nil
}

// fetchAndProcessBundle fetches a single device's prekey bundle and
// processes it to establish a Double Ratchet session.
func (c *Client) fetchAndProcessBundle(ctx context.Context, creds web.Credentials, recipientACI string, devID uint32) error {
	resp, err := c.webc.FetchPreKeyBundle(ctx, creds, recipientACI, strconv.FormatUint(uint64(devID), 10))
	if err != nil {
		return fmt.Errorf("fetch bundle for device %d: %w", devID, err)
	}
	if len(resp.Devices) == 0 {
		return fmt.Errorf("no bundle returned for device %d", devID)
	}
	return c.processBundle(resp.IdentityKey, resp.Devices[0], store.Address{ServiceID: recipientACI, DeviceID: devID})
}

// processBundle converts a wire-format prekey bundle into a libsignal
// PreKeyBundle and calls ProcessPreKeyBundle to establish a session.
func (c *Client) processBundle(identityKeyB64 string, dev web.BundleDevice, addr store.Address) error {
	bundle, err := buildLibsignalBundle(identityKeyB64, dev)
	if err != nil {
		return fmt.Errorf("signal.Send: build bundle for %s: %w", addr, err)
	}
	remote, err := libsignal.NewAddress(addr.ServiceID, addr.DeviceID)
	if err != nil {
		return err
	}
	local, err := libsignal.NewAddress(c.acct.ACI, c.acct.DeviceID)
	if err != nil {
		return err
	}
	h := libsignal.NewStoreHandle(c.stores)
	defer h.Release()
	if err := libsignal.ProcessPreKeyBundle(bundle, remote, local, h, time.Now()); err != nil {
		return fmt.Errorf("signal.Send: process bundle for %s: %w", addr, err)
	}
	return nil
}

// buildSendRequest encrypts padded content for every address and
// assembles the [web.SendMessageRequest].
func (c *Client) buildSendRequest(ts uint64, addrs []store.Address, padded []byte) (web.SendMessageRequest, error) {
	msgs := make([]web.MessageEnvelope, 0, len(addrs))
	for _, addr := range addrs {
		envBytes, msgType, destRegID, err := c.encryptForDevice(addr, padded)
		if err != nil {
			return web.SendMessageRequest{}, err
		}
		msgs = append(msgs, web.MessageEnvelope{
			Type:                      msgType,
			DestinationDeviceID:       addr.DeviceID,
			DestinationRegistrationID: destRegID,
			Content:                   base64.StdEncoding.EncodeToString(envBytes),
		})
	}
	return web.SendMessageRequest{
		Timestamp: ts,
		Urgent:    true,
		Messages:  msgs,
	}, nil
}

// filterOutDevices removes addresses whose DeviceID appears in remove.
func filterOutDevices(addrs []store.Address, remove []uint32) []store.Address {
	if len(remove) == 0 {
		return addrs
	}
	skip := make(map[uint32]bool, len(remove))
	for _, id := range remove {
		skip[id] = true
	}
	out := make([]store.Address, 0, len(addrs))
	for _, a := range addrs {
		if !skip[a.DeviceID] {
			out = append(out, a)
		}
	}
	return out
}

// addressDeviceIDs extracts the DeviceID from each address.
func addressDeviceIDs(addrs []store.Address) []uint32 {
	ids := make([]uint32, len(addrs))
	for i, a := range addrs {
		ids[i] = a.DeviceID
	}
	return ids
}

// encryptForDevice runs libsignal's session-cipher encrypt and returns
// the (serialized ciphertext, wire-format type, peer's reg-id) we need
// to put on the wire.
func (c *Client) encryptForDevice(addr store.Address, padded []byte) ([]byte, web.CiphertextType, uint32, error) {
	remote, err := libsignal.NewAddress(addr.ServiceID, addr.DeviceID)
	if err != nil {
		return nil, 0, 0, err
	}
	local, err := libsignal.NewAddress(c.acct.ACI, c.acct.DeviceID)
	if err != nil {
		return nil, 0, 0, err
	}
	h := libsignal.NewStoreHandle(c.stores)
	defer h.Release()
	cipher, err := libsignal.EncryptMessage(padded, remote, local, h, time.Now())
	if err != nil {
		return nil, 0, 0, fmt.Errorf("signal.Send: encrypt: %w", err)
	}
	typ, err := cipher.Type()
	if err != nil {
		return nil, 0, 0, err
	}
	body, err := cipher.Serialize()
	if err != nil {
		return nil, 0, 0, err
	}

	// Recover the peer's registration id from the session record so the
	// server can authenticate the destination device.
	sessBlob, err := c.stores.LoadSession(addr)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("signal.Send: load session post-encrypt: %w", err)
	}
	sess, err := libsignal.DeserializeSessionRecord(sessBlob)
	if err != nil {
		return nil, 0, 0, err
	}
	regID, err := sess.RemoteRegistrationID()
	if err != nil {
		return nil, 0, 0, err
	}
	return body, web.CiphertextType(typ), regID, nil
}

// buildDataMessageContent wraps text + ts in a signalservice.Content
// containing a DataMessage, and returns the marshalled bytes.
func buildDataMessageContent(text string, tsMillis uint64) ([]byte, error) {
	body := text
	timestamp := tsMillis
	content := &sspb.Content{
		Content: &sspb.Content_DataMessage{
			DataMessage: &sspb.DataMessage{
				Body:      &body,
				Timestamp: &timestamp,
			},
		},
	}
	return proto.Marshal(content)
}

// padContent applies Signal's per-message padding scheme: append a
// terminator byte 0x80, then 0x00 bytes to the next multiple of 160.
// The recipient strips padding after decrypt (everything from the last
// 0x80 onward) before parsing the protobuf.
//
// This is type-hiding only; the cipher itself doesn't need it.
func padContent(body []byte) []byte {
	// Same formula signalmeow uses: next multiple of 160 >= len(body)+1.
	target := 160 * (1 + len(body)/160)
	out := make([]byte, target)
	copy(out, body)
	out[len(body)] = 0x80
	return out
}

// buildLibsignalBundle translates a [web.BundleDevice] into a
// libsignal-owned *PreKeyBundle, doing base64 decoding and key
// deserialization along the way.
func buildLibsignalBundle(identityKeyB64 string, dev web.BundleDevice) (*libsignal.PreKeyBundle, error) {
	idBytes, err := web.DecodeBase64(identityKeyB64)
	if err != nil {
		return nil, fmt.Errorf("identity key: %w", err)
	}
	identityKey, err := libsignal.DeserializePublicKey(idBytes)
	if err != nil {
		return nil, fmt.Errorf("identity key: %w", err)
	}

	if dev.SignedPreKey.PublicKey == "" {
		return nil, errors.New("missing signed prekey")
	}
	spkBytes, err := web.DecodeBase64(dev.SignedPreKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("signed prekey: %w", err)
	}
	spkPub, err := libsignal.DeserializePublicKey(spkBytes)
	if err != nil {
		return nil, fmt.Errorf("signed prekey: %w", err)
	}
	spkSig, err := web.DecodeBase64(dev.SignedPreKey.Signature)
	if err != nil {
		return nil, fmt.Errorf("signed prekey sig: %w", err)
	}

	if dev.PqPreKey == nil || dev.PqPreKey.PublicKey == "" {
		return nil, errors.New("server returned bundle without Kyber prekey (PQXDH required)")
	}
	kpkBytes, err := web.DecodeBase64(dev.PqPreKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("kyber prekey: %w", err)
	}
	kpkPub, err := libsignal.DeserializeKyberPublicKey(kpkBytes)
	if err != nil {
		return nil, fmt.Errorf("kyber prekey: %w", err)
	}
	kpkSig, err := web.DecodeBase64(dev.PqPreKey.Signature)
	if err != nil {
		return nil, fmt.Errorf("kyber prekey sig: %w", err)
	}

	params := libsignal.PreKeyBundleParams{
		RegistrationID:        dev.RegistrationID,
		DeviceID:              dev.DeviceID,
		SignedPreKeyID:        dev.SignedPreKey.KeyID,
		SignedPreKeyPublic:    spkPub,
		SignedPreKeySignature: spkSig,
		IdentityKey:           identityKey,
		KyberPreKeyID:         dev.PqPreKey.KeyID,
		KyberPreKeyPublic:     kpkPub,
		KyberPreKeySignature:  kpkSig,
	}
	if dev.PreKey != nil && dev.PreKey.PublicKey != "" {
		// Server *may* omit the one-time prekey if the recipient has no
		// remaining one-time keys; libsignal accepts a bundle without
		// one (PreKeyID = 0 and PreKeyPublic = nil).
		pkBytes, err := web.DecodeBase64(dev.PreKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("one-time prekey: %w", err)
		}
		pkPub, err := libsignal.DeserializePublicKey(pkBytes)
		if err != nil {
			return nil, fmt.Errorf("one-time prekey: %w", err)
		}
		params.PreKeyID = dev.PreKey.KeyID
		params.PreKeyPublic = pkPub
	}
	return libsignal.NewPreKeyBundle(params)
}
