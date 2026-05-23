package signal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// Canonical fixtures used across every send-side test. Pulled out as
// package-level constants because they're only ever the same literal —
// golangci-lint's `unparam` flags helpers that always receive the same
// argument, and the values are also referenced from assertions.
//
// Multi-device tests use [newRecipientWithIdentity] directly with
// varying deviceID + registrationID.
const (
	testRecipientACI   = "bob-aci-uuid"
	testRecipientDevID = uint32(1)
	testRecipientRegID = uint32(4242)
	testSenderACI      = "alice-aci-uuid"
	testSenderDevID    = uint32(1)
)

// recipientFixture is a peer ("Bob") with all the identity + prekey
// material needed to (a) publish a bundle the sender can fetch, and
// (b) decrypt what the sender produces.
type recipientFixture struct {
	t      *testing.T
	aci    string
	devID  uint32
	regID  uint32
	stores *memstore.SignalStores
	acct   *account.Account
	// Saved-from-link material we surface in the fake /v2/keys response.
	identityKey  *libsignal.IdentityKeyPair
	signedPreKey *signedPreKeyTriple
	kyberPreKey  *kyberPreKeyTriple
	oneTimeKey   *oneTimePreKeyPair
}

type signedPreKeyTriple struct {
	id   uint32
	pub  *libsignal.PublicKey
	priv *libsignal.PrivateKey
	sig  []byte
}

type kyberPreKeyTriple struct {
	id   uint32
	pub  *libsignal.KyberPublicKey
	pair *libsignal.KyberKeyPair
	sig  []byte
}

type oneTimePreKeyPair struct {
	id   uint32
	pub  *libsignal.PublicKey
	priv *libsignal.PrivateKey
}

func newRecipient(t *testing.T) *recipientFixture {
	t.Helper()
	aci := testRecipientACI
	devID := testRecipientDevID
	regID := testRecipientRegID
	ident, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	// signed prekey
	spkPriv, err := libsignal.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("spk priv: %v", err)
	}
	spkPub, err := spkPriv.PublicKey()
	if err != nil {
		t.Fatalf("spk pub: %v", err)
	}
	spkPubBytes, _ := spkPub.Serialize()
	spkSig, err := libsignal.Sign(ident.Private, spkPubBytes)
	if err != nil {
		t.Fatalf("sign spk: %v", err)
	}

	// kyber prekey
	kkp, err := libsignal.GenerateKyberKeyPair()
	if err != nil {
		t.Fatalf("kyber pair: %v", err)
	}
	kpkPub, _ := kkp.Public()
	kpkBytes, _ := kpkPub.Serialize()
	kpkSig, err := libsignal.Sign(ident.Private, kpkBytes)
	if err != nil {
		t.Fatalf("sign kpk: %v", err)
	}

	ss := memstore.NewSignalStores()
	idPubBytes, _ := ident.Public.Serialize()
	idPrivBytes, _ := ident.Private.Serialize()
	ss.SetLocalIdentity(idPubBytes, idPrivBytes, regID)

	// Persist the signed + kyber prekey blobs in Bob's stores so
	// libsignal can find them by id when Alice's PreKeySignalMessage
	// hits Bob's decrypt path.
	const epochTS = uint64(0)
	spkBlob, err := libsignal.NewSignedPreKeyRecordBlob(1, epochTS, spkPub, spkPriv, spkSig)
	if err != nil {
		t.Fatalf("signed prekey blob: %v", err)
	}
	if err := ss.StoreSignedPreKey(1, spkBlob); err != nil {
		t.Fatalf("store signed prekey: %v", err)
	}
	kpkBlob, err := libsignal.NewKyberPreKeyRecordBlob(1, epochTS, kkp, kpkSig)
	if err != nil {
		t.Fatalf("kyber prekey blob: %v", err)
	}
	if err := ss.StoreKyberPreKey(1, kpkBlob); err != nil {
		t.Fatalf("store kyber prekey: %v", err)
	}

	// One-time prekey. libsignal's NewPreKeyBundle currently requires a
	// non-nil one-time prekey; including one matches the common case
	// where the recipient still has spare one-time keys on the server.
	otPriv, err := libsignal.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("one-time priv: %v", err)
	}
	otPub, err := otPriv.PublicKey()
	if err != nil {
		t.Fatalf("one-time pub: %v", err)
	}
	otRec, err := libsignal.NewPreKeyRecord(2, otPriv, otPub)
	if err != nil {
		t.Fatalf("one-time record: %v", err)
	}
	otBlob, err := otRec.Serialize()
	if err != nil {
		t.Fatalf("one-time serialize: %v", err)
	}
	if err := ss.StorePreKey(2, otBlob); err != nil {
		t.Fatalf("store one-time: %v", err)
	}

	acct := &account.Account{
		ACI: aci, PNI: "pni-" + aci, Number: "+15551110001",
		DeviceID: devID, Password: "bob-password",
	}
	return &recipientFixture{
		t: t, aci: aci, devID: devID, regID: regID, stores: ss, acct: acct,
		identityKey:  ident,
		signedPreKey: &signedPreKeyTriple{id: 1, pub: spkPub, priv: spkPriv, sig: spkSig},
		kyberPreKey:  &kyberPreKeyTriple{id: 1, pub: kpkPub, pair: kkp, sig: kpkSig},
		oneTimeKey:   &oneTimePreKeyPair{id: 2, pub: otPub, priv: otPriv},
	}
}

// newRecipientWithIdentity is like newRecipient but uses the provided identity
// key pair rather than generating a fresh one. This is required for multi-device
// fixtures where all devices of the same account must share one identity key:
// libsignal verifies every prekey signature against the identity key reported
// in the bundle, so both must come from the same keypair.
func newRecipientWithIdentity(t *testing.T, aci string, devID, regID uint32, ident *libsignal.IdentityKeyPair) *recipientFixture {
	t.Helper()

	spkPriv, err := libsignal.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("spk priv: %v", err)
	}
	spkPub, err := spkPriv.PublicKey()
	if err != nil {
		t.Fatalf("spk pub: %v", err)
	}
	spkPubBytes, _ := spkPub.Serialize()
	spkSig, err := libsignal.Sign(ident.Private, spkPubBytes)
	if err != nil {
		t.Fatalf("sign spk: %v", err)
	}

	kkp, err := libsignal.GenerateKyberKeyPair()
	if err != nil {
		t.Fatalf("kyber pair: %v", err)
	}
	kpkPub, _ := kkp.Public()
	kpkBytes, _ := kpkPub.Serialize()
	kpkSig, err := libsignal.Sign(ident.Private, kpkBytes)
	if err != nil {
		t.Fatalf("sign kpk: %v", err)
	}

	ss := memstore.NewSignalStores()
	idPubBytes, _ := ident.Public.Serialize()
	idPrivBytes, _ := ident.Private.Serialize()
	ss.SetLocalIdentity(idPubBytes, idPrivBytes, regID)

	const epochTS = uint64(0)
	spkBlob, err := libsignal.NewSignedPreKeyRecordBlob(1, epochTS, spkPub, spkPriv, spkSig)
	if err != nil {
		t.Fatalf("signed prekey blob: %v", err)
	}
	if err := ss.StoreSignedPreKey(1, spkBlob); err != nil {
		t.Fatalf("store signed prekey: %v", err)
	}
	kpkBlob, err := libsignal.NewKyberPreKeyRecordBlob(1, epochTS, kkp, kpkSig)
	if err != nil {
		t.Fatalf("kyber prekey blob: %v", err)
	}
	if err := ss.StoreKyberPreKey(1, kpkBlob); err != nil {
		t.Fatalf("store kyber prekey: %v", err)
	}

	otPriv, err := libsignal.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("one-time priv: %v", err)
	}
	otPub, err := otPriv.PublicKey()
	if err != nil {
		t.Fatalf("one-time pub: %v", err)
	}
	otRec, err := libsignal.NewPreKeyRecord(2, otPriv, otPub)
	if err != nil {
		t.Fatalf("one-time record: %v", err)
	}
	otBlob, err := otRec.Serialize()
	if err != nil {
		t.Fatalf("one-time serialize: %v", err)
	}
	if err := ss.StorePreKey(2, otBlob); err != nil {
		t.Fatalf("store one-time: %v", err)
	}

	acct := &account.Account{
		ACI: aci, PNI: "pni-" + aci, Number: "+15551110001",
		DeviceID: devID, Password: "bob-password",
	}
	return &recipientFixture{
		t: t, aci: aci, devID: devID, regID: regID, stores: ss, acct: acct,
		identityKey:  ident,
		signedPreKey: &signedPreKeyTriple{id: 1, pub: spkPub, priv: spkPriv, sig: spkSig},
		kyberPreKey:  &kyberPreKeyTriple{id: 1, pub: kpkPub, pair: kkp, sig: kpkSig},
		oneTimeKey:   &oneTimePreKeyPair{id: 2, pub: otPub, priv: otPriv},
	}
}

// bundleDevice builds the web.BundleDevice for this fixture.
func (r *recipientFixture) bundleDevice() web.BundleDevice {
	spkBytes, _ := r.signedPreKey.pub.Serialize()
	kpkBytes, _ := r.kyberPreKey.pub.Serialize()
	otBytes, _ := r.oneTimeKey.pub.Serialize()
	dev := web.BundleDevice{
		DeviceID:       r.devID,
		RegistrationID: r.regID,
		SignedPreKey: web.SignedPreKeySlot{
			KeyID:     r.signedPreKey.id,
			PublicKey: base64.StdEncoding.EncodeToString(spkBytes),
			Signature: base64.StdEncoding.EncodeToString(r.signedPreKey.sig),
		},
		PqPreKey: &web.SignedPreKeySlot{
			KeyID:     r.kyberPreKey.id,
			PublicKey: base64.StdEncoding.EncodeToString(kpkBytes),
			Signature: base64.StdEncoding.EncodeToString(r.kyberPreKey.sig),
		},
		PreKey: &web.PreKeySlot{
			KeyID:     r.oneTimeKey.id,
			PublicKey: base64.StdEncoding.EncodeToString(otBytes),
		},
	}
	return dev
}

// bundleResponse builds the JSON the server returns for
// GET /v2/keys/{aci}/*.
func (r *recipientFixture) bundleResponse() web.FetchPreKeyResponse {
	idBytes, _ := r.identityKey.Public.Serialize()
	return web.FetchPreKeyResponse{
		IdentityKey: base64.StdEncoding.EncodeToString(idBytes),
		Devices:     []web.BundleDevice{r.bundleDevice()},
	}
}

// senderFixture is the equivalent of Bob's setup but for the sender
// ("Alice"). Alice doesn't publish a bundle in this test; she just
// needs identity + stores to drive Send.
func newSenderClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	aci := testSenderACI
	devID := testSenderDevID
	ident, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("alice identity: %v", err)
	}
	ss := memstore.NewSignalStores()
	idPub, _ := ident.Public.Serialize()
	idPriv, _ := ident.Private.Serialize()
	ss.SetLocalIdentity(idPub, idPriv, 12345)
	acct := &account.Account{
		ACI: aci, PNI: "pni-" + aci, Number: "+15550000001",
		DeviceID: devID, Password: "alice-password",
	}
	return &Client{
		acct:   acct,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		webc:   web.New(baseURL, "test"),
		stores: ss,
		events: make(chan Event, 4),
	}
}

func TestSendRoundTripDecryptsOnRecipient(t *testing.T) {
	bob := newRecipient(t)

	// Fake REST server: GET /* returns bundle, PUT /messages captures envelope.
	var (
		mu       sync.Mutex
		captured *web.SendMessageRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/*":
			_ = json.NewEncoder(w).Encode(bob.bundleResponse())
		case r.Method == http.MethodPut && r.URL.Path == "/v1/messages/"+bob.aci:
			raw, _ := io.ReadAll(r.Body)
			var req web.SendMessageRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			captured = &req
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(web.SendMessageResponse{})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	alice := newSenderClient(t, srv.URL)

	const text = "hello bob, from alice"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	receipt, err := alice.Send(ctx, bob.aci, text)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receipt.RecipientACI != bob.aci || receipt.Timestamp.IsZero() {
		t.Errorf("receipt: %+v", receipt)
	}

	mu.Lock()
	defer mu.Unlock()
	if captured == nil {
		t.Fatal("server saw no /v1/messages PUT")
	}
	if len(captured.Messages) != 1 {
		t.Fatalf("want 1 envelope, got %d", len(captured.Messages))
	}
	env := captured.Messages[0]
	if env.DestinationDeviceID != bob.devID || env.DestinationRegistrationID != bob.regID {
		t.Errorf("dest dev/reg = %d/%d, want %d/%d", env.DestinationDeviceID, env.DestinationRegistrationID, bob.devID, bob.regID)
	}
	// First send is always a prekey message.
	if env.Type != web.CiphertextTypePreKey {
		t.Errorf("first-send type = %d, want %d (PreKey)", env.Type, web.CiphertextTypePreKey)
	}

	// Decrypt Bob-side using the captured ciphertext.
	cipherBytes, err := base64.StdEncoding.DecodeString(env.Content)
	if err != nil {
		t.Fatalf("decode content: %v", err)
	}
	plain := bobDecryptPreKey(t, bob, alice.acct.ACI, alice.acct.DeviceID, cipherBytes)

	// Strip padding (terminator 0x80 + trailing zeros) and parse the
	// signalservice.Content protobuf.
	unpadded := stripContentPadding(plain)
	if bytes.LastIndexByte(plain, 0x80) < 0 {
		t.Fatalf("no 0x80 terminator in padded plaintext")
	}
	var got sspb.Content
	if err := proto.Unmarshal(unpadded, &got); err != nil {
		t.Fatalf("unmarshal Content: %v", err)
	}
	if got.GetDataMessage() == nil {
		t.Fatalf("Content has no DataMessage: %+v", &got)
	}
	if got.GetDataMessage().GetBody() != text {
		t.Errorf("body = %q, want %q", got.GetDataMessage().GetBody(), text)
	}
}

func TestSendSecondMessageReusesSession(t *testing.T) {
	// After the first send the session is cached in our SignalStores and
	// the device list is cached in Client; the second send must NOT hit
	// the bundle endpoint again.
	//
	// Note: both envelopes are still type=PREKEY_BUNDLE. Under the
	// Double Ratchet, the sending side keeps emitting prekey messages
	// until it observes a reply from the recipient — that's by design,
	// so a recipient who didn't yet receive the first envelope can
	// still bootstrap from any later one. Transitioning to type=Whisper
	// requires inbound traffic from the recipient, which this test
	// doesn't model.
	bob := newRecipient(t)

	var (
		mu           sync.Mutex
		bundleHits   int
		sentEnvCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/*":
			mu.Lock()
			bundleHits++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(bob.bundleResponse())
		case r.Method == http.MethodPut && r.URL.Path == "/v1/messages/"+bob.aci:
			raw, _ := io.ReadAll(r.Body)
			var req web.SendMessageRequest
			_ = json.Unmarshal(raw, &req)
			mu.Lock()
			sentEnvCount += len(req.Messages)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(web.SendMessageResponse{})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	alice := newSenderClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := alice.Send(ctx, bob.aci, "first"); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	if _, err := alice.Send(ctx, bob.aci, "second"); err != nil {
		t.Fatalf("second Send: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if bundleHits != 1 {
		t.Errorf("bundle hits = %d, want exactly 1 (device list cached after first send)", bundleHits)
	}
	if sentEnvCount != 2 {
		t.Errorf("sent envelopes = %d, want 2", sentEnvCount)
	}
}

func TestSendMultiDeviceFanOut(t *testing.T) {
	// Bob has two linked devices. Send should produce one envelope per device
	// in a single PUT /v1/messages request.
	//
	// Both devices must share one identity key (Signal's invariant). We use
	// newRecipientWithIdentity for device 2 so its prekeys are signed by the
	// same keypair that the bundle response advertises.
	bobDev1 := newRecipient(t)
	bobDev2 := newRecipientWithIdentity(t, "bob-aci-uuid", 2, 5555, bobDev1.identityKey)

	var (
		mu      sync.Mutex
		putBody web.SendMessageRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bobDev1.aci+"/*":
			idBytes, _ := bobDev1.identityKey.Public.Serialize()
			_ = json.NewEncoder(w).Encode(web.FetchPreKeyResponse{
				IdentityKey: base64.StdEncoding.EncodeToString(idBytes),
				Devices:     []web.BundleDevice{bobDev1.bundleDevice(), bobDev2.bundleDevice()},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/messages/"+bobDev1.aci:
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			_ = json.Unmarshal(raw, &putBody)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(web.SendMessageResponse{})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	alice := newSenderClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := alice.Send(ctx, bobDev1.aci, "hi both devices"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(putBody.Messages) != 2 {
		t.Fatalf("want 2 envelopes (one per device), got %d", len(putBody.Messages))
	}
	devIDs := map[uint32]bool{}
	for _, msg := range putBody.Messages {
		devIDs[msg.DestinationDeviceID] = true
	}
	if !devIDs[1] || !devIDs[2] {
		t.Errorf("envelopes targeted devices %v, want both 1 and 2", devIDs)
	}
}

func TestSendRetryOnMismatchedDevices(t *testing.T) {
	// The server initially reports only device 1 in the bundle response,
	// then the first PUT returns 409 with missingDevices=[2]. Send must
	// fetch a bundle for device 2, then retry and succeed.
	//
	// Device 2 uses the same identity key as device 1 (shared account identity).
	bob := newRecipient(t)
	bob2 := newRecipientWithIdentity(t, "bob-aci-uuid", 2, 5555, bob.identityKey)

	var (
		mu      sync.Mutex
		putHits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/*":
			// Initial discovery: only device 1.
			_ = json.NewEncoder(w).Encode(bob.bundleResponse())
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/2":
			// Bundle fetch for the missing device 2.
			idBytes, _ := bob2.identityKey.Public.Serialize()
			_ = json.NewEncoder(w).Encode(web.FetchPreKeyResponse{
				IdentityKey: base64.StdEncoding.EncodeToString(idBytes),
				Devices:     []web.BundleDevice{bob2.bundleDevice()},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/messages/"+bob.aci:
			mu.Lock()
			putHits++
			hit := putHits
			mu.Unlock()
			if hit == 1 {
				// First attempt: tell the sender about a new device.
				w.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"missingDevices": []uint32{2},
					"extraDevices":   []uint32{},
				})
				return
			}
			// Retry: succeed.
			_ = json.NewEncoder(w).Encode(web.SendMessageResponse{})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	alice := newSenderClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receipt, err := alice.Send(ctx, bob.aci, "hello with retry")
	if err != nil {
		t.Fatalf("Send returned error after retry: %v", err)
	}
	if receipt.RecipientACI != bob.aci {
		t.Errorf("receipt.RecipientACI = %q, want %q", receipt.RecipientACI, bob.aci)
	}

	mu.Lock()
	defer mu.Unlock()
	if putHits != 2 {
		t.Errorf("PUT hits = %d, want exactly 2 (initial + retry)", putHits)
	}
}

func TestSendRetryOnStaleDevices(t *testing.T) {
	// The first PUT returns HTTP 410 (stale registration ID for device 1).
	// Send must drop the session, re-fetch the bundle, and retry.
	bob := newRecipient(t)

	var (
		mu      sync.Mutex
		putHits int
		getHits int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/*":
			mu.Lock()
			getHits++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(bob.bundleResponse())
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/1":
			mu.Lock()
			getHits++
			mu.Unlock()
			// Re-fetch of stale device's bundle: return fresh bundle.
			_ = json.NewEncoder(w).Encode(bob.bundleResponse())
		case r.Method == http.MethodPut && r.URL.Path == "/v1/messages/"+bob.aci:
			mu.Lock()
			putHits++
			hit := putHits
			mu.Unlock()
			if hit == 1 {
				w.WriteHeader(http.StatusGone)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"staleDevices": []uint32{1},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(web.SendMessageResponse{})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	alice := newSenderClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receipt, err := alice.Send(ctx, bob.aci, "hello after stale session")
	if err != nil {
		t.Fatalf("Send returned error after stale-device retry: %v", err)
	}
	if receipt.RecipientACI != bob.aci {
		t.Errorf("receipt.RecipientACI = %q, want %q", receipt.RecipientACI, bob.aci)
	}

	mu.Lock()
	defer mu.Unlock()
	if putHits != 2 {
		t.Errorf("PUT hits = %d, want exactly 2", putHits)
	}
	// Discovery (/*) + stale re-fetch (/1) = 2.
	if getHits != 2 {
		t.Errorf("GET hits = %d, want 2 (discover + stale re-fetch)", getHits)
	}
}

func TestSendInputValidation(t *testing.T) {
	c := &Client{} // no webc / stores
	if _, err := c.Send(context.Background(), "bob", "hi"); err == nil {
		t.Error("expected error when send-side deps missing")
	}

	c.webc = web.New("http://x", "t")
	c.stores = memstore.NewSignalStores()
	if _, err := c.Send(context.Background(), "", "hi"); err == nil {
		t.Error("expected error empty recipient")
	}
	if _, err := c.Send(context.Background(), "bob", ""); err == nil {
		t.Error("expected error empty body")
	}
}

func TestSendPropagatesMismatchedDevicesErrorAfterRetry(t *testing.T) {
	// If both the initial send AND the retry return 409, the error
	// propagates to the caller. errors.As can still find the typed error
	// through the wrapping.
	bob := newRecipient(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(bob.bundleResponse())
		case http.MethodPut:
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"missingDevices": []uint32{2},
				"extraDevices":   []uint32{},
			})
		}
	}))
	defer srv.Close()

	alice := newSenderClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := alice.Send(ctx, bob.aci, "hi")
	var mde *web.MismatchedDevicesError
	if !errors.As(err, &mde) {
		t.Fatalf("err = %v (%T), want *web.MismatchedDevicesError in chain", err, err)
	}
}

// bobDecryptPreKey runs libsignal's prekey-message decrypt path with
// Bob's stores + identity. The resulting plaintext is the
// padded-Content bytes.
func bobDecryptPreKey(t *testing.T, bob *recipientFixture, senderACI string, senderDev uint32, cipherBytes []byte) []byte {
	t.Helper()
	msg, err := libsignal.DeserializePreKeySignalMessage(cipherBytes)
	if err != nil {
		t.Fatalf("deserialize prekey msg: %v", err)
	}
	remote, err := libsignal.NewAddress(senderACI, senderDev)
	if err != nil {
		t.Fatalf("remote addr: %v", err)
	}
	local, err := libsignal.NewAddress(bob.aci, bob.devID)
	if err != nil {
		t.Fatalf("local addr: %v", err)
	}
	h := libsignal.NewStoreHandle(bob.stores)
	defer h.Release()
	plain, err := libsignal.DecryptPreKeySignalMessage(msg, remote, local, h)
	if err != nil {
		t.Fatalf("DecryptPreKeySignalMessage: %v", err)
	}
	return plain
}

func TestBuildEditMessageContent(t *testing.T) {
	b, err := buildEditMessageContent("edited", 2000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var c sspb.Content
	if err := proto.Unmarshal(b, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	em := c.GetEditMessage()
	if em == nil {
		t.Fatal("missing EditMessage")
	}
	if em.GetTargetSentTimestamp() != 1000 {
		t.Fatalf("target ts = %d", em.GetTargetSentTimestamp())
	}
	if em.GetDataMessage().GetTimestamp() != 2000 {
		t.Fatalf("data ts = %d", em.GetDataMessage().GetTimestamp())
	}
	if em.GetDataMessage().GetBody() != "edited" {
		t.Fatalf("body = %q", em.GetDataMessage().GetBody())
	}
}

// Compile-time guard: stress an obvious pitfall — the addr used during
// encrypt is also the one used to look up the resulting session.
var _ = store.Address{ServiceID: "x", DeviceID: 1}
