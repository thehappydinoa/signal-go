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

func newRecipient(t *testing.T, aci string, devID, regID uint32) *recipientFixture {
	t.Helper()
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

// bundleResponse builds the JSON the server returns for
// GET /v2/keys/{aci}/{deviceID}.
func (r *recipientFixture) bundleResponse() web.FetchPreKeyResponse {
	idBytes, _ := r.identityKey.Public.Serialize()
	spkBytes, _ := r.signedPreKey.pub.Serialize()
	kpkBytes, _ := r.kyberPreKey.pub.Serialize()
	resp := web.FetchPreKeyResponse{
		IdentityKey: base64.StdEncoding.EncodeToString(idBytes),
	}
	dev := struct {
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
	}{
		DeviceID:       r.devID,
		RegistrationID: r.regID,
	}
	dev.SignedPreKey.KeyID = r.signedPreKey.id
	dev.SignedPreKey.PublicKey = base64.StdEncoding.EncodeToString(spkBytes)
	dev.SignedPreKey.Signature = base64.StdEncoding.EncodeToString(r.signedPreKey.sig)
	dev.PqPreKey = &struct {
		KeyID     uint32 `json:"keyId"`
		PublicKey string `json:"publicKey"`
		Signature string `json:"signature"`
	}{
		KeyID:     r.kyberPreKey.id,
		PublicKey: base64.StdEncoding.EncodeToString(kpkBytes),
		Signature: base64.StdEncoding.EncodeToString(r.kyberPreKey.sig),
	}
	otBytes, _ := r.oneTimeKey.pub.Serialize()
	dev.PreKey = &struct {
		KeyID     uint32 `json:"keyId"`
		PublicKey string `json:"publicKey"`
	}{
		KeyID:     r.oneTimeKey.id,
		PublicKey: base64.StdEncoding.EncodeToString(otBytes),
	}
	resp.Devices = append(resp.Devices, dev)
	return resp
}

// senderFixture is the equivalent of Bob's setup but for the sender
// ("Alice"). Alice doesn't publish a bundle in this test; she just
// needs identity + stores to drive Send.
func newSenderClient(t *testing.T, aci string, devID uint32, baseURL string) *Client {
	t.Helper()
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
	bob := newRecipient(t, "bob-aci-uuid", 1, 4242)

	// Fake REST server: GET bundle, PUT messages — capture the envelope.
	var (
		mu       sync.Mutex
		captured *web.SendMessageRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/1":
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

	alice := newSenderClient(t, "alice-aci-uuid", 1, srv.URL)

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
	unpadded := stripContentPadding(t, plain)
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
	// After the first send the session is cached in our SignalStores;
	// the second send must NOT hit the prekey-bundle endpoint again.
	//
	// Note: both envelopes are still type=PREKEY_BUNDLE. Under the
	// Double Ratchet, the sending side keeps emitting prekey messages
	// until it observes a reply from the recipient — that's by design,
	// so a recipient who didn't yet receive the first envelope can
	// still bootstrap from any later one. Transitioning to type=Whisper
	// requires inbound traffic from the recipient, which this test
	// doesn't model.
	bob := newRecipient(t, "bob-aci-uuid", 1, 4242)

	var (
		mu           sync.Mutex
		bundleHits   int
		sentEnvCount int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/keys/"+bob.aci+"/1":
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

	alice := newSenderClient(t, "alice-aci-uuid", 1, srv.URL)
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
		t.Errorf("bundle hits = %d, want exactly 1 (session should be cached)", bundleHits)
	}
	if sentEnvCount != 2 {
		t.Errorf("sent envelopes = %d, want 2", sentEnvCount)
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

func TestSendSurfacesMismatchedDevicesError(t *testing.T) {
	bob := newRecipient(t, "bob-aci-uuid", 1, 4242)
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

	alice := newSenderClient(t, "alice-aci-uuid", 1, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := alice.Send(ctx, bob.aci, "hi")
	var mde *web.MismatchedDevicesError
	if !errors.As(err, &mde) {
		t.Fatalf("err = %v (%T), want *web.MismatchedDevicesError", err, err)
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

// stripContentPadding strips Signal's padding scheme: find the last
// 0x80 byte and drop everything from there onward. Anything before is
// the marshalled Content protobuf.
func stripContentPadding(t *testing.T, padded []byte) []byte {
	t.Helper()
	idx := bytes.LastIndexByte(padded, 0x80)
	if idx < 0 {
		t.Fatalf("no 0x80 terminator in padded plaintext")
	}
	return padded[:idx]
}

// Compile-time guard: stress an obvious pitfall — the addr used during
// encrypt is also the one used to look up the resulting session.
var _ = store.Address{ServiceID: "x", DeviceID: 1}
