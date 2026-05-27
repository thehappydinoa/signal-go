package web

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/thehappydinoa/signal-go/internal/tlsroots"
	"github.com/thehappydinoa/signal-go/internal/web/useragent"
)

// DefaultBaseURL is Signal's production REST endpoint.
const DefaultBaseURL = "https://chat.signal.org"

// MinTLSVersion is the minimum TLS version this package ever negotiates.
// Documented for the Phase-8 audit: see docs/security/threat-model.md.
const MinTLSVersion = tls.VersionTLS12

// Client is an HTTP client targeting one Signal service endpoint.
//
// Goroutine-safe: net/http.Client underneath.
type Client struct {
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
	// CDNHosts overrides [DefaultCDNHosts] for tests.
	CDNHosts map[uint32]string
}

// Options tweaks the default HTTP behaviour. The zero value is fine for
// production use; tests pass a [Options.PinnedRootCAs] of nil to keep the
// system trust store.
type Options struct {
	// Timeout overrides the 30s default request timeout.
	Timeout time.Duration
	// PinnedRootCAs, when non-nil, replaces the trust store with the
	// supplied pool. Opt-in pinning is documented in
	// docs/security/threat-model.md. When nil and the client base URL is a
	// *.signal.org host, [tlsroots.ApplyRootCAs] pins Signal's private root.
	PinnedRootCAs *x509.CertPool
	// InsecureSkipVerify disables certificate verification. Tests only.
	// Production code MUST leave this false — the field exists so the
	// test harness can stand up an httptest.NewTLSServer without a real
	// CA. A run-time guard in [New] panics if this is set while the
	// BaseURL points at chat.signal.org.
	InsecureSkipVerify bool
}

// New returns a Client with sensible defaults. baseURL may be empty to use
// [DefaultBaseURL]; pass a test server URL in unit tests.
func New(baseURL, userAgent string) *Client {
	return NewWithOptions(baseURL, userAgent, Options{})
}

// NewWithOptions returns a Client configured per opts. All TLS dials use
// MinVersion=TLS 1.2 explicitly (see [MinTLSVersion]); callers may
// optionally pin a CA pool via [Options.PinnedRootCAs].
func NewWithOptions(baseURL, userAgent string, opts Options) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if userAgent == "" {
		userAgent = useragent.Resolve(useragent.SignalGo, "", useragent.Options{})
	}
	if opts.InsecureSkipVerify && isProductionBaseURL(baseURL) {
		panic("web: InsecureSkipVerify must never be set against the production base URL")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	host := ""
	if u, err := url.Parse(baseURL); err == nil {
		host = tlsroots.Hostname(u.Host)
	}
	return &Client{
		BaseURL:    baseURL,
		UserAgent:  userAgent,
		HTTPClient: newHTTPClient(timeout, opts, host),
	}
}

// newHTTPClient returns an *http.Client whose transport enforces an
// explicit TLS minimum version and (optionally) a pinned CA pool. Cloning
// [http.DefaultTransport] preserves Go's connection-pool and HTTP/2
// upgrade behaviour.
func newHTTPClient(timeout time.Duration, opts Options, host string) *http.Client {
	tr, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{TLSClientConfig: tlsConfigFromOptions(opts, host)},
		}
	}
	clone := tr.Clone()
	clone.TLSClientConfig = tlsConfigFromOptions(opts, host)
	return &http.Client{Timeout: timeout, Transport: clone}
}

func tlsConfigFromOptions(opts Options, host string) *tls.Config {
	cfg := &tls.Config{
		MinVersion:         MinTLSVersion,
		RootCAs:            opts.PinnedRootCAs,
		InsecureSkipVerify: opts.InsecureSkipVerify, //nolint:gosec // guarded in NewWithOptions, used only by tests
	}
	if err := tlsroots.ApplyRootCAs(cfg, host); err != nil {
		panic(err)
	}
	return cfg
}

// Credentials carries the username + password used for HTTP Basic auth.
// Empty values disable the Authorization header (some endpoints, like
// /v1/accounts/attestation, are unauthenticated).
type Credentials struct {
	Username string
	Password string
}

// Header returns the value for the Authorization header, or empty if
// neither field is set.
func (c Credentials) Header() string {
	if c.Username == "" && c.Password == "" {
		return ""
	}
	raw := c.Username + ":" + c.Password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// Request is a tagged HTTP call. Body, if non-nil, is JSON-encoded and the
// Content-Type header is set to application/json. RawBody, if non-nil, is sent
// as-is (Content-Type must be set in Headers). Out, if non-nil, is JSON-decoded
// from the response body on 2xx responses.
type Request struct {
	Method      string
	Path        string      // e.g. "/v1/devices/link"
	Query       url.Values  // optional
	Headers     http.Header // optional, merged after auth + user-agent
	Credentials Credentials // optional
	Body        any         // JSON-encoded if non-nil
	RawBody     []byte      // sent as-is when Body is nil
	Out         any         // JSON-decoded if non-nil
	RawOut      *[]byte     // raw body on 2xx, skips JSON decode
}

// Error is returned for non-2xx HTTP responses.
type Error struct {
	StatusCode int
	Status     string
	Body       []byte
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	preview := e.Body
	if len(preview) > 256 {
		preview = preview[:256]
	}
	return fmt.Sprintf("web: HTTP %s: %s", e.Status, preview)
}

// Do executes the request and decodes the response.
func (c *Client) Do(ctx context.Context, req Request) error {
	full, err := c.buildURL(req.Path, req.Query)
	if err != nil {
		return err
	}

	var bodyReader io.Reader
	switch {
	case req.Body != nil:
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return fmt.Errorf("web: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	case len(req.RawBody) > 0:
		bodyReader = bytes.NewReader(req.RawBody)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, full, bodyReader)
	if err != nil {
		return fmt.Errorf("web: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", c.UserAgent)
	httpReq.Header.Set("X-Signal-Agent", c.UserAgent)
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if h := req.Credentials.Header(); h != "" {
		httpReq.Header.Set("Authorization", h)
	}
	for k, vs := range req.Headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("web: do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("web: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       body,
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	if req.RawOut != nil {
		*req.RawOut = append((*req.RawOut)[:0], body...)
		return nil
	}
	if req.Out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, req.Out); err != nil {
		return fmt.Errorf("web: decode response: %w", err)
	}
	return nil
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	d := time.Until(when)
	if d < 0 {
		return 0
	}
	return d
}

// DoAbsolute executes an HTTP request against an absolute URL (CDN uploads,
// signed upload locations). Auth and User-Agent match [Do].
func (c *Client) DoAbsolute(ctx context.Context, rawURL string, req Request) error {
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, rawURL, nil)
	if err != nil {
		return fmt.Errorf("web: build absolute request: %w", err)
	}
	var bodyReader io.Reader
	switch {
	case req.Body != nil:
		raw, err := json.Marshal(req.Body)
		if err != nil {
			return fmt.Errorf("web: marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	case len(req.RawBody) > 0:
		bodyReader = bytes.NewReader(req.RawBody)
	}
	if bodyReader != nil {
		httpReq.Body = io.NopCloser(bodyReader)
	}
	httpReq.Header.Set("User-Agent", c.UserAgent)
	httpReq.Header.Set("X-Signal-Agent", c.UserAgent)
	if req.Body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if h := req.Credentials.Header(); h != "" {
		httpReq.Header.Set("Authorization", h)
	}
	for k, vs := range req.Headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("web: do absolute: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("web: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &Error{StatusCode: resp.StatusCode, Status: resp.Status, Body: body}
	}
	if req.RawOut != nil {
		*req.RawOut = append((*req.RawOut)[:0], body...)
		return nil
	}
	if req.Out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, req.Out); err != nil {
		return fmt.Errorf("web: decode response: %w", err)
	}
	return nil
}

func (c *Client) buildURL(path string, query url.Values) (string, error) {
	if path == "" || path[0] != '/' {
		return "", errors.New("web: path must start with /")
	}
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return "", fmt.Errorf("web: parse url: %w", err)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
}

func isProductionBaseURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return tlsroots.Hostname(u.Host) == "chat.signal.org"
}
