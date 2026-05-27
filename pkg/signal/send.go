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
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.Send: empty recipient")
	}
	if text == "" {
		return Receipt{}, errors.New("signal.Send: empty body")
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildDataMessageContent(text, ts)
	if err != nil {
		return Receipt{}, err
	}
	return c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: true})
}

// SendEdit sends an edit to a previously delivered 1:1 text message. The
// recipient must have received the original message at targetSentTimestamp
// (the sender-side millisecond timestamp from the original [Receipt] or
// [MessageEvent]).
func (c *Client) SendEdit(
	ctx context.Context,
	recipientACI string,
	newText string,
	targetSentTimestamp time.Time,
) (Receipt, error) {
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.SendEdit: empty recipient")
	}
	if newText == "" {
		return Receipt{}, errors.New("signal.SendEdit: empty body")
	}
	if targetSentTimestamp.IsZero() {
		return Receipt{}, errors.New("signal.SendEdit: zero target timestamp")
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildEditMessageContent(newText, ts, uint64(targetSentTimestamp.UnixMilli()))
	if err != nil {
		return Receipt{}, err
	}
	return c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: true})
}

// SendReceipt sends a receipt (delivery, read, or viewed) for one or
// more previously-received messages identified by their sender-side
// timestamps.
//
// Receipts are sent via the same PUT /v1/messages/{recipientACI} pipe as
// regular content, but with online=false, urgent=false, silent=true so
// the recipient doesn't get a fresh notification.
func (c *Client) SendReceipt(ctx context.Context, recipientACI string, kind ReceiptType, timestamps []time.Time) (Receipt, error) {
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.SendReceipt: empty recipient")
	}
	if len(timestamps) == 0 {
		return Receipt{}, errors.New("signal.SendReceipt: no timestamps")
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildReceiptContent(kind, timestamps)
	if err != nil {
		return Receipt{}, err
	}
	return c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: false, Silent: true})
}

// SendTyping sends a typing indicator (started or stopped). 1:1 only;
// group typing requires a group identifier and the Phase 5 send path.
//
// Typing indicators are urgent=false, online=true (don't store offline),
// silent=true.
func (c *Client) SendTyping(ctx context.Context, recipientACI string, action TypingAction) (Receipt, error) {
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.SendTyping: empty recipient")
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildTypingContent(action, ts, nil)
	if err != nil {
		return Receipt{}, err
	}
	return c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: false, Online: true, Silent: true})
}

// SendReaction sends a reaction (or removes one) to a previously-received
// message. The target is identified by (targetAuthorACI, targetTimestamp).
//
// Pass remove=true to clear a prior reaction; the emoji string in that
// case may be empty.
func (c *Client) SendReaction(
	ctx context.Context,
	recipientACI string,
	emoji string,
	targetAuthorACI string,
	targetTimestamp time.Time,
	remove bool,
) (Receipt, error) {
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.SendReaction: empty recipient")
	}
	if targetAuthorACI == "" {
		return Receipt{}, errors.New("signal.SendReaction: empty target author")
	}
	if targetTimestamp.IsZero() {
		return Receipt{}, errors.New("signal.SendReaction: zero target timestamp")
	}
	if !remove && emoji == "" {
		return Receipt{}, errors.New("signal.SendReaction: emoji required when not removing")
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildReactionContent(emoji, targetAuthorACI, targetTimestamp, remove, ts)
	if err != nil {
		return Receipt{}, err
	}
	return c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: true})
}

// deliveryOpts controls SendMessageRequest top-level flags and per-
// envelope flags that depend on the message kind (content vs receipt
// vs typing vs reaction).
type deliveryOpts struct {
	// Urgent prompts the recipient device to wake. True for content/
	// reactions; false for receipts/typing.
	Urgent bool
	// Online tells the server "drop if recipient offline". True for
	// typing indicators (no point storing them).
	Online bool
	// Silent suppresses recipient-side notifications for this envelope.
	// True for receipts/typing.
	Silent bool
}

// sendContent is the shared payload-delivery pipeline used by Send,
// SendEdit, SendReceipt, SendTyping, and SendReaction. It handles device
// discovery, session establishment, sealed-sender selection,
// 409/410 retry, and basic-auth fallback.
func (c *Client) sendContent(
	ctx context.Context,
	recipientACI string,
	contentBytes []byte,
	ts uint64,
	opts deliveryOpts,
) (Receipt, error) {
	if c.webc == nil || c.stores == nil {
		return Receipt{}, errors.New("signal.Send: Client was opened without send-side dependencies")
	}
	padded := padContent(contentBytes)
	creds := c.credentials()

	addrs, err := c.discoverAndEnsureSessions(ctx, creds, recipientACI)
	if err != nil {
		return Receipt{}, err
	}

	c.ensureRecipientUAK(recipientACI)

	c.mu.Lock()
	uak := c.knownUAKs[recipientACI]
	c.mu.Unlock()

	if len(uak) > 0 {
		if cert, certErr := c.cachedSenderCert(ctx, creds); certErr == nil {
			return c.deliverSealed(ctx, creds, recipientACI, addrs, padded, ts, opts, cert, uak)
		}
		c.log.Debug("sender cert unavailable; falling back to basic auth", "recipient", recipientACI)
	}
	return c.deliverBasicAuth(ctx, creds, recipientACI, addrs, padded, ts, opts)
}

// deliverBasicAuth sends using HTTP Basic auth. The server sees the sender's ACI.
func (c *Client) deliverBasicAuth(
	ctx context.Context,
	creds web.Credentials,
	recipientACI string,
	addrs []store.Address,
	padded []byte,
	ts uint64,
	opts deliveryOpts,
) (Receipt, error) {
	req, err := c.buildSendRequest(ts, addrs, padded, opts)
	if err != nil {
		return Receipt{}, err
	}
	if _, err := c.webc.SendMessage(ctx, creds, recipientACI, req); err != nil {
		addrs, err = c.handleSendError(ctx, creds, recipientACI, addrs, err)
		if err != nil {
			return Receipt{}, err
		}
		req, err = c.buildSendRequest(ts, addrs, padded, opts)
		if err != nil {
			return Receipt{}, err
		}
		if _, err := c.webc.SendMessage(ctx, creds, recipientACI, req); err != nil {
			return Receipt{}, fmt.Errorf("signal.Send: %w", err)
		}
	}
	return Receipt{Timestamp: time.UnixMilli(int64(ts)), RecipientACI: recipientACI}, nil
}

// deliverSealed sends using sealed-sender (USMC envelopes + UAK header).
// The server does not record the sender's ACI.
func (c *Client) deliverSealed(
	ctx context.Context,
	creds web.Credentials,
	recipientACI string,
	addrs []store.Address,
	padded []byte,
	ts uint64,
	opts deliveryOpts,
	cert *libsignal.SenderCertificate,
	uak []byte,
) (Receipt, error) {
	req, err := c.buildSealedSendRequest(ts, addrs, padded, cert, opts)
	if err != nil {
		return Receipt{}, err
	}
	if _, err := c.webc.SendMessageUnidentified(ctx, uak, recipientACI, req); err != nil {
		addrs, err = c.handleSendError(ctx, creds, recipientACI, addrs, err)
		if err != nil {
			return Receipt{}, err
		}
		req, err = c.buildSealedSendRequest(ts, addrs, padded, cert, opts)
		if err != nil {
			return Receipt{}, err
		}
		if _, err := c.webc.SendMessageUnidentified(ctx, uak, recipientACI, req); err != nil {
			return Receipt{}, fmt.Errorf("signal.Send: %w", err)
		}
	}
	return Receipt{Timestamp: time.UnixMilli(int64(ts)), RecipientACI: recipientACI}, nil
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

// handleSendError interprets an HTTP 409 or 410 response and fixes up
// the session state and address list for a retry. It returns the updated
// address list. Non-device-set errors are returned unchanged.
//
// The caller is responsible for rebuilding the send request with the
// updated addresses.
func (c *Client) handleSendError(
	ctx context.Context,
	creds web.Credentials,
	recipientACI string,
	addrs []store.Address,
	sendErr error,
) ([]store.Address, error) {
	var mde *web.MismatchedDevicesError
	var sde *web.StaleDevicesError
	switch {
	case errors.As(sendErr, &mde):
		// Drop sessions for devices the recipient no longer has.
		for _, devID := range mde.ExtraDevices {
			if err := c.stores.DeleteSession(store.Address{ServiceID: recipientACI, DeviceID: devID}); err != nil {
				return nil, fmt.Errorf("signal.Send: delete extra device %d: %w", devID, err)
			}
		}
		addrs = filterOutDevices(addrs, mde.ExtraDevices)

		// Establish sessions for newly-added devices.
		for _, devID := range mde.MissingDevices {
			if err := c.fetchAndProcessBundle(ctx, creds, recipientACI, devID); err != nil {
				return nil, fmt.Errorf("signal.Send: missing device %d: %w", devID, err)
			}
			addrs = append(addrs, store.Address{ServiceID: recipientACI, DeviceID: devID})
		}

		// Update the in-memory device cache.
		c.mu.Lock()
		c.knownDevices[recipientACI] = addressDeviceIDs(addrs)
		c.mu.Unlock()

	case errors.As(sendErr, &sde):
		// Registration IDs changed: drop sessions and re-fetch bundles.
		for _, devID := range sde.StaleDevices {
			addr := store.Address{ServiceID: recipientACI, DeviceID: devID}
			if err := c.stores.DeleteSession(addr); err != nil {
				return nil, fmt.Errorf("signal.Send: delete stale session %d: %w", devID, err)
			}
			if err := c.fetchAndProcessBundle(ctx, creds, recipientACI, devID); err != nil {
				return nil, fmt.Errorf("signal.Send: stale device %d: %w", devID, err)
			}
		}
		// addrs is unchanged — same set of devices, new sessions.

	default:
		return nil, sendErr
	}
	return addrs, nil
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

// cachedSenderCert returns a valid sender certificate, fetching one from
// /v1/certificate/delivery if the cached copy is absent or within 5 minutes
// of its expiry timestamp.
func (c *Client) cachedSenderCert(ctx context.Context, creds web.Credentials) (*libsignal.SenderCertificate, error) {
	c.certMu.Lock()
	defer c.certMu.Unlock()
	const headroom = 5 * time.Minute
	if c.senderCert != nil && time.Now().Before(c.certExpiry.Add(-headroom)) {
		return c.senderCert, nil
	}
	certBytes, err := c.webc.FetchSenderCertificate(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("signal.Send: fetch sender cert: %w", err)
	}
	cert, err := libsignal.DeserializeSenderCertificate(certBytes)
	if err != nil {
		return nil, fmt.Errorf("signal.Send: parse sender cert: %w", err)
	}
	expiry, err := cert.Expiration()
	if err != nil {
		return nil, fmt.Errorf("signal.Send: sender cert expiry: %w", err)
	}
	c.senderCert = cert
	c.certExpiry = expiry
	return cert, nil
}

// sealedEncryptForDevice wraps one device's Double Ratchet ciphertext in a
// sealed-sender USMC. Returns (USMC bytes, recipient registration ID, error).
func (c *Client) sealedEncryptForDevice(addr store.Address, padded []byte, cert *libsignal.SenderCertificate) ([]byte, uint32, error) {
	remote, err := libsignal.NewAddress(addr.ServiceID, addr.DeviceID)
	if err != nil {
		return nil, 0, err
	}
	local, err := libsignal.NewAddress(c.acct.ACI, c.acct.DeviceID)
	if err != nil {
		return nil, 0, err
	}
	h := libsignal.NewStoreHandle(c.stores)
	defer h.Release()
	cipher, err := libsignal.EncryptMessage(padded, remote, local, h, time.Now())
	if err != nil {
		return nil, 0, fmt.Errorf("signal.Send: sealed encrypt: %w", err)
	}
	usmc, err := libsignal.NewUSMC(cipher, cert)
	if err != nil {
		return nil, 0, fmt.Errorf("signal.Send: build USMC: %w", err)
	}
	usmcBytes, err := usmc.Serialize()
	if err != nil {
		return nil, 0, fmt.Errorf("signal.Send: serialize USMC: %w", err)
	}

	sessBlob, err := c.stores.LoadSession(addr)
	if err != nil {
		return nil, 0, fmt.Errorf("signal.Send: load session post-seal-encrypt: %w", err)
	}
	sess, err := libsignal.DeserializeSessionRecord(sessBlob)
	if err != nil {
		return nil, 0, err
	}
	regID, err := sess.RemoteRegistrationID()
	if err != nil {
		return nil, 0, err
	}
	return usmcBytes, regID, nil
}

// buildSealedSendRequest assembles a [web.SendMessageRequest] using sealed-
// sender USMC envelopes (type 6 = UNIDENTIFIED_SENDER).
func (c *Client) buildSealedSendRequest(ts uint64, addrs []store.Address, padded []byte, cert *libsignal.SenderCertificate, opts deliveryOpts) (web.SendMessageRequest, error) {
	msgs := make([]web.MessageEnvelope, 0, len(addrs))
	for _, addr := range addrs {
		usmcBytes, regID, err := c.sealedEncryptForDevice(addr, padded, cert)
		if err != nil {
			return web.SendMessageRequest{}, err
		}
		msgs = append(msgs, web.MessageEnvelope{
			Type:                      web.CiphertextTypeUnidentifiedSender,
			DestinationDeviceID:       addr.DeviceID,
			DestinationRegistrationID: regID,
			Content:                   base64.StdEncoding.EncodeToString(usmcBytes),
			Silent:                    opts.Silent,
		})
	}
	return web.SendMessageRequest{
		Timestamp: ts,
		Urgent:    opts.Urgent,
		Online:    opts.Online,
		Messages:  msgs,
	}, nil
}

// buildSendRequest encrypts padded content for every address and
// assembles the [web.SendMessageRequest].
func (c *Client) buildSendRequest(ts uint64, addrs []store.Address, padded []byte, opts deliveryOpts) (web.SendMessageRequest, error) {
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
			Silent:                    opts.Silent,
		})
	}
	return web.SendMessageRequest{
		Timestamp: ts,
		Urgent:    opts.Urgent,
		Online:    opts.Online,
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
	return body, libsignalToWireType(typ), regID, nil
}

// libsignalToWireType translates libsignal's CiphertextMessageType to the
// type field Signal's HTTP API expects. The only mismatch is Whisper:
// libsignal uses 2 but the wire protocol uses 1.
func libsignalToWireType(t libsignal.CiphertextMessageType) web.CiphertextType {
	if t == libsignal.CiphertextWhisper {
		return web.CiphertextTypeWhisper // 1, not libsignal's 2
	}
	return web.CiphertextType(t)
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

func buildEditMessageContent(newText string, editTSMillis, targetSentMillis uint64) ([]byte, error) {
	body := newText
	ts := editTSMillis
	targetTS := targetSentMillis
	em := &sspb.EditMessage{
		TargetSentTimestamp: &targetTS,
		DataMessage: &sspb.DataMessage{
			Body:      &body,
			Timestamp: &ts,
		},
	}
	content := &sspb.Content{
		Content: &sspb.Content_EditMessage{EditMessage: em},
	}
	return proto.Marshal(content)
}

// buildReceiptContent wraps the receipt kind + acknowledged-message
// timestamps in a signalservice.Content containing a ReceiptMessage.
func buildReceiptContent(kind ReceiptType, timestamps []time.Time) ([]byte, error) {
	var rt sspb.ReceiptMessage_Type
	switch kind {
	case ReceiptDelivery:
		rt = sspb.ReceiptMessage_DELIVERY
	case ReceiptRead:
		rt = sspb.ReceiptMessage_READ
	case ReceiptViewed:
		rt = sspb.ReceiptMessage_VIEWED
	default:
		return nil, fmt.Errorf("signal.SendReceipt: unknown receipt type %d", int(kind))
	}
	tsMillis := make([]uint64, len(timestamps))
	for i, t := range timestamps {
		if t.IsZero() {
			return nil, fmt.Errorf("signal.SendReceipt: zero timestamp at index %d", i)
		}
		tsMillis[i] = uint64(t.UnixMilli())
	}
	rm := &sspb.ReceiptMessage{
		Type:      &rt,
		Timestamp: tsMillis,
	}
	content := &sspb.Content{
		Content: &sspb.Content_ReceiptMessage{ReceiptMessage: rm},
	}
	return proto.Marshal(content)
}

// buildTypingContent wraps a TypingMessage in Content. groupID is
// reserved for the Phase 5 group-typing path; pass nil for 1:1.
func buildTypingContent(action TypingAction, tsMillis uint64, groupID []byte) ([]byte, error) {
	var ta sspb.TypingMessage_Action
	switch action {
	case TypingStarted:
		ta = sspb.TypingMessage_STARTED
	case TypingStopped:
		ta = sspb.TypingMessage_STOPPED
	default:
		return nil, fmt.Errorf("signal.SendTyping: unknown typing action %d", int(action))
	}
	timestamp := tsMillis
	tm := &sspb.TypingMessage{
		Action:    &ta,
		Timestamp: &timestamp,
		GroupId:   groupID,
	}
	content := &sspb.Content{
		Content: &sspb.Content_TypingMessage{TypingMessage: tm},
	}
	return proto.Marshal(content)
}

// buildReactionContent wraps a Reaction inside a DataMessage envelope
// (Signal carries reactions inside DataMessage, not as a top-level
// Content variant).
func buildReactionContent(emoji, targetAuthorACI string, targetTS time.Time, remove bool, tsMillis uint64) ([]byte, error) {
	emo := emoji
	target := targetAuthorACI
	rem := remove
	targetTSMillis := uint64(targetTS.UnixMilli())
	timestamp := tsMillis
	r := &sspb.DataMessage_Reaction{
		Emoji:               &emo,
		Remove:              &rem,
		TargetAuthorAci:     &target,
		TargetSentTimestamp: &targetTSMillis,
	}
	dm := &sspb.DataMessage{
		Timestamp: &timestamp,
		Reaction:  r,
	}
	content := &sspb.Content{
		Content: &sspb.Content_DataMessage{DataMessage: dm},
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
