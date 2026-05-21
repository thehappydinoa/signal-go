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
// ACI (UUID string). If no session exists with the recipient's primary
// device, Send transparently fetches a prekey bundle and establishes one
// before encrypting.
//
// Phase 4 ships single-device + basic-auth send: the message goes to the
// recipient's primary device (device 1) only, and the server learns
// we're the sender. Multi-device fan-out and sealed-sender land in the
// next iteration.
//
// On 409 / 410 from the server (mismatched / stale devices), Send returns
// the typed [*web.MismatchedDevicesError] / [*web.StaleDevicesError]
// without retrying — callers will get cleaner retry semantics once the
// Phase 4.1 fan-out work lands.
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

	// Phase 4 v1: target the recipient's device 1 (primary). Fan-out
	// across all devices needs a /v2/keys/<aci>/* response that lists
	// every device, and a session per device — defer that.
	addr := store.Address{ServiceID: recipientACI, DeviceID: 1}

	if err := c.ensureSession(ctx, addr); err != nil {
		return Receipt{}, err
	}

	envBytes, msgType, destRegID, err := c.encryptForDevice(addr, padded)
	if err != nil {
		return Receipt{}, err
	}

	creds := web.Credentials{
		Username: fmt.Sprintf("%s.%d", c.acct.ACI, c.acct.DeviceID),
		Password: c.acct.Password,
	}
	req := web.SendMessageRequest{
		Timestamp: uint64(ts),
		Urgent:    true,
		Messages: []web.MessageEnvelope{{
			Type:                      msgType,
			DestinationDeviceID:       addr.DeviceID,
			DestinationRegistrationID: destRegID,
			Content:                   base64.StdEncoding.EncodeToString(envBytes),
		}},
	}
	if _, err := c.webc.SendMessage(ctx, creds, recipientACI, req); err != nil {
		return Receipt{}, err
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

// ensureSession installs a Double Ratchet session for addr if none
// exists yet, by fetching the recipient's prekey bundle and running it
// through libsignal's processPreKeyBundle.
func (c *Client) ensureSession(ctx context.Context, addr store.Address) error {
	if _, err := c.stores.LoadSession(addr); err == nil {
		return nil // already have one
	} else if !errors.Is(err, store.ErrRecordNotFound) {
		return fmt.Errorf("signal.Send: load session: %w", err)
	}

	creds := web.Credentials{
		Username: fmt.Sprintf("%s.%d", c.acct.ACI, c.acct.DeviceID),
		Password: c.acct.Password,
	}
	bundleResp, err := c.webc.FetchPreKeyBundle(ctx, creds, addr.ServiceID, strconv.FormatUint(uint64(addr.DeviceID), 10))
	if err != nil {
		return fmt.Errorf("signal.Send: fetch prekey bundle: %w", err)
	}
	if len(bundleResp.Devices) == 0 {
		return fmt.Errorf("signal.Send: server returned no devices for %s", addr.ServiceID)
	}
	dev := bundleResp.Devices[0]

	bundle, err := buildLibsignalBundle(bundleResp.IdentityKey, dev)
	if err != nil {
		return fmt.Errorf("signal.Send: build prekey bundle: %w", err)
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
		return fmt.Errorf("signal.Send: process prekey bundle: %w", err)
	}
	return nil
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

// buildLibsignalBundle translates the JSON-decoded prekey bundle into a
// libsignal-owned *PreKeyBundle, doing the base64 decoding and key
// deserialization along the way.
func buildLibsignalBundle(identityKeyB64 string, dev struct {
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
},
) (*libsignal.PreKeyBundle, error) {
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
