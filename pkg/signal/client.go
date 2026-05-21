package signal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/account"
	"github.com/thehappydinoa/signal-go/internal/chat"
	"github.com/thehappydinoa/signal-go/internal/cipher"
	"github.com/thehappydinoa/signal-go/internal/prekeymaint"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// ErrNotLinked is returned by [Open] when no linked account exists in the
// store. Call [Link] first.
var ErrNotLinked = account.ErrNotLinked

// Decryptor transforms raw encrypted envelope content into a plaintext
// Content protobuf via libsignal ([cipher.EnvelopeDecryptor] by default).
type Decryptor interface {
	// Decrypt takes an envelope and returns the decrypted Content bytes
	// plus the verified sender address. For sealed-sender envelopes the
	// sender is extracted during decryption; for non-sealed envelopes it
	// comes from the envelope's plaintext fields.
	Decrypt(ctx context.Context, env *sspb.Envelope) (content []byte, sender string, senderDevice uint32, err error)
}

// DecryptorFunc adapts a plain function to [Decryptor].
type DecryptorFunc func(ctx context.Context, env *sspb.Envelope) ([]byte, string, uint32, error)

// Decrypt implements [Decryptor].
func (f DecryptorFunc) Decrypt(ctx context.Context, env *sspb.Envelope) ([]byte, string, uint32, error) {
	return f(ctx, env)
}

// OpenOptions configures [Open].
type OpenOptions struct {
	// AccountStore is the account-level persistence backend (required).
	AccountStore account.Store

	// SignalStores provides per-peer session/identity/prekey persistence
	// for the ACI namespace (required). The decrypt pipeline uses these
	// through the libsignal cgo callback bridge.
	SignalStores store.SignalStores

	// ChatURL overrides the authenticated websocket endpoint. Default:
	// production Signal.
	ChatURL string
	// APIBaseURL overrides the REST endpoint for prekey top-up. Default:
	// production Signal.
	APIBaseURL string
	// UserAgent is sent in X-Signal-Agent headers. Default: "signal-go".
	UserAgent string
	// Logger for structured diagnostics. Default: slog.Default().
	Logger *slog.Logger

	// EventBufferSize is the capacity of the [Client.Events] channel.
	// Default: 256.
	EventBufferSize int

	// Decryptor overrides the default libsignal-backed decryptor built
	// from the linked account + SignalStores. Useful for tests.
	Decryptor Decryptor

	// DisablePreKeyMaintenance turns off automatic PUT /v2/keys top-up
	// after inbound prekey decrypts. Default: enabled when using the
	// built-in [cipher.EnvelopeDecryptor].
	DisablePreKeyMaintenance bool

	// DialFunc overrides websocket dial for testing.
	DialFunc chat.DialFunc
}

func (o *OpenOptions) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

func (o *OpenOptions) eventBufferSize() int {
	if o.EventBufferSize > 0 {
		return o.EventBufferSize
	}
	return 256
}

// Client is a connected, receiving Signal device. Create one with [Open].
type Client struct {
	acct   *account.Account
	conn   *chat.Connection
	events chan Event
	log    *slog.Logger
	dec    Decryptor
}

// Open loads a previously-linked account from opts.AccountStore and
// connects to the Signal chat websocket. It returns a [Client] that
// streams typed events through [Client.Events].
//
// If no account has been linked, Open returns [ErrNotLinked].
func Open(ctx context.Context, opts OpenOptions) (*Client, error) {
	if opts.AccountStore == nil {
		return nil, errors.New("signal.Open: AccountStore is required")
	}
	if opts.SignalStores == nil {
		return nil, errors.New("signal.Open: SignalStores is required")
	}

	acct, err := opts.AccountStore.LoadAccount()
	if err != nil {
		return nil, fmt.Errorf("signal.Open: %w", err)
	}

	events := make(chan Event, opts.eventBufferSize())
	log := opts.logger()

	dec := opts.Decryptor
	if dec == nil {
		libDec, err := cipher.NewEnvelopeDecryptor(acct, opts.SignalStores)
		if err != nil {
			return nil, fmt.Errorf("signal.Open: %w", err)
		}
		if !opts.DisablePreKeyMaintenance {
			webc := web.New(opts.APIBaseURL, opts.UserAgent)
			libDec.SetPreKeyMaintainer(prekeymaint.NewMaintainer(
				opts.AccountStore,
				opts.SignalStores,
				webc,
			))
		}
		dec = libDec
	}

	c := &Client{
		acct:   acct,
		events: events,
		log:    log,
		dec:    dec,
	}

	conn, err := chat.Connect(ctx, chat.Options{
		ACI:       acct.ACI,
		DeviceID:  acct.DeviceID,
		Password:  acct.Password,
		Handler:   c.handleInbound,
		URL:       opts.ChatURL,
		UserAgent: opts.UserAgent,
		Logger:    log,
		DialFunc:  opts.DialFunc,
	})
	if err != nil {
		return nil, fmt.Errorf("signal.Open: %w", err)
	}
	c.conn = conn
	return c, nil
}

// Events returns the channel on which typed events arrive. The channel
// is closed when [Client.Close] is called.
//
// Callers should receive from this channel in a loop:
//
//	for ev := range client.Events() {
//	    switch e := ev.(type) {
//	    case *signal.MessageEvent:  ...
//	    case *signal.ReceiptEvent:  ...
//	    case *signal.TypingEvent:   ...
//	    }
//	}
func (c *Client) Events() <-chan Event { return c.events }

// Close shuts down the websocket connection and closes the [Events]
// channel. Blocks until teardown completes.
func (c *Client) Close() error {
	err := c.conn.Close()
	close(c.events)
	return err
}

// Account returns the linked account metadata.
func (c *Client) Account() *account.Account { return c.acct }

// Done returns a channel closed when the underlying connection exits.
func (c *Client) Done() <-chan struct{} { return c.conn.Done() }

// handleInbound processes a server-pushed websocket request.
func (c *Client) handleInbound(ctx context.Context, req *chat.InboundRequest) {
	switch {
	case req.Verb == "PUT" && req.Path == "/api/v1/message":
		c.processEnvelope(ctx, req.Body)
	case req.Verb == "PUT" && req.Path == "/api/v1/queue/empty":
		c.emit(&QueueEmptyEvent{})
	default:
		c.log.Debug("unhandled inbound request", "verb", req.Verb, "path", req.Path)
	}
}

func (c *Client) processEnvelope(ctx context.Context, data []byte) {
	var env sspb.Envelope
	if err := proto.Unmarshal(data, &env); err != nil {
		c.log.Error("envelope unmarshal failed", "err", err)
		return
	}

	sender := env.GetSourceServiceId()
	senderDevice := env.GetSourceDeviceId()
	envTS := msToTime(env.GetClientTimestamp())
	srvTS := msToTime(env.GetServerTimestamp())

	contentBytes, decSender, decDevice, err := c.dec.Decrypt(ctx, &env)
	if err != nil {
		c.emit(&DecryptionErrorEvent{
			Sender:       sender,
			SenderDevice: senderDevice,
			Timestamp:    envTS,
			Err:          fmt.Errorf("signal: decrypt %s: %w", env.GetType().String(), err),
		})
		return
	}

	if decSender != "" {
		sender = decSender
	}
	if decDevice != 0 {
		senderDevice = decDevice
	}

	var content sspb.Content
	if err := proto.Unmarshal(contentBytes, &content); err != nil {
		c.emit(&DecryptionErrorEvent{
			Sender:       sender,
			SenderDevice: senderDevice,
			Timestamp:    envTS,
			Err:          fmt.Errorf("signal: unmarshal Content: %w", err),
		})
		return
	}

	c.dispatchContent(sender, senderDevice, envTS, srvTS, &content)
}

func (c *Client) emit(ev Event) {
	select {
	case c.events <- ev:
	default:
		c.log.Warn(
			"event channel full, dropping event",
			"type", fmt.Sprintf("%T", ev),
		)
	}
}

// passthroughDecryptor treats envelope content as already-decrypted
// Content protobuf bytes. This is the default until the libsignal
// decrypt wrappers land.
type passthroughDecryptor struct{}

func (passthroughDecryptor) Decrypt(_ context.Context, env *sspb.Envelope) ([]byte, string, uint32, error) {
	content := env.GetContent()
	if len(content) == 0 {
		return nil, "", 0, errors.New("empty envelope content")
	}

	if env.GetType() == sspb.Envelope_PLAINTEXT_CONTENT {
		if len(content) < 2 {
			return nil, "", 0, errors.New("plaintext content too short")
		}
		return content[1:], env.GetSourceServiceId(), env.GetSourceDeviceId(), nil
	}

	return content, env.GetSourceServiceId(), env.GetSourceDeviceId(), nil
}
