package signal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// fakeSignal answers both halves of the link flow: the provisioning
// websocket and the REST /v1/devices/link call.
type fakeSignal struct {
	t            *testing.T
	httpSrv      *httptest.Server
	wsSrv        *httptest.Server
	addressUUID  string
	envelopeCh   chan *provpb.ProvisionEnvelope // buffered; server waits on this
	lastLinkReq  web.LinkDeviceRequest
	lastLinkAuth string
	linkResponse web.LinkDeviceResponse
}

func newFakeSignal(t *testing.T, addr string) *fakeSignal {
	t.Helper()
	f := &fakeSignal{
		t:           t,
		addressUUID: addr,
		envelopeCh:  make(chan *provpb.ProvisionEnvelope, 1),
		linkResponse: web.LinkDeviceResponse{
			UUID: "aci-uuid-server", DeviceID: 3, PNI: "pni-uuid-server",
		},
	}
	f.httpSrv = httptest.NewServer(http.HandlerFunc(f.handleHTTP))
	f.wsSrv = httptest.NewServer(http.HandlerFunc(f.handleWS))
	return f
}

func (f *fakeSignal) Close() {
	f.httpSrv.Close()
	f.wsSrv.Close()
}

func (f *fakeSignal) ProvisioningURL() string {
	return "ws://" + strings.TrimPrefix(f.wsSrv.URL, "http://")
}
func (f *fakeSignal) APIBaseURL() string { return f.httpSrv.URL }

func (f *fakeSignal) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/devices/link" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(raw, &f.lastLinkReq)
	f.lastLinkAuth = r.Header.Get("Authorization")
	_ = json.NewEncoder(w).Encode(f.linkResponse)
}

func (f *fakeSignal) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		f.t.Errorf("ws accept: %v", err)
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")
	addrBody, _ := proto.Marshal(&provpb.ProvisioningAddress{Address: &f.addressUUID})
	verb, path := "PUT", "/v1/address"
	id := uint64(1)
	reqType := wspb.WebSocketMessage_REQUEST
	addrMsg := &wspb.WebSocketMessage{
		Type:    &reqType,
		Request: &wspb.WebSocketRequestMessage{Verb: &verb, Path: &path, Body: addrBody, Id: &id},
	}
	raw, _ := proto.Marshal(addrMsg)
	_ = c.Write(r.Context(), websocket.MessageBinary, raw)
	for {
		_, data, err := c.Read(r.Context())
		if err != nil {
			return
		}
		var msg wspb.WebSocketMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			return
		}
		if msg.GetType() == wspb.WebSocketMessage_RESPONSE && msg.GetResponse().GetStatus() == 200 && msg.GetResponse().GetId() == id {
			go func() {
				// Block until the test has fed us an envelope. This is
				// what synchronises with OnURL — the test produces the
				// envelope only after seeing the linking URL.
				var env *provpb.ProvisionEnvelope
				select {
				case env = <-f.envelopeCh:
				case <-r.Context().Done():
					return
				}
				envBody, _ := proto.Marshal(env)
				envVerb, envPath := "PUT", "/v1/message"
				envID := uint64(2)
				out, _ := proto.Marshal(&wspb.WebSocketMessage{
					Type:    &reqType,
					Request: &wspb.WebSocketRequestMessage{Verb: &envVerb, Path: &envPath, Body: envBody, Id: &envID},
				})
				_ = c.Write(r.Context(), websocket.MessageBinary, out)
			}()
		}
	}
}

// fullProvisionMessage builds a ProvisionMessage with real ACI + PNI keys,
// encrypts it as a ProvisionEnvelope addressed to secondaryPub, and returns
// it along with the expected ACI/PNI values for assertions.
func fullProvisionMessage(t *testing.T, secondaryPub *libsignal.PublicKey) (*provpb.ProvisionEnvelope, *provpb.ProvisionMessage) {
	t.Helper()
	primary, _ := libsignal.GenerateIdentityKeyPair()
	aciKP, _ := libsignal.GenerateIdentityKeyPair()
	pniKP, _ := libsignal.GenerateIdentityKeyPair()
	aciPub, _ := aciKP.Public.Serialize()
	aciPriv, _ := aciKP.Private.Serialize()
	pniPub, _ := pniKP.Public.Serialize()
	pniPriv, _ := pniKP.Private.Serialize()
	aci := "11111111-aaaa-bbbb-cccc-222222222222"
	pni := "33333333-dddd-eeee-ffff-444444444444"
	num := "+15558675309"
	code := "SECRET-CODE-42"
	msg := &provpb.ProvisionMessage{
		Aci:                   &aci,
		Pni:                   &pni,
		Number:                &num,
		ProvisioningCode:      &code,
		ProfileKey:            make([]byte, 32),
		ReadReceipts:          proto.Bool(true),
		AciIdentityKeyPublic:  aciPub,
		AciIdentityKeyPrivate: aciPriv,
		PniIdentityKeyPublic:  pniPub,
		PniIdentityKeyPrivate: pniPriv,
	}
	// Build the envelope using the same primitives the production code
	// uses, but here we're the "primary" side. To avoid duplicating the
	// AES-CBC + HMAC code we reuse the internal helper. Since we're an
	// external test we exercise it via the public path: encrypt and then
	// hand back. We re-implement the encrypt locally to keep the public
	// API surface clean.
	env := encryptEnvelope(t, primary.Private, primary.Public, secondaryPub, msg)
	return env, msg
}

func TestLinkRequiresOpts(t *testing.T) {
	if _, err := Link(context.Background(), LinkOptions{}); err == nil {
		t.Error("expected error for missing OnURL")
	}
	if _, err := Link(context.Background(), LinkOptions{OnURL: func(string) error { return nil }}); err == nil {
		t.Error("expected error for missing Store")
	}
}

func TestLinkHappyPath(t *testing.T) {
	fake := newFakeSignal(t, "addr-uuid")
	defer fake.Close()

	// We need to encrypt the envelope to the secondary's freshly-generated
	// ephemeral key. The public Link API generates its own keypair, so we
	// recover the public half from the linking URL inside OnURL and use it
	// to build an envelope, which we hand to the fake server via its
	// channel.
	var onceURL sync.Once
	opts := LinkOptions{
		ProvisioningURL: fake.ProvisioningURL(),
		APIBaseURL:      fake.APIBaseURL(),
		Store:           memstore.New(),
		OnURL: func(linkURL string) error {
			onceURL.Do(func() {
				u, err := url.Parse(linkURL)
				if err != nil {
					t.Errorf("link URL parse: %v", err)
					return
				}
				pubBytes, err := base64.URLEncoding.DecodeString(u.Query().Get("pub_key"))
				if err != nil {
					t.Errorf("decode pub_key: %v", err)
					return
				}
				secondaryPub, err := libsignal.DeserializePublicKey(pubBytes)
				if err != nil {
					t.Errorf("DeserializePublicKey: %v", err)
					return
				}
				env, _ := fullProvisionMessage(t, secondaryPub)
				fake.envelopeCh <- env
			})
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	la, err := Link(ctx, opts)
	if err != nil {
		t.Fatalf("Link: %v", err)
	}

	if la.ACI != fake.linkResponse.UUID || la.DeviceID != fake.linkResponse.DeviceID {
		t.Errorf("returned account: %+v, want %+v", la, fake.linkResponse)
	}
	// Verify the server saw what we expected.
	if fake.lastLinkReq.VerificationCode != "SECRET-CODE-42" {
		t.Errorf("server saw verificationCode = %q", fake.lastLinkReq.VerificationCode)
	}
	if fake.lastLinkReq.ACISignedPreKey.KeyID == 0 {
		t.Errorf("ACI signed prekey not sent")
	}
	if fake.lastLinkReq.PNIPqLastResortPreKey.KeyID == 0 {
		t.Errorf("PNI Kyber prekey not sent")
	}
	if !strings.HasPrefix(fake.lastLinkAuth, "Basic ") {
		t.Errorf("auth header = %q", fake.lastLinkAuth)
	}

	// Verify the account was persisted in the store.
	got, err := opts.Store.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount after Link: %v", err)
	}
	if got.ACI != fake.linkResponse.UUID || got.DeviceID != fake.linkResponse.DeviceID {
		t.Errorf("persisted account: %+v", got)
	}
	if len(got.Password) == 0 {
		t.Error("persisted account has empty password")
	}
}

func TestLinkSurfacesRegistrationError(t *testing.T) {
	fake := newFakeSignal(t, "addr-uuid")
	defer fake.Close()
	fake.httpSrv.Close()
	fake.httpSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusForbidden)
	}))

	opts := LinkOptions{
		ProvisioningURL: fake.ProvisioningURL(),
		APIBaseURL:      fake.httpSrv.URL,
		Store:           memstore.New(),
		OnURL: func(linkURL string) error {
			u, _ := url.Parse(linkURL)
			pubBytes, _ := base64.URLEncoding.DecodeString(u.Query().Get("pub_key"))
			secondaryPub, _ := libsignal.DeserializePublicKey(pubBytes)
			env, _ := fullProvisionMessage(t, secondaryPub)
			fake.envelopeCh <- env
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Link(ctx, opts)
	if err == nil {
		t.Fatal("expected error from registration failure")
	}
	var werr *web.Error
	if !errors.As(err, &werr) || werr.StatusCode != http.StatusForbidden {
		t.Errorf("err = %v (type %T)", err, err)
	}
}
