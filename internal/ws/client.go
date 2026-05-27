package ws

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"

	wspb "github.com/thehappydinoa/signal-go/internal/proto/gen/websocketpb"
	"github.com/thehappydinoa/signal-go/internal/tlsroots"
)

// Default read/keepalive tuning for long-lived Signal websockets (chat,
// provisioning). The chat service may be silent for long stretches; a
// short read deadline causes spurious disconnects.
const (
	defaultReadIdleTimeout   = 10 * time.Minute
	defaultKeepaliveInterval = 30 * time.Second
)

// DialOptions configures [Dial].
type DialOptions struct {
	// HTTPClient is used for the upgrade handshake. nil uses http.DefaultClient.
	HTTPClient *http.Client
	// TLSConfig is applied if HTTPClient is nil. Otherwise set TLSClientConfig
	// on the client's transport yourself.
	TLSConfig *tls.Config
	// Header lets the caller add request headers (e.g. Authorization,
	// X-Signal-Agent). Required for any authenticated endpoint.
	Header http.Header
	// Handler handles inbound REQUEST messages pushed by the server.
	// May be nil if the endpoint never sends server-initiated requests.
	Handler RequestHandler

	// ReadIdleTimeout is the maximum time to wait for the next inbound data
	// frame before the read loop exits. Zero uses [defaultReadIdleTimeout].
	ReadIdleTimeout time.Duration
	// KeepaliveInterval is how often to send WebSocket pings while connected.
	// Zero uses [defaultKeepaliveInterval]. Negative disables keepalive pings.
	KeepaliveInterval time.Duration
}

// MinTLSVersion is the minimum TLS version this package ever negotiates.
// Documented for the Phase-8 audit: see docs/security/threat-model.md.
const MinTLSVersion = tls.VersionTLS12

// Dial opens a websocket connection to rawURL (wss:// or ws://) and starts a
// Client speaking Signal's WebSocketMessage envelope on top of it.
func Dial(ctx context.Context, rawURL string, opts *DialOptions) (*Client, error) {
	if opts == nil {
		opts = &DialOptions{}
	}
	httpc := opts.HTTPClient
	if httpc == nil {
		host := ""
		if u, err := url.Parse(rawURL); err == nil {
			host = tlsroots.Hostname(u.Host)
		}
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = mergeTLSConfig(opts.TLSConfig, host)
		httpc = &http.Client{Transport: tr}
	}
	conn, resp, err := websocket.Dial(ctx, rawURL, &websocket.DialOptions{
		HTTPClient: httpc,
		HTTPHeader: opts.Header,
	})
	// coder/websocket may surface a non-nil *http.Response on success (HTTP
	// 101 Switching Protocols) and on some failure paths. Either way the
	// body is empty after upgrade — close it so the connection doesn't
	// leak through net/http's pool.
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("ws.Dial: %w", err)
	}
	// Signal sends ~1 MiB messages at the high end (group keys, syncs);
	// raise the per-message limit accordingly.
	conn.SetReadLimit(8 << 20)
	return newClient(conn, opts), nil
}

// RequestHandler handles inbound [wspb.WebSocketRequestMessage]s pushed by
// the peer. Return the response status, message, and body to send back.
type RequestHandler interface {
	HandleRequest(ctx context.Context, req *wspb.WebSocketRequestMessage) (status uint32, message string, body []byte, err error)
}

// RequestHandlerFunc adapts an ordinary function to RequestHandler.
type RequestHandlerFunc func(ctx context.Context, req *wspb.WebSocketRequestMessage) (uint32, string, []byte, error)

// HandleRequest implements [RequestHandler].
func (f RequestHandlerFunc) HandleRequest(ctx context.Context, req *wspb.WebSocketRequestMessage) (uint32, string, []byte, error) {
	return f(ctx, req)
}

// Client multiplexes Signal's WebSocketMessage envelope on top of a single
// underlying ws connection. One goroutine owns reads; writes are serialised
// internally by coder/websocket.
type Client struct {
	conn    *websocket.Conn
	handler RequestHandler

	readIdle  time.Duration
	keepalive time.Duration

	nextID atomic.Uint64

	mu      sync.Mutex
	pending map[uint64]chan *wspb.WebSocketResponseMessage
	closed  bool

	readErr atomic.Pointer[error]
	done    chan struct{}
}

func newClient(conn *websocket.Conn, opts *DialOptions) *Client {
	if opts == nil {
		opts = &DialOptions{}
	}
	readIdle := opts.ReadIdleTimeout
	if readIdle == 0 {
		readIdle = defaultReadIdleTimeout
	}
	keepalive := opts.KeepaliveInterval
	if keepalive == 0 {
		keepalive = defaultKeepaliveInterval
	}
	c := &Client{
		conn:      conn,
		handler:   opts.Handler,
		readIdle:  readIdle,
		keepalive: keepalive,
		pending:   make(map[uint64]chan *wspb.WebSocketResponseMessage),
		done:      make(chan struct{}),
	}
	go c.readLoop()
	if keepalive > 0 {
		go c.keepaliveLoop()
	}
	return c
}

// Close tears down the connection and unblocks any in-flight Send calls.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
	err := c.conn.Close(websocket.StatusNormalClosure, "")
	<-c.done
	return err
}

// Done returns a channel closed when the read loop exits.
func (c *Client) Done() <-chan struct{} { return c.done }

// ReadError returns the error that ended the read loop, if any. Only
// meaningful after Done is closed.
func (c *Client) ReadError() error {
	if p := c.readErr.Load(); p != nil {
		return *p
	}
	return nil
}

// Send issues a REQUEST and waits for its matching RESPONSE. ctx aborts the
// wait but does not cancel the request on the server.
func (c *Client) Send(ctx context.Context, verb, path string, headers []string, body []byte) (*wspb.WebSocketResponseMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan *wspb.WebSocketResponseMessage, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("ws.Client: closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	msgType := wspb.WebSocketMessage_REQUEST
	out := &wspb.WebSocketMessage{
		Type: &msgType,
		Request: &wspb.WebSocketRequestMessage{
			Verb:    &verb,
			Path:    &path,
			Headers: headers,
			Body:    body,
			Id:      &id,
		},
	}
	if err := c.writeMessage(ctx, out); err != nil {
		return nil, err
	}
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, errors.New("ws.Client: closed during request")
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		if err := c.ReadError(); err != nil {
			return nil, err
		}
		return nil, errors.New("ws.Client: read loop exited")
	}
}

// Ping is a convenience over coder/websocket's keep-alive ping. Useful to
// hold the unauthenticated provisioning ws open while the user scans.
func (c *Client) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}

func (c *Client) writeMessage(ctx context.Context, msg *wspb.WebSocketMessage) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("ws.Client: marshal: %w", err)
	}
	// All Signal ws traffic is binary protobuf.
	if err := c.conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		return fmt.Errorf("ws.Client: write: %w", err)
	}
	return nil
}

func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(c.keepalive)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				_ = c.conn.Close(websocket.StatusGoingAway, "keepalive ping failed")
				return
			}
		}
	}
}

func (c *Client) readLoop() {
	defer close(c.done)
	for {
		readCtx, cancel := context.WithTimeout(context.Background(), c.readIdle)
		_, data, err := c.conn.Read(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && c.keepalive > 0 {
				// Keepalive pings prove liveness; on idle reads, keep waiting.
				continue
			}
			c.readErr.Store(&err)
			c.failPending()
			return
		}
		var msg wspb.WebSocketMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			err = fmt.Errorf("ws.Client: unmarshal: %w", err)
			c.readErr.Store(&err)
			c.failPending()
			return
		}
		switch msg.GetType() {
		case wspb.WebSocketMessage_REQUEST:
			c.dispatchRequest(msg.GetRequest())
		case wspb.WebSocketMessage_RESPONSE:
			c.dispatchResponse(msg.GetResponse())
		default:
			// UNKNOWN — RFC says ignore silently.
		}
	}
}

func (c *Client) dispatchRequest(req *wspb.WebSocketRequestMessage) {
	if req == nil {
		return
	}
	if c.handler == nil {
		c.replyRequest(req, 400, "no handler", nil)
		return
	}
	go func() {
		ctx := context.Background()
		status, message, body, err := c.handler.HandleRequest(ctx, req)
		if err != nil {
			c.replyRequest(req, 500, err.Error(), nil)
			return
		}
		c.replyRequest(req, status, message, body)
	}()
}

func (c *Client) replyRequest(req *wspb.WebSocketRequestMessage, status uint32, message string, body []byte) {
	id := req.GetId()
	msgType := wspb.WebSocketMessage_RESPONSE
	out := &wspb.WebSocketMessage{
		Type: &msgType,
		Response: &wspb.WebSocketResponseMessage{
			Id:      &id,
			Status:  &status,
			Message: &message,
			Body:    body,
		},
	}
	_ = c.writeMessage(context.Background(), out)
}

func (c *Client) dispatchResponse(resp *wspb.WebSocketResponseMessage) {
	if resp == nil {
		return
	}
	id := resp.GetId()
	c.mu.Lock()
	ch, ok := c.pending[id]
	if !ok {
		c.mu.Unlock()
		return // unsolicited or already-cancelled request
	}
	select {
	case ch <- resp:
	default:
	}
	c.mu.Unlock()
}

func (c *Client) failPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// mergeTLSConfig clones the supplied *tls.Config and enforces the
// package-wide minimum TLS version. A nil base config yields a fresh
// config with only MinVersion set. Returning a clone (rather than
// mutating the caller's config) avoids surprising callers who reuse the
// same *tls.Config for multiple dials.
func mergeTLSConfig(base *tls.Config, host string) *tls.Config {
	var cfg *tls.Config
	if base != nil {
		cfg = base.Clone()
	} else {
		cfg = &tls.Config{}
	}
	if cfg.MinVersion < MinTLSVersion {
		cfg.MinVersion = MinTLSVersion
	}
	if err := tlsroots.ApplyRootCAs(cfg, host); err != nil {
		// Embedded root is build-time constant; failure is a programming error.
		panic(err)
	}
	return cfg
}
