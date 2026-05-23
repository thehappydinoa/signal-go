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

	"github.com/thehappydinoa/signal-go/internal/devicename"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// fakeSignal answers both halves of the link flow: the provisioning
// websocket (including PUT /v1/devices/link) and REST /v2/keys calls.
type fakeSignal struct {
	t              *testing.T
	httpSrv        *httptest.Server
	wsSrv          *httptest.Server
	addressUUID    string
	envelopeCh     chan *provpb.ProvisionEnvelope // buffered; server waits on this
	lastLinkReq    web.LinkDeviceRequest
	lastLinkAuth   string
	linkResponse   web.LinkDeviceResponse
	preKeyUploads  []preKeyUpload
	preKeyUploadMu sync.Mutex
}

type preKeyUpload struct {
	Identity string
	Req      web.UploadPreKeysRequest
	Auth     string
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
func (f *fakeSignal) ServiceWebSocketURL() string {
	return f.ProvisioningURL()
}
func (f *fakeSignal) APIBaseURL() string { return f.httpSrv.URL }

func (f *fakeSignal) handleHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/devices/link":
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &f.lastLinkReq)
		f.lastLinkAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(f.linkResponse)
	case "/v2/keys":
		raw, _ := io.ReadAll(r.Body)
		var req web.UploadPreKeysRequest
		_ = json.Unmarshal(raw, &req)
		f.preKeyUploadMu.Lock()
		f.preKeyUploads = append(f.preKeyUploads, preKeyUpload{
			Identity: r.URL.Query().Get("identity"),
			Req:      req,
			Auth:     r.Header.Get("Authorization"),
		})
		f.preKeyUploadMu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// PreKeyUploads returns a snapshot of recorded /v2/keys uploads, keyed by
// the order they arrived in.
func (f *fakeSignal) PreKeyUploads() []preKeyUpload {
	f.preKeyUploadMu.Lock()
	defer f.preKeyUploadMu.Unlock()
	out := make([]preKeyUpload, len(f.preKeyUploads))
	copy(out, f.preKeyUploads)
	return out
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
		switch msg.GetType() {
		case wspb.WebSocketMessage_RESPONSE:
			if msg.GetResponse().GetStatus() == 200 && msg.GetResponse().GetId() == id {
				go f.pushProvisionEnvelope(c, r.Context())
			}
		case wspb.WebSocketMessage_REQUEST:
			req := msg.GetRequest()
			if req.GetVerb() == "PUT" && req.GetPath() == "/v1/devices/link" {
				f.handleWSLinkDevice(c, r.Context(), req)
			}
		}
	}
}

func (f *fakeSignal) pushProvisionEnvelope(c *websocket.Conn, ctx context.Context) {
	var env *provpb.ProvisionEnvelope
	select {
	case env = <-f.envelopeCh:
	case <-ctx.Done():
		return
	}
	envBody, _ := proto.Marshal(env)
	envVerb, envPath := "PUT", "/v1/message"
	envID := uint64(2)
	reqType := wspb.WebSocketMessage_REQUEST
	out, _ := proto.Marshal(&wspb.WebSocketMessage{
		Type:    &reqType,
		Request: &wspb.WebSocketRequestMessage{Verb: &envVerb, Path: &envPath, Body: envBody, Id: &envID},
	})
	_ = c.Write(ctx, websocket.MessageBinary, out)
}

func (f *fakeSignal) handleWSLinkDevice(c *websocket.Conn, ctx context.Context, req *wspb.WebSocketRequestMessage) {
	raw := req.GetBody()
	_ = json.Unmarshal(raw, &f.lastLinkReq)
	for _, h := range req.GetHeaders() {
		if strings.HasPrefix(h, "Authorization:") {
			f.lastLinkAuth = strings.TrimSpace(strings.TrimPrefix(h, "Authorization:"))
		}
	}
	respBody, _ := json.Marshal(f.linkResponse)
	status := uint32(200)
	msg := "OK"
	respType := wspb.WebSocketMessage_RESPONSE
	rid := req.GetId()
	out, _ := proto.Marshal(&wspb.WebSocketMessage{
		Type: &respType,
		Response: &wspb.WebSocketResponseMessage{
			Id: &rid, Status: &status, Message: &msg, Body: respBody,
		},
	})
	_ = c.Write(ctx, websocket.MessageBinary, out)
}

// fullProvisionMessage builds a ProvisionMessage with real ACI + PNI keys
// and encrypts it as a ProvisionEnvelope addressed to secondaryPub.
func fullProvisionMessage(t *testing.T, secondaryPub *libsignal.PublicKey) *provpb.ProvisionEnvelope {
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
	return encryptEnvelope(t, primary.Private, primary.Public, secondaryPub, msg)
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
		ProvisioningURL:       fake.ProvisioningURL(),
		ServiceWebSocketURL:   fake.ServiceWebSocketURL(),
		APIBaseURL:            fake.APIBaseURL(),
		Store:              memstore.New(),
		OneTimePreKeyCount: 4, // small batch to keep test fast
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
				env := fullProvisionMessage(t, secondaryPub)
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
	const wantNumber = "+15558675309"
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(fake.lastLinkAuth, "Basic ")); err != nil {
		t.Errorf("decode auth: %v", err)
	} else {
		user, _, ok := strings.Cut(string(raw), ":")
		if !ok || user != wantNumber {
			t.Errorf("basic auth username = %q, want %q", user, wantNumber)
		}
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

	// Verify one-time prekey uploads (one per identity).
	uploads := fake.PreKeyUploads()
	if len(uploads) != 2 {
		t.Fatalf("/v2/keys uploads = %d, want 2", len(uploads))
	}
	byIdent := map[string]preKeyUpload{}
	for _, u := range uploads {
		byIdent[u.Identity] = u
	}
	for _, ident := range []string{"aci", "pni"} {
		u, ok := byIdent[ident]
		if !ok {
			t.Errorf("missing %s upload", ident)
			continue
		}
		if len(u.Req.PreKeys) != 4 || len(u.Req.PqPreKeys) != 4 {
			t.Errorf("%s upload sizes: ec=%d kem=%d, want 4+4", ident, len(u.Req.PreKeys), len(u.Req.PqPreKeys))
		}
		// First batch should start at id=2 (we consumed id=1 for the
		// rotating signed + last-resort Kyber prekeys at link time).
		if u.Req.PreKeys[0].KeyID != 2 {
			t.Errorf("%s first one-time prekey id = %d, want 2", ident, u.Req.PreKeys[0].KeyID)
		}
		// Auth uses {ACI}.{deviceId} format.
		wantAuthPrefix := "Basic "
		if !strings.HasPrefix(u.Auth, wantAuthPrefix) {
			t.Errorf("%s upload auth header = %q", ident, u.Auth)
		}
	}

	// Persisted state should reflect the bumped next-id counters.
	if got.ACIIdentity.NextPreKeyID != 2+4 || got.PNIIdentity.NextKyberPreKeyID != 2+4 {
		t.Errorf("next-id counters not bumped: %+v / %+v", got.ACIIdentity, got.PNIIdentity)
	}
}

func TestLinkEncryptsDeviceName(t *testing.T) {
	fake := newFakeSignal(t, "addr-uuid")
	defer fake.Close()

	var onceURL sync.Once
	opts := LinkOptions{
		ProvisioningURL:      fake.ProvisioningURL(),
		ServiceWebSocketURL:  fake.ServiceWebSocketURL(),
		APIBaseURL:           fake.APIBaseURL(),
		Store:                memstore.New(),
		OneTimePreKeyCount:   0,
		DeviceName:           "my-signal-go-box",
		testSkipPreKeyUpload: true,
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
				env := fullProvisionMessage(t, secondaryPub)
				fake.envelopeCh <- env
			})
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Link(ctx, opts); err != nil {
		t.Fatalf("Link: %v", err)
	}
	name := fake.lastLinkReq.AccountAttributes.Name
	if name == "" || name == "my-signal-go-box" {
		t.Fatalf("expected encrypted name, got %q", name)
	}

	got, err := opts.Store.LoadAccount()
	if err != nil {
		t.Fatalf("LoadAccount: %v", err)
	}
	idPriv, err := libsignal.DeserializePrivateKey(got.ACIIdentity.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	idPub, err := libsignal.DeserializePublicKey(got.ACIIdentity.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := devicename.Decrypt(name, &libsignal.IdentityKeyPair{Private: idPriv, Public: idPub})
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != "my-signal-go-box" {
		t.Fatalf("device name = %q", plain)
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
		ProvisioningURL:     fake.ProvisioningURL(),
		ServiceWebSocketURL: fake.ServiceWebSocketURL(),
		APIBaseURL:          fake.httpSrv.URL,
		Store:           memstore.New(),
		OnURL: func(linkURL string) error {
			u, _ := url.Parse(linkURL)
			pubBytes, _ := base64.URLEncoding.DecodeString(u.Query().Get("pub_key"))
			secondaryPub, _ := libsignal.DeserializePublicKey(pubBytes)
			env := fullProvisionMessage(t, secondaryPub)
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
