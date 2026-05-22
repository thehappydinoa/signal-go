package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TransferArchiveError is returned by the primary when link-and-sync cannot
// complete as requested.
type TransferArchiveError string

const (
	// TransferArchiveRelinkRequested means the primary cancelled upload and
	// wants the secondary to relink.
	TransferArchiveRelinkRequested TransferArchiveError = "RELINK_REQUESTED"
	// TransferArchiveContinueWithoutUpload means the primary chose to finish
	// linking without uploading message history.
	TransferArchiveContinueWithoutUpload TransferArchiveError = "CONTINUE_WITHOUT_UPLOAD"
)

// TransferArchive describes a CDN-hosted encrypted backup archive for
// link-and-sync.
type TransferArchive struct {
	CDN    uint32
	CDNKey string
}

// TransferArchivePollResult is the outcome of polling GET
// /v1/devices/transfer_archive.
type TransferArchivePollResult struct {
	Archive *TransferArchive
	Error   TransferArchiveError
}

// DefaultTransferArchiveTimeout is the total time a secondary waits for the
// primary to upload a transfer archive.
const DefaultTransferArchiveTimeout = time.Hour

const transferArchivePollChunk = 5 * time.Minute

// FetchTransferArchive polls GET /v1/devices/transfer_archive until the
// primary uploads an archive, reports an error, or timeout elapses. The server
// responds with HTTP 204 while the archive is not ready yet.
func (c *Client) FetchTransferArchive(
	ctx context.Context,
	creds Credentials,
	timeout time.Duration,
) (*TransferArchivePollResult, error) {
	if creds.Username == "" || creds.Password == "" {
		return nil, errors.New("web.FetchTransferArchive: credentials required")
	}
	if timeout <= 0 {
		timeout = DefaultTransferArchiveTimeout
	}
	deadline := time.Now().Add(timeout)

	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("web.FetchTransferArchive: %w", err)
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errors.New("web.FetchTransferArchive: timed out waiting for archive")
		}
		chunk := remaining
		if chunk > transferArchivePollChunk {
			chunk = transferArchivePollChunk
		}
		result, retry, err := c.fetchTransferArchiveOnce(ctx, creds, chunk)
		if err != nil {
			return nil, err
		}
		if !retry {
			return result, nil
		}
	}
}

func (c *Client) fetchTransferArchiveOnce(
	ctx context.Context,
	creds Credentials,
	timeout time.Duration,
) (*TransferArchivePollResult, bool, error) {
	timeoutSecs := int(timeout.Round(time.Second) / time.Second)
	if timeoutSecs < 1 {
		timeoutSecs = 1
	}
	q := url.Values{"timeout": []string{strconv.Itoa(timeoutSecs)}}
	full, err := c.buildURL("/v1/devices/transfer_archive", q)
	if err != nil {
		return nil, false, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, false, fmt.Errorf("web.FetchTransferArchive: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("X-Signal-Agent", c.UserAgent)
	req.Header.Set("Authorization", creds.Header())

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("web.FetchTransferArchive: do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, false, fmt.Errorf("web.FetchTransferArchive: read body: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, true, nil
	case http.StatusOK:
		var raw struct {
			CDN   *uint32 `json:"cdn"`
			Key   *string `json:"key"`
			Error *string `json:"error"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, false, fmt.Errorf("web.FetchTransferArchive: decode: %w", err)
		}
		if raw.Error != nil {
			switch TransferArchiveError(*raw.Error) {
			case TransferArchiveRelinkRequested, TransferArchiveContinueWithoutUpload:
				return &TransferArchivePollResult{Error: TransferArchiveError(*raw.Error)}, false, nil
			default:
				return nil, false, fmt.Errorf("web.FetchTransferArchive: unknown error %q", *raw.Error)
			}
		}
		if raw.CDN == nil || raw.Key == nil || *raw.Key == "" {
			return nil, false, errors.New("web.FetchTransferArchive: incomplete archive response")
		}
		return &TransferArchivePollResult{
			Archive: &TransferArchive{CDN: *raw.CDN, CDNKey: *raw.Key},
		}, false, nil
	default:
		return nil, false, &Error{StatusCode: resp.StatusCode, Status: resp.Status, Body: body}
	}
}
