package web

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// AttachmentUploadForm is returned by GET /v4/attachments/form/upload.
type AttachmentUploadForm struct {
	CDN                  uint32
	Key                  string
	Headers              map[string]string
	SignedUploadLocation string
}

type attachmentUploadFormJSON struct {
	CDN                  uint32            `json:"cdn"`
	Key                  string            `json:"key"`
	Headers              map[string]string `json:"headers"`
	SignedUploadLocation string            `json:"signedUploadLocation"`
}

// FetchAttachmentUploadForm issues GET /v4/attachments/form/upload.
func (c *Client) FetchAttachmentUploadForm(ctx context.Context, creds Credentials, uploadLength uint64) (*AttachmentUploadForm, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.FetchAttachmentUploadForm: credentials required")
	}
	var resp attachmentUploadFormJSON
	q := url.Values{"uploadLength": []string{strconv.FormatUint(uploadLength, 10)}}
	if err := c.Do(ctx, Request{
		Method:      http.MethodGet,
		Path:        "/v4/attachments/form/upload",
		Query:       q,
		Credentials: creds,
		Out:         &resp,
	}); err != nil {
		return nil, err
	}
	if resp.Key == "" || resp.SignedUploadLocation == "" {
		return nil, errors.New("web.FetchAttachmentUploadForm: incomplete form")
	}
	return &AttachmentUploadForm{
		CDN:                  resp.CDN,
		Key:                  resp.Key,
		Headers:              resp.Headers,
		SignedUploadLocation: resp.SignedUploadLocation,
	}, nil
}

// UploadAttachment uploads encrypted attachment bytes using the form returned
// by [FetchAttachmentUploadForm]. CDN 3 uses TUS creation-with-upload; CDN 2
// posts to the signed location with form headers.
func (c *Client) UploadAttachment(ctx context.Context, form *AttachmentUploadForm, ciphertext []byte) error {
	if form == nil {
		return errors.New("web.UploadAttachment: form required")
	}
	if len(ciphertext) == 0 {
		return errors.New("web.UploadAttachment: empty ciphertext")
	}
	switch form.CDN {
	case 3:
		return c.uploadAttachmentTUS(ctx, form, ciphertext)
	default:
		return c.uploadAttachmentPOST(ctx, form, ciphertext)
	}
}

func (c *Client) uploadAttachmentTUS(ctx context.Context, form *AttachmentUploadForm, ciphertext []byte) error {
	meta := "filename " + base64.StdEncoding.EncodeToString([]byte(form.Key))
	headers := http.Header{
		"Tus-Resumable":   []string{"1.0.0"},
		"Upload-Length":   []string{strconv.Itoa(len(ciphertext))},
		"Upload-Metadata": []string{meta},
		"Content-Type":    []string{"application/offset+octet-stream"},
	}
	for k, v := range form.Headers {
		headers.Set(k, v)
	}
	return c.DoAbsolute(ctx, form.SignedUploadLocation, Request{
		Method:  http.MethodPost,
		Headers: headers,
		RawBody: ciphertext,
	})
}

func (c *Client) uploadAttachmentPOST(ctx context.Context, form *AttachmentUploadForm, ciphertext []byte) error {
	headers := http.Header{}
	for k, v := range form.Headers {
		headers.Set(k, v)
	}
	return c.DoAbsolute(ctx, form.SignedUploadLocation, Request{
		Method:  http.MethodPost,
		Headers: headers,
		RawBody: ciphertext,
	})
}

// DownloadAttachmentCDN fetches encrypted attachment bytes from a CDN.
func (c *Client) DownloadAttachmentCDN(ctx context.Context, cdnNumber uint32, cdnKey string) ([]byte, error) {
	if cdnKey == "" {
		return nil, errors.New("web.DownloadAttachmentCDN: cdnKey required")
	}
	rawURL, err := c.cdnAttachmentURL(cdnNumber, cdnKey)
	if err != nil {
		return nil, err
	}
	var out []byte
	if err := c.DoAbsolute(ctx, rawURL, Request{
		Method: http.MethodGet,
		RawOut: &out,
	}); err != nil {
		return nil, fmt.Errorf("web.DownloadAttachmentCDN: %w", err)
	}
	return out, nil
}
