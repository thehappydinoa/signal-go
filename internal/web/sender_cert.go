package web

import (
	"context"
	"fmt"
	"net/http"
)

// SenderCertificateResponse is the JSON body returned by
// GET /v1/certificate/delivery.
type SenderCertificateResponse struct {
	Certificate string `json:"certificate"` // base64-encoded serialized SenderCertificate
}

// FetchSenderCertificate issues GET /v1/certificate/delivery and returns the
// raw certificate bytes. The caller is responsible for deserialization and
// expiry checking.
func (c *Client) FetchSenderCertificate(ctx context.Context, creds Credentials) ([]byte, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, fmt.Errorf("web.FetchSenderCertificate: credentials required")
	}
	var resp SenderCertificateResponse
	if err := c.Do(ctx, Request{
		Method:      http.MethodGet,
		Path:        "/v1/certificate/delivery",
		Credentials: creds,
		Out:         &resp,
	}); err != nil {
		return nil, fmt.Errorf("web.FetchSenderCertificate: %w", err)
	}
	if resp.Certificate == "" {
		return nil, fmt.Errorf("web.FetchSenderCertificate: server returned empty certificate")
	}
	data, err := DecodeBase64(resp.Certificate)
	if err != nil {
		return nil, fmt.Errorf("web.FetchSenderCertificate: decode certificate: %w", err)
	}
	return data, nil
}
