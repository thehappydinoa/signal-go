package web

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DefaultBaseURL is Signal's production REST endpoint.
const DefaultBaseURL = "https://chat.signal.org"

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

// New returns a Client with sensible defaults. baseURL may be empty to use
// [DefaultBaseURL]; pass a test server URL in unit tests.
func New(baseURL, userAgent string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if userAgent == "" {
		userAgent = "signal-go"
	}
	return &Client{
		BaseURL:    baseURL,
		UserAgent:  userAgent,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
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
