package chat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/ws"
)

// DefaultChatURL is Signal's production authenticated websocket endpoint.
const DefaultChatURL = "wss://chat.signal.org/v1/websocket/"

// DialFunc is the signature for the websocket dial function. Override in
// tests to inject a fake server.
type DialFunc func(ctx context.Context, url string, opts *ws.DialOptions) (*ws.Client, error)

// InboundRequest is a server-initiated WebSocket request delivered to the
// handler. It carries the raw body (typically a serialized Envelope) plus
// metadata about the request path.
type InboundRequest struct {
	Verb string
	Path string
	Body []byte
}

// RequestHandler processes inbound server-initiated messages. The
// connection dispatches each PUT /api/v1/message and PUT
// /api/v1/queue/empty through this callback.
type RequestHandler func(ctx context.Context, req *InboundRequest)

// Options configures a [Connection].
type Options struct {
	// ACI is the account UUID (required).
	ACI string
	// DeviceID is the linked device number (required, >0).
	DeviceID uint32
	// Password is the HTTP Basic credential (required).
	Password string

	// Handler receives each inbound server request. Required.
	Handler RequestHandler

	// URL overrides the websocket endpoint. Default: [DefaultChatURL].
	URL string
	// UserAgent is sent as X-Signal-Agent. Default: "signal-go".
	UserAgent string
	// Logger for structured diagnostics. Default: slog.Default().
	Logger *slog.Logger

	// Backoff tuning (zero values use defaults).
	InitialBackoff time.Duration // default 1s
	MaxBackoff     time.Duration // default 60s

	// DialFunc overrides [ws.Dial] for testing.
	DialFunc DialFunc
}

func (o *Options) url() string {
	if o.URL != "" {
		return o.URL
	}
	return DefaultChatURL
}

func (o *Options) userAgent() string {
	if o.UserAgent != "" {
		return o.UserAgent
	}
	return "signal-go"
}

func (o *Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

func (o *Options) initialBackoff() time.Duration {
	if o.InitialBackoff > 0 {
		return o.InitialBackoff
	}
	return 1 * time.Second
}

func (o *Options) maxBackoff() time.Duration {
	if o.MaxBackoff > 0 {
		return o.MaxBackoff
	}
	return 60 * time.Second
}

func (o *Options) dial(ctx context.Context, url string, dopts *ws.DialOptions) (*ws.Client, error) {
	if o.DialFunc != nil {
		return o.DialFunc(ctx, url, dopts)
	}
	return ws.Dial(ctx, url, dopts)
}

// Connection is a reconnecting authenticated websocket that receives
// Signal envelopes from the chat service. Create one via [Connect].
type Connection struct {
	opts Options
	log  *slog.Logger

	mu       sync.Mutex
	client   *ws.Client
	closed   bool
	cancelFn context.CancelFunc

	done chan struct{}
}

// Connect establishes an authenticated chat websocket and starts the
// receive loop. The loop reconnects automatically on transient failures.
// Call [Connection.Close] to tear it down.
func Connect(ctx context.Context, opts Options) (*Connection, error) {
	if opts.ACI == "" {
		return nil, errors.New("chat.Connect: ACI is required")
	}
	if opts.DeviceID == 0 {
		return nil, errors.New("chat.Connect: DeviceID is required")
	}
	if opts.Password == "" {
		return nil, errors.New("chat.Connect: Password is required")
	}
	if opts.Handler == nil {
		return nil, errors.New("chat.Connect: Handler is required")
	}

	runCtx, cancel := context.WithCancel(ctx)
	c := &Connection{
		opts:     opts,
		log:      opts.logger().With("pkg", "chat"),
		cancelFn: cancel,
		done:     make(chan struct{}),
	}

	if err := c.connectOnce(runCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("chat.Connect: %w", err)
	}

	go c.runLoop(runCtx)
	return c, nil
}

// Close shuts down the connection and stops reconnection. Blocks until
// the run loop exits.
func (c *Connection) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		<-c.done
		return nil
	}
	c.closed = true
	c.cancelFn()
	client := c.client
	c.mu.Unlock()

	if client != nil {
		_ = client.Close()
	}
	<-c.done
	return nil
}

// Done returns a channel that is closed when the connection loop exits
// (either from [Close] or because the parent context was cancelled).
func (c *Connection) Done() <-chan struct{} { return c.done }

func (c *Connection) connectOnce(ctx context.Context) error {
	creds := fmt.Sprintf("%s.%d:%s", c.opts.ACI, c.opts.DeviceID, c.opts.Password)
	authHeader := "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))

	header := http.Header{}
	header.Set("Authorization", authHeader)
	header.Set("X-Signal-Agent", c.opts.userAgent())

	handler := ws.RequestHandlerFunc(c.handleServerRequest)

	client, err := c.opts.dial(ctx, c.opts.url(), &ws.DialOptions{
		Header:  header,
		Handler: handler,
	})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.client = client
	c.mu.Unlock()
	return nil
}

// handleServerRequest translates WebSocket requests into the simpler
// [InboundRequest] and delegates to the caller's handler.
func (c *Connection) handleServerRequest(ctx context.Context, req *wspb.WebSocketRequestMessage) (uint32, string, []byte, error) {
	verb := req.GetVerb()
	path := req.GetPath()

	c.opts.Handler(ctx, &InboundRequest{
		Verb: verb,
		Path: path,
		Body: req.GetBody(),
	})

	return 200, "OK", nil, nil
}

// runLoop monitors the current ws client and reconnects on failure. It
// exits when ctx is cancelled or [Close] is called.
func (c *Connection) runLoop(ctx context.Context) {
	defer close(c.done)

	attempt := 0
	for {
		c.mu.Lock()
		client := c.client
		closed := c.closed
		c.mu.Unlock()

		if closed {
			return
		}
		if client == nil {
			if err := c.reconnect(ctx, &attempt); err != nil {
				return
			}
			continue
		}

		select {
		case <-ctx.Done():
			c.closeClient()
			return
		case <-client.Done():
			if err := client.ReadError(); err != nil {
				c.log.Warn("websocket disconnected", "err", err)
			}
			c.closeClient()
			if err := c.reconnect(ctx, &attempt); err != nil {
				return
			}
		}
	}
}

func (c *Connection) closeClient() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		_ = c.client.Close()
		c.client = nil
	}
}

func (c *Connection) reconnect(ctx context.Context, attempt *int) error {
	for {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return errors.New("closed")
		}

		delay := c.backoffDelay(*attempt)
		c.log.Info("reconnecting", "attempt", *attempt+1, "delay", delay)
		*attempt++

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		if err := c.connectOnce(ctx); err != nil {
			c.log.Warn("reconnect failed", "err", err, "attempt", *attempt)
			continue
		}

		c.log.Info("reconnected", "attempt", *attempt)
		*attempt = 0
		return nil
	}
}

// backoffDelay computes capped exponential backoff with jitter.
func (c *Connection) backoffDelay(attempt int) time.Duration {
	base := c.opts.initialBackoff()
	maxD := c.opts.maxBackoff()

	exp := math.Pow(2, float64(attempt))
	d := time.Duration(float64(base) * exp)
	if d > maxD {
		d = maxD
	}
	// Add up to 25% jitter.
	jitter := time.Duration(rand.Int64N(int64(d/4) + 1))
	return d + jitter
}
