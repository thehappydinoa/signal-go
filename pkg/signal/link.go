package signal

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/devicename"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/prekeymaint"
	"github.com/thehappydinoa/signal-go/internal/prekeys"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	"github.com/thehappydinoa/signal-go/internal/provisioning"
	"github.com/thehappydinoa/signal-go/internal/store"
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

	// LinkAndSync advertises backup3 during provisioning and, when the
	// primary sends an ephemeralBackupKey, polls for and validates the
	// transfer archive after registration completes.
	LinkAndSync bool
	// ImportTransferArchive imports validated backup frames into
	// SignalStores / BackupImportStore after link-and-sync. When nil,
	// defaults to true if any import sink (stores or OnChatItem) is set.
	ImportTransferArchive *bool
	// SignalStores receives imported contact identity keys during
	// link-and-sync import.
	SignalStores store.SignalStores
	// BackupImportStore receives imported contact/group list entries.
	BackupImportStore store.BackupImportStore
	// OnChatItem receives each validated transfer-archive ChatItem frame as
	// protobuf bytes (signal.backup.ChatItem). When set with
	// [LinkOptions.SignalStores] or [LinkOptions.BackupImportStore], it
	// participates in the default "should import" decision for link-and-sync.
	OnChatItem func(serializedChatItem []byte) error

	// ClientProfile selects a realistic User-Agent preset when UserAgent is
	// empty. Default: [UserAgentSignalGo].
	ClientProfile UserAgentProfile
	// UserAgentOptions overrides app/OS version strings in ClientProfile.
	UserAgentOptions UserAgentOptions
	// UserAgent is sent in User-Agent and X-Signal-Agent headers. When empty,
	// formatted from ClientProfile.
	UserAgent string

	// DeviceName is shown in the user's "Linked devices" list. If empty,
	// the device appears unnamed until a SyncMessage updates it.
	// Non-empty values are encrypted for the ACI identity key (same scheme
	// as Signal Android's DeviceNameCipher) before JSON encoding.
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
	// Sync is populated when [LinkOptions.LinkAndSync] is true and the
	// primary participates in link-and-sync.
	Sync *SyncTransferArchiveResult
}

// Link runs the secondary-device QR-link handshake against chat.signal.org
// and completes registration. On success the new device's state is
// persisted via opts.Store; the returned [LinkedAccount] is a summary.
//
// The user must scan the URL passed to opts.OnURL with their primary
// device's "Linked devices" menu within the Signal mobile app. ctx
// cancellation aborts the wait.
//
//nolint:gocyclo // linear provisioning protocol; helpers would obscure the sequence.
func Link(ctx context.Context, opts LinkOptions) (*LinkedAccount, error) {
	if opts.OnURL == nil {
		return nil, errors.New("signal.Link: LinkOptions.OnURL is required")
	}
	if opts.Store == nil {
		return nil, errors.New("signal.Link: LinkOptions.Store is required")
	}

	// Step 1: QR handshake (provisioning ws + decrypt envelope).
	ua := resolveUserAgent(opts.ClientProfile, opts.UserAgent, opts.UserAgentOptions)
	sess, err := provisioning.Link(ctx, provisioning.Options{
		UserAgent:    ua,
		URL:          opts.ProvisioningURL,
		OnURL:        opts.OnURL,
		Capabilities: opts.provisioningCapabilities(),
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
	webc := web.New(opts.APIBaseURL, ua)
	req, err := buildLinkRequest(msg.GetProvisioningCode(), msg.GetProfileKey(), aciIdent, pniIdent, opts.DeviceName)
	if err != nil {
		return nil, fmt.Errorf("signal.Link: build link request: %w", err)
	}
	// Authenticate the link request with the e164 number (not the
	// provisioning code); the code travels in req.VerificationCode. This
	// matches signal-cli / libsignal-service-java.
	resp, err := webc.LinkDevice(ctx, msg.GetNumber(), password, req)
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

	linked := &LinkedAccount{
		ACI: acct.ACI, PNI: acct.PNI, Number: acct.Number, DeviceID: acct.DeviceID,
	}
	if syncResult, err := maybeSyncTransferArchive(ctx, opts, msg, webc, resp.UUID, resp.DeviceID, password); err != nil {
		return linked, err
	} else if syncResult != nil {
		linked.Sync = syncResult
	}
	return linked, nil
}

func maybeSyncTransferArchive(
	ctx context.Context,
	opts LinkOptions,
	msg *provpb.ProvisionMessage,
	webc *web.Client,
	aci string,
	deviceID uint32,
	password string,
) (*SyncTransferArchiveResult, error) {
	if !opts.LinkAndSync || len(msg.GetEphemeralBackupKey()) != libsignal.BackupKeyLen {
		return nil, nil
	}
	creds := web.Credentials{
		Username: fmt.Sprintf("%s.%d", aci, deviceID),
		Password: password,
	}
	syncResult, err := SyncTransferArchive(ctx, webc, creds, SyncTransferArchiveOptions{
		EphemeralBackupKey: msg.GetEphemeralBackupKey(),
		ACI:                aci,
		Import:             opts.shouldImportTransferArchive(),
		Identities:         opts.SignalStores,
		BackupImport:       opts.BackupImportStore,
		OnChatItem:         opts.OnChatItem,
	})
	if err != nil {
		return nil, fmt.Errorf("signal.Link: sync transfer archive: %w", err)
	}
	return syncResult, nil
}

// skipOneTimePreKeys is a hook for tests to disable the one-time upload
// without changing the public API. Set by link_test.go.
func (o LinkOptions) skipOneTimePreKeys() bool {
	return o.testSkipPreKeyUpload
}

func (o LinkOptions) provisioningCapabilities() []string {
	if o.LinkAndSync {
		return []string{CapabilityBackup3}
	}
	return nil
}

func (o LinkOptions) shouldImportTransferArchive() bool {
	if o.ImportTransferArchive != nil {
		return *o.ImportTransferArchive
	}
	return o.SignalStores != nil || o.BackupImportStore != nil || o.OnChatItem != nil
}

func uploadOneTimePreKeys(ctx context.Context, webc *web.Client, creds web.Credentials, kind web.IdentityType, ident account.Identity, count int) (account.Identity, error) {
	upload := prekeymaint.UploadIdentity{
		PublicKey:         ident.PublicKey,
		PrivateKey:        ident.PrivateKey,
		NextPreKeyID:      ident.NextPreKeyID,
		NextKyberPreKeyID: ident.NextKyberPreKeyID,
	}
	upload, err := prekeymaint.UploadOneTimeBatch(ctx, webc, creds, kind, upload, count, nil)
	if err != nil {
		return ident, err
	}
	ident.NextPreKeyID = upload.NextPreKeyID
	ident.NextKyberPreKeyID = upload.NextKyberPreKeyID
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
// The readReceipts flag from the ProvisionMessage is persisted on the
// Account but is not part of the registration request; the server
// learns it later via SyncMessage.
func buildLinkRequest(provisioningCode string, profileKey []byte, aci, pni account.Identity, deviceName string) (web.LinkDeviceRequest, error) {
	name := ""
	if deviceName != "" {
		aciPub, err := libsignal.DeserializePublicKey(aci.PublicKey)
		if err != nil {
			return web.LinkDeviceRequest{}, fmt.Errorf("signal.Link: ACI identity public key: %w", err)
		}
		enc, err := encryptDeviceNameForLink(deviceName, aciPub)
		if err != nil {
			return web.LinkDeviceRequest{}, err
		}
		name = enc
	}
	return web.LinkDeviceRequest{
		VerificationCode: provisioningCode,
		AccountAttributes: web.AccountAttributes{
			FetchesMessages:           true,
			RegistrationID:            aci.RegistrationID,
			PNIRegistrationID:         pni.RegistrationID,
			Name:                      name,
			Capabilities:              web.DefaultCapabilities(),
			UnidentifiedAccessKey:     deriveUDKey(profileKey),
			DiscoverableByPhoneNumber: true,
		},
		ACISignedPreKey:       web.SignedECPreKeyFrom(aci.SignedPreKey),
		PNISignedPreKey:       web.SignedECPreKeyFrom(pni.SignedPreKey),
		ACIPqLastResortPreKey: web.SignedKEMPreKeyFrom(aci.LastResortKyberPreKey),
		PNIPqLastResortPreKey: web.SignedKEMPreKeyFrom(pni.LastResortKyberPreKey),
	}, nil
}

// deriveUDKey returns the unidentified-access key field for AccountAttributes
// at link time. Signal accepts an all-zero placeholder during registration;
// sealed-sender send uses the profile-key-derived UAK from inbound traffic
// (see ADR 0017), not this registration-time value.
func deriveUDKey(profileKey []byte) string {
	_ = profileKey
	return base64.StdEncoding.EncodeToString(make([]byte, 16))
}

func encryptDeviceNameForLink(deviceName string, aciIdentityPublic *libsignal.PublicKey) (string, error) {
	s, err := devicename.Encrypt(deviceName, aciIdentityPublic)
	if err != nil {
		return "", fmt.Errorf("signal.Link: encrypt device name: %w", err)
	}
	return s, nil
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
