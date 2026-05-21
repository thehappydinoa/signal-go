package signal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	"github.com/thehappydinoa/signal-go/internal/provisioning"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// LinkOptions configures [Link].
type LinkOptions struct {
	// OnURL is called once the linking URL is ready. Typical implementations
	// print the URL or render it as a QR code in the terminal. Required.
	OnURL func(linkURL string) error

	// Store persists the completed account so [Open] can later resume.
	// Required.
	Store account.Store

	// UserAgent reported in X-Signal-Agent headers. Defaults to "signal-go".
	UserAgent string

	// DeviceName is shown in the user's "Linked devices" list. If empty,
	// the device appears unnamed until a SyncMessage updates it.
	// (Encrypted device names land in Phase 3.)
	DeviceName string

	// OneTimePreKeyCount is the size of each one-time prekey batch
	// (Curve25519 + Kyber) uploaded after the link succeeds. Defaults to
	// 100, matching upstream clients. Set to 0 to skip the upload (the
	// device will still be linked, but cannot receive new sessions until
	// you upload prekeys later).
	OneTimePreKeyCount int

	// ProvisioningURL overrides the websocket endpoint. Default: production.
	ProvisioningURL string
	// APIBaseURL overrides chat.signal.org for the REST call.
	APIBaseURL string

	// testSkipPreKeyUpload is a test-only hook to opt out of the one-time
	// prekey upload while still keeping the rest of the flow.
	testSkipPreKeyUpload bool
}

// DefaultOneTimePreKeyCount is the size of each one-time prekey batch
// uploaded at link time when [LinkOptions.OneTimePreKeyCount] is zero.
const DefaultOneTimePreKeyCount = 100

// LinkedAccount summarises a newly-linked device. The full state has been
// persisted via Store.SaveAccount; callers can drop it after this returns.
type LinkedAccount struct {
	ACI      string
	PNI      string
	Number   string
	DeviceID uint32
}

// Link runs the secondary-device QR-link handshake against chat.signal.org
// and completes registration. On success the new device's state is
// persisted via opts.Store; the returned [LinkedAccount] is a summary.
//
// The user must scan the URL passed to opts.OnURL with their primary
// device's "Linked devices" menu within the Signal mobile app. ctx
// cancellation aborts the wait.
func Link(ctx context.Context, opts LinkOptions) (*LinkedAccount, error) {
	if opts.OnURL == nil {
		return nil, errors.New("signal.Link: LinkOptions.OnURL is required")
	}
	if opts.Store == nil {
		return nil, errors.New("signal.Link: LinkOptions.Store is required")
	}

	// Step 1: QR handshake (provisioning ws + decrypt envelope).
	sess, err := provisioning.Link(ctx, provisioning.Options{
		UserAgent: opts.UserAgent,
		URL:       opts.ProvisioningURL,
		OnURL:     opts.OnURL,
	})
	if err != nil {
		return nil, err
	}
	msg := sess.Message
	if msg.GetAci() == "" || msg.GetNumber() == "" || msg.GetProvisioningCode() == "" {
		return nil, errors.New("signal.Link: provisioning message missing required fields")
	}

	// Step 2: generate per-namespace identity-derived state (registration
	// IDs, signed prekeys, last-resort Kyber prekeys).
	aciIdent, err := buildIdentity(msg.GetAciIdentityKeyPublic(), msg.GetAciIdentityKeyPrivate())
	if err != nil {
		return nil, fmt.Errorf("signal.Link: ACI identity: %w", err)
	}
	pniIdent, err := buildIdentity(msg.GetPniIdentityKeyPublic(), msg.GetPniIdentityKeyPrivate())
	if err != nil {
		return nil, fmt.Errorf("signal.Link: PNI identity: %w", err)
	}

	// Step 3: account password (HTTP Basic credential for all post-link
	// authenticated requests).
	password, err := generatePassword()
	if err != nil {
		return nil, fmt.Errorf("signal.Link: generate password: %w", err)
	}

	// Step 4: assemble + send the link request.
	webc := web.New(opts.APIBaseURL, opts.UserAgent)
	req := buildLinkRequest(msg.GetProvisioningCode(), msg.GetProfileKey(), msg.GetReadReceipts(), aciIdent, pniIdent, opts.DeviceName)
	resp, err := webc.LinkDevice(ctx, msg.GetProvisioningCode(), password, req)
	if err != nil {
		return nil, fmt.Errorf("signal.Link: register: %w", err)
	}
	if resp.DeviceID == 0 {
		return nil, errors.New("signal.Link: server returned deviceId=0")
	}

	// Step 5: upload one-time prekey batches (ACI + PNI, EC + Kyber) so
	// recipients can establish sessions with us. Skipped if disabled.
	count := opts.OneTimePreKeyCount
	if count < 0 {
		return nil, errors.New("signal.Link: OneTimePreKeyCount must be non-negative")
	}
	if count == 0 && !opts.skipOneTimePreKeys() {
		count = DefaultOneTimePreKeyCount
	}
	if count > 0 {
		creds := web.Credentials{
			Username: fmt.Sprintf("%s.%d", resp.UUID, resp.DeviceID),
			Password: password,
		}
		var err error
		aciIdent, err = uploadOneTimePreKeys(ctx, webc, creds, web.IdentityACI, aciIdent, count)
		if err != nil {
			return nil, fmt.Errorf("signal.Link: upload ACI prekeys: %w", err)
		}
		pniIdent, err = uploadOneTimePreKeys(ctx, webc, creds, web.IdentityPNI, pniIdent, count)
		if err != nil {
			return nil, fmt.Errorf("signal.Link: upload PNI prekeys: %w", err)
		}
	}

	// Step 6: persist.
	acct := &account.Account{
		ACI:                resp.UUID,
		PNI:                resp.PNI,
		Number:             msg.GetNumber(),
		DeviceID:           resp.DeviceID,
		Password:           password,
		ProfileKey:         msg.GetProfileKey(),
		AccountEntropyPool: msg.GetAccountEntropyPool(),
		ReadReceipts:       msg.GetReadReceipts(),
		ACIIdentity:        aciIdent,
		PNIIdentity:        pniIdent,
	}
	if err := opts.Store.SaveAccount(acct); err != nil {
		return nil, fmt.Errorf("signal.Link: persist account: %w", err)
	}
	return &LinkedAccount{
		ACI: acct.ACI, PNI: acct.PNI, Number: acct.Number, DeviceID: acct.DeviceID,
	}, nil
}

// skipOneTimePreKeys is a hook for tests to disable the one-time upload
// without changing the public API. Set by link_test.go.
func (o LinkOptions) skipOneTimePreKeys() bool {
	return o.testSkipPreKeyUpload
}

// uploadOneTimePreKeys generates count one-time Curve25519 + Kyber prekeys
// for ident, uploads them via PUT /v2/keys, and returns the identity with
// its NextPreKeyID / NextKyberPreKeyID bumped.
func uploadOneTimePreKeys(ctx context.Context, webc *web.Client, creds web.Credentials, kind web.IdentityType, ident account.Identity, count int) (account.Identity, error) {
	identityPriv, err := libsignal.DeserializePrivateKey(ident.PrivateKey)
	if err != nil {
		return ident, fmt.Errorf("identity priv: %w", err)
	}
	ecBatch, err := prekeys.GenerateOneTimePreKeys(ident.NextPreKeyID, count)
	if err != nil {
		return ident, err
	}
	kemBatch, err := prekeys.GenerateOneTimeKyberPreKeys(identityPriv, ident.NextKyberPreKeyID, count)
	if err != nil {
		return ident, err
	}
	req := web.UploadPreKeysRequest{
		IdentityKey: base64.StdEncoding.EncodeToString(ident.PublicKey),
		PreKeys:     web.ECPreKeysFrom(ecBatch),
		PqPreKeys:   web.KEMPreKeysFrom(kemBatch),
	}
	if err := webc.UploadPreKeys(ctx, creds, kind, req); err != nil {
		return ident, err
	}
	ident.NextPreKeyID += uint32(count)
	ident.NextKyberPreKeyID += uint32(count)
	return ident, nil
}

// buildIdentity rehydrates an identity keypair from its ProvisionMessage
// bytes and generates the initial signed + Kyber prekeys for it.
func buildIdentity(pubBytes, privBytes []byte) (account.Identity, error) {
	if len(pubBytes) == 0 || len(privBytes) == 0 {
		return account.Identity{}, errors.New("identity key missing")
	}
	priv, err := libsignal.DeserializePrivateKey(privBytes)
	if err != nil {
		return account.Identity{}, fmt.Errorf("private key invalid: %w", err)
	}
	if _, err := libsignal.DeserializePublicKey(pubBytes); err != nil {
		return account.Identity{}, fmt.Errorf("public key invalid: %w", err)
	}
	regID, err := prekeys.NewRegistrationID()
	if err != nil {
		return account.Identity{}, err
	}
	spk, err := prekeys.GenerateSignedPreKey(priv, 1)
	if err != nil {
		return account.Identity{}, err
	}
	kspk, err := prekeys.GenerateLastResortKyberPreKey(priv, 1)
	if err != nil {
		return account.Identity{}, err
	}
	return account.Identity{
		PublicKey:             pubBytes,
		PrivateKey:            privBytes,
		RegistrationID:        regID,
		SignedPreKey:          *spk,
		LastResortKyberPreKey: *kspk,
		// We've consumed id 1 for the rotating signed + last-resort Kyber
		// prekeys, so one-time keys start at 2.
		NextPreKeyID:      2,
		NextKyberPreKeyID: 2,
	}, nil
}

// buildLinkRequest constructs the /v1/devices/link request body.
func buildLinkRequest(provisioningCode string, profileKey []byte, readReceipts bool, aci, pni account.Identity, deviceName string) web.LinkDeviceRequest {
	return web.LinkDeviceRequest{
		VerificationCode: provisioningCode,
		AccountAttributes: web.AccountAttributes{
			FetchesMessages:           true,
			RegistrationID:            aci.RegistrationID,
			PNIRegistrationID:         pni.RegistrationID,
			Name:                      deviceName,
			Capabilities:              web.DefaultCapabilities(),
			UnidentifiedAccessKey:     deriveUDKey(profileKey),
			DiscoverableByPhoneNumber: true,
		},
		ACISignedPreKey:       web.SignedECPreKeyFrom(aci.SignedPreKey),
		PNISignedPreKey:       web.SignedECPreKeyFrom(pni.SignedPreKey),
		ACIPqLastResortPreKey: web.SignedKEMPreKeyFrom(aci.LastResortKyberPreKey),
		PNIPqLastResortPreKey: web.SignedKEMPreKeyFrom(pni.LastResortKyberPreKey),
	}
}

// deriveUDKey produces the 16-byte unidentified-access key that Signal
// expects in AccountAttributes.
//
// Phase 2 placeholder: we send 16 zero bytes (server accepts this at link
// time; sealed-sender will not work until we plug in the proper AES-GCM-SIV
// derivation in Phase 4). See ROADMAP.md.
func deriveUDKey(profileKey []byte) string {
	_ = profileKey
	return base64.StdEncoding.EncodeToString(make([]byte, 16))
}

// generatePassword returns a random 18-byte password as a base64 string.
// 18 bytes encode to 24 chars without padding, matching what other Signal
// clients emit.
func generatePassword() (string, error) {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}
