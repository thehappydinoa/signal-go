package provisioning

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
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
}

// Link runs the provisioning handshake to completion and returns a Session.
// The function blocks until either an envelope arrives or ctx is cancelled.
//
// Phase 1 scope: we display the linking URL via opts.OnURL and wait for the
// envelope. We do not decrypt or register yet.
func Link(ctx context.Context, opts Options) (*Session, error) {
	if opts.OnURL == nil {
		return nil, errors.New("provisioning.Link: Options.OnURL is required")
	}
	if opts.URL == "" {
		opts.URL = DefaultProvisioningURL
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "signal-go"
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
	errCh := make(chan error, 1)

	handler := ws.RequestHandlerFunc(func(_ context.Context, req *wspb.WebSocketRequestMessage) (uint32, string, []byte, error) {
		path := req.GetPath()
		verb := req.GetVerb()
		switch {
		case verb == "PUT" && path == "/v1/address":
			var addr provpb.ProvisioningAddress
			if err := proto.Unmarshal(req.GetBody(), &addr); err != nil {
				return 400, "bad ProvisioningAddress", nil, nil
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
				return 400, "bad ProvisionEnvelope", nil, nil
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
	client, err := ws.Dial(ctx, opts.URL, &ws.DialOptions{
		Header:  header,
		Handler: handler,
	})
	if err != nil {
		return nil, fmt.Errorf("provisioning.Link: dial: %w", err)
	}
	defer client.Close()

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

	// Wait for the encrypted envelope, or for the user to abort, or for the
	// ws to die under us.
	var env *provpb.ProvisionEnvelope
	select {
	case env = <-envCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	msg, err := DecryptEnvelope(ephemeral.Private, env)
	if err != nil {
		return nil, fmt.Errorf("provisioning.Link: decrypt: %w", err)
	}
	return &Session{EphemeralKey: ephemeral, Message: msg}, nil
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
