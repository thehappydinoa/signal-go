package provisioning

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/debugsession"
	"github.com/thehappydinoa/signal-go/internal/web/useragent"
	"github.com/thehappydinoa/signal-go/internal/ws"
)

// DefaultProvisioningURL is Signal's production provisioning websocket
// endpoint. Override via [Options.URL] in tests.
const DefaultProvisioningURL = "wss://chat.signal.org/v1/websocket/provisioning/"

// Capability strings advertised in the linking URL. The set the primary
// will see determines which sync messages it sends post-link. We start with
// the conservative minimum and grow as the higher layers learn to handle
// each one.
var defaultCapabilities = []string{}

// Options configures [Link].
type Options struct {
	// URL overrides the websocket endpoint. Defaults to
	// [DefaultProvisioningURL].
	URL string
	// UserAgent is sent as the X-Signal-Agent header. Defaults to "signal-go".
	UserAgent string
	// Capabilities advertised in the link URL. See [defaultCapabilities].
	Capabilities []string
	// EphemeralKey lets tests inject a known keypair. Production code leaves
	// this nil and [Link] generates a fresh one.
	EphemeralKey *libsignal.IdentityKeyPair
	// OnURL is called once the linking URL is ready. Typical implementations
	// print it or render an ANSI QR code. Required.
	OnURL func(linkURL string) error
	// AfterDecrypt, if set, runs on the open provisioning websocket
	// immediately after the envelope is decrypted. Production link flows
	// should register via PUT /v1/devices/link here — Signal closes the
	// provisioning socket shortly after the envelope is delivered.
	AfterDecrypt func(ctx context.Context, conn *ws.Client, msg *provpb.ProvisionMessage) error
}

// Session is the result of [Link]: a successful pairing handshake that
// yielded a decrypted [ProvisionMessage] from the user's primary device.
type Session struct {
	// EphemeralKey is the keypair we generated for the handshake. Kept on
	// the session for completeness; higher layers usually don't need it.
	EphemeralKey *libsignal.IdentityKeyPair
	// Message holds the decoded ProvisionMessage with the linked account's
	// ACI/PNI identity keys, UUIDs, number, and provisioning code.
	Message *provpb.ProvisionMessage
	// Conn is the open provisioning websocket. Callers must [ws.Client.Close]
	// it after finishing registration (e.g. PUT /v1/devices/link on this conn).
	Conn *ws.Client
}

// Link runs the provisioning handshake to completion and returns a Session.
// The function blocks until either an envelope arrives or ctx is cancelled.
//
// Phase 1 scope: we display the linking URL via opts.OnURL and wait for
// the envelope. We do not decrypt or register yet.
//
// Cyclomatic complexity is high because this is a linear protocol with
// several select arms; factoring into helpers hides the sequence the
// reader needs to follow.
//
//nolint:gocyclo
func Link(ctx context.Context, opts Options) (*Session, error) {
	if opts.OnURL == nil {
		return nil, errors.New("provisioning.Link: Options.OnURL is required")
	}
	if opts.URL == "" {
		opts.URL = DefaultProvisioningURL
	}
	if opts.UserAgent == "" {
		opts.UserAgent = useragent.Resolve(useragent.SignalGo, "", useragent.Options{})
	}
	if opts.Capabilities == nil {
		opts.Capabilities = defaultCapabilities
	}

	ephemeral := opts.EphemeralKey
	if ephemeral == nil {
		kp, err := libsignal.GenerateIdentityKeyPair()
		if err != nil {
			return nil, fmt.Errorf("provisioning.Link: generate keypair: %w", err)
		}
		ephemeral = kp
	}
	pubBytes, err := ephemeral.Public.Serialize()
	if err != nil {
		return nil, fmt.Errorf("provisioning.Link: serialize public key: %w", err)
	}

	addrCh := make(chan string, 1)
	envCh := make(chan *provpb.ProvisionEnvelope, 1)
	msgReadyCh := make(chan *provpb.ProvisionMessage, 1)
	errCh := make(chan error, 1)

	var client *ws.Client
	handler := ws.RequestHandlerFunc(func(hctx context.Context, req *wspb.WebSocketRequestMessage) (uint32, string, []byte, error) {
		path := req.GetPath()
		verb := req.GetVerb()
		switch {
		case verb == "PUT" && path == "/v1/address":
			var addr provpb.ProvisioningAddress
			if err := proto.Unmarshal(req.GetBody(), &addr); err != nil {
				// Translate parse failure to HTTP 400 for the peer; we do
				// not want the ws handler to surface a Go error (which
				// would close the connection).
				return 400, "bad ProvisioningAddress", nil, nil //nolint:nilerr
			}
			if addr.GetAddress() == "" {
				return 400, "empty address", nil, nil
			}
			select {
			case addrCh <- addr.GetAddress():
			default:
				// duplicate; reply 200 anyway since we already have one.
			}
			return 200, "OK", nil, nil

		case verb == "PUT" && path == "/v1/message":
			var env provpb.ProvisionEnvelope
			if err := proto.Unmarshal(req.GetBody(), &env); err != nil {
				return 400, "bad ProvisionEnvelope", nil, nil //nolint:nilerr
			}
			// #region agent log
			debugsession.Log("H2", "provisioning/provisioning.go:handler", "provision message received", map[string]any{
				"hasAfterDecrypt": opts.AfterDecrypt != nil,
			})
			// #endregion
			if opts.AfterDecrypt != nil {
				msg, err := DecryptEnvelope(ephemeral.Private, &env)
				if err != nil {
					return 400, "bad ProvisionEnvelope", nil, nil //nolint:nilerr
				}
				// #region agent log
				debugsession.Log("H1", "provisioning/provisioning.go:handler", "before AfterDecrypt in handler", map[string]any{
					"connClosed": client.Closed(), "runId": "post-fix",
				})
				// #endregion
				if err := opts.AfterDecrypt(hctx, client, msg); err != nil {
					return 500, err.Error(), nil, nil
				}
				// #region agent log
				debugsession.Log("H4", "provisioning/provisioning.go:handler", "register done, replying 200 to message", map[string]any{
					"connClosed": client.Closed(), "runId": "post-fix",
				})
				// #endregion
				select {
				case msgReadyCh <- msg:
				default:
				}
				return 200, "OK", nil, nil
			}
			select {
			case envCh <- &env:
			default:
			}
			return 200, "OK", nil, nil

		default:
			return 404, "not found", nil, nil
		}
	})

	header := newSignalHeaders(opts.UserAgent)
	client, err = ws.Dial(ctx, opts.URL, &ws.DialOptions{
		Header:  header,
		Handler: handler,
	})
	if err != nil {
		return nil, fmt.Errorf("provisioning.Link: dial: %w", err)
	}

	// Watch the read loop for premature death.
	go func() {
		<-client.Done()
		if e := client.ReadError(); e != nil {
			select {
			case errCh <- fmt.Errorf("provisioning.Link: ws read: %w", e):
			default:
			}
		}
	}()

	// Wait for the server to send us the address.
	var address string
	select {
	case address = <-addrCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	linkURL, err := buildLinkURL(address, pubBytes, opts.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("provisioning.Link: build url: %w", err)
	}
	if err := opts.OnURL(linkURL); err != nil {
		return nil, fmt.Errorf("provisioning.Link: OnURL: %w", err)
	}

	// Hold the provisioning socket open while the user scans (can take minutes).
	pingCtx, stopPing := context.WithCancel(ctx)
	defer stopPing()
	go keepAlive(pingCtx, client)

	// Wait for the encrypted envelope, or for the user to abort, or for the
	// ws to die under us.
	var msg *provpb.ProvisionMessage
	if opts.AfterDecrypt != nil {
		select {
		case msg = <-msgReadyCh:
		case err := <-errCh:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		var env *provpb.ProvisionEnvelope
		select {
		case env = <-envCh:
		case err := <-errCh:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		var err error
		msg, err = DecryptEnvelope(ephemeral.Private, env)
		if err != nil {
			return nil, fmt.Errorf("provisioning.Link: decrypt: %w", err)
		}
	}
	return &Session{EphemeralKey: ephemeral, Message: msg, Conn: client}, nil
}

const provisioningKeepAliveInterval = 30 * time.Second

func keepAlive(ctx context.Context, client *ws.Client) {
	ticker := time.NewTicker(provisioningKeepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Ping(ctx); err != nil {
				return
			}
		}
	}
}

// buildLinkURL returns the sgnl://linkdevice URL that the Signal mobile
// app will recognise after scanning the QR.
//
// Schema observed in current Signal-Android / Signal-Desktop:
//
//	sgnl://linkdevice?uuid=<address>&pub_key=<base64url(publicKey)>
//	  [&capabilities=cap1,cap2,...]
//
// The base64url encoding is *padded* per current upstream clients.
func buildLinkURL(address string, publicKey []byte, capabilities []string) (string, error) {
	if address == "" {
		return "", errors.New("empty provisioning address")
	}
	if len(publicKey) == 0 {
		return "", errors.New("empty public key")
	}
	v := url.Values{}
	v.Set("uuid", address)
	v.Set("pub_key", base64.URLEncoding.EncodeToString(publicKey))
	if len(capabilities) > 0 {
		v.Set("capabilities", strings.Join(capabilities, ","))
	}
	return "sgnl://linkdevice?" + v.Encode(), nil
}
