package signal

import (
	"context"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// DiscoveredContact is one phone number resolved via CDSI contact discovery.
type DiscoveredContact struct {
	E164 string
	ACI  string
	PNI  string
}

// DiscoverResult holds the outcome of a CDSI lookup batch.
type DiscoverResult struct {
	Contacts []DiscoveredContact
	// Token is a continuation token for incremental lookups. Empty when
	// the server did not return one.
	Token []byte
}

// DiscoverContacts resolves E.164 phone numbers to Signal ACIs via CDSI.
// Numbers must include a leading '+' and country code (e.g. "+15551234567").
//
// CDSI requires network access to Signal's contact discovery service and
// short-lived directory credentials from GET /v2/directory/auth.
func (c *Client) DiscoverContacts(ctx context.Context, e164s []string) (*DiscoverResult, error) {
	return c.discoverContacts(ctx, e164s, nil)
}

// DiscoverContactsWithToken performs a delta CDSI lookup using a token from
// a previous [DiscoverResult.Token].
func (c *Client) DiscoverContactsWithToken(ctx context.Context, e164s []string, token []byte) (*DiscoverResult, error) {
	return c.discoverContacts(ctx, e164s, token)
}

func (c *Client) discoverContacts(ctx context.Context, e164s []string, token []byte) (*DiscoverResult, error) {
	if c.webc == nil {
		return nil, errors.New("signal.DiscoverContacts: Client was opened without REST client")
	}
	if len(e164s) == 0 {
		return &DiscoverResult{}, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	auth, err := c.webc.FetchDirectoryAuth(ctx, web.Credentials{
		Username: fmt.Sprintf("%s.%d", c.acct.ACI, c.acct.DeviceID),
		Password: c.acct.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("signal.DiscoverContacts: %w", err)
	}

	tokio, connMgr, err := c.ensureCDSI()
	if err != nil {
		return nil, fmt.Errorf("signal.DiscoverContacts: %w", err)
	}

	req, err := libsignal.NewLookupRequest()
	if err != nil {
		return nil, fmt.Errorf("signal.DiscoverContacts: %w", err)
	}
	defer req.Close()
	if len(token) > 0 {
		if err := req.SetToken(token); err != nil {
			return nil, fmt.Errorf("signal.DiscoverContacts: set token: %w", err)
		}
	}
	for _, e164 := range e164s {
		if err := req.AddE164(e164); err != nil {
			return nil, fmt.Errorf("signal.DiscoverContacts: add %q: %w", e164, err)
		}
	}

	results, err := libsignal.CDSILookup(tokio, connMgr, auth.Username, auth.Password, req)
	if err != nil {
		return nil, fmt.Errorf("signal.DiscoverContacts: %w", err)
	}

	out := &DiscoverResult{
		Contacts: make([]DiscoveredContact, 0, len(results)),
	}
	for _, r := range results {
		out.Contacts = append(out.Contacts, DiscoveredContact{
			E164: r.E164String(),
			ACI:  libsignal.FormatRawUUID(r.ACI),
			PNI:  libsignal.FormatRawUUID(r.PNI),
		})
	}
	return out, nil
}

func (c *Client) ensureCDSI() (*libsignal.TokioAsyncContext, *libsignal.ConnectionManager, error) {
	c.cdsiMu.Lock()
	defer c.cdsiMu.Unlock()

	if c.cdsiTokio != nil && c.cdsiConnMgr != nil {
		return c.cdsiTokio, c.cdsiConnMgr, nil
	}
	tokio, err := libsignal.NewTokioAsyncContext()
	if err != nil {
		return nil, nil, err
	}
	userAgent := "signal-go"
	if c.webc != nil && c.webc.UserAgent != "" {
		userAgent = c.webc.UserAgent
	}
	connMgr, err := libsignal.NewConnectionManager(libsignal.NetworkEnvironmentProduction, userAgent)
	if err != nil {
		tokio.Close()
		return nil, nil, err
	}
	c.cdsiTokio = tokio
	c.cdsiConnMgr = connMgr
	return tokio, connMgr, nil
}

// closeCDSI tears down cached CDSI runtime resources. Called from Close.
func (c *Client) closeCDSI() {
	c.cdsiMu.Lock()
	defer c.cdsiMu.Unlock()
	if c.cdsiConnMgr != nil {
		c.cdsiConnMgr.Close()
		c.cdsiConnMgr = nil
	}
	if c.cdsiTokio != nil {
		c.cdsiTokio.Close()
		c.cdsiTokio = nil
	}
}
