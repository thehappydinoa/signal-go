package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultGroupLogsLimit = 64

// GroupLogsOptions configures GET /v2/groups/logs/{fromVersion}.
type GroupLogsOptions struct {
	// CachedSendEndorsementsExpiration is the unix seconds expiration of the
	// caller's cached GSE, sent in the Cached-Send-Endorsements header.
	// Use 0 when no cache or when continuing a multi-page fetch after partial
	// content (per Signal storage-service semantics).
	CachedSendEndorsementsExpiration int64
	Limit                            int
	MaxSupportedChangeEpoch          uint32
	IncludeFirstState                bool
	IncludeLastState                 bool
}

// GroupLogsPage is one page of group change logs from the storage service.
type GroupLogsPage struct {
	Body         []byte
	StatusCode   int
	ContentRange string
}

// FetchGroupLogs issues GET /v2/groups/logs/{fromVersion}. Returns 200 or 206
// responses; callers inspect StatusCode and ContentRange for pagination.
func (c *Client) FetchGroupLogs(ctx context.Context, groupsAuthHeader string, fromVersion uint32, opts GroupLogsOptions) (*GroupLogsPage, error) {
	if groupsAuthHeader == "" {
		return nil, errors.New("web.FetchGroupLogs: empty authorization")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultGroupLogsLimit
	}
	q := url.Values{
		"limit": {strconv.Itoa(limit)},
	}
	if opts.MaxSupportedChangeEpoch > 0 {
		q.Set("maxSupportedChangeEpoch", strconv.FormatUint(uint64(opts.MaxSupportedChangeEpoch), 10))
	}
	if opts.IncludeFirstState {
		q.Set("includeFirstState", "true")
	}
	if opts.IncludeLastState {
		q.Set("includeLastState", "true")
	}

	path := fmt.Sprintf("/v2/groups/logs/%d", fromVersion)
	full, err := c.buildURL(path, q)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("web.FetchGroupLogs: build request: %w", err)
	}
	httpReq.Header.Set("User-Agent", c.UserAgent)
	httpReq.Header.Set("X-Signal-Agent", c.UserAgent)
	httpReq.Header.Set("Authorization", groupsAuthHeader)
	httpReq.Header.Set("Accept", "application/x-protobuf")
	httpReq.Header.Set("Cached-Send-Endorsements", strconv.FormatInt(opts.CachedSendEndorsementsExpiration, 10))

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("web.FetchGroupLogs: do: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("web.FetchGroupLogs: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, &Error{StatusCode: resp.StatusCode, Status: resp.Status, Body: body}
	}
	return &GroupLogsPage{
		Body:         body,
		StatusCode:   resp.StatusCode,
		ContentRange: resp.Header.Get("Content-Range"),
	}, nil
}

// NextGroupLogRevision parses a Content-Range header of the form
// "versions start-end/total" and returns end+1 for the next page request.
func NextGroupLogRevision(contentRange string) (uint32, error) {
	contentRange = strings.TrimSpace(contentRange)
	if contentRange == "" {
		return 0, errors.New("web.NextGroupLogRevision: empty content range")
	}
	const prefix = "versions "
	if !strings.HasPrefix(contentRange, prefix) {
		return 0, fmt.Errorf("web.NextGroupLogRevision: unexpected format %q", contentRange)
	}
	rest := strings.TrimPrefix(contentRange, prefix)
	dash := strings.IndexByte(rest, '-')
	slash := strings.IndexByte(rest, '/')
	if dash < 0 || slash < 0 || dash >= slash {
		return 0, fmt.Errorf("web.NextGroupLogRevision: malformed range %q", contentRange)
	}
	endStr := rest[dash+1 : slash]
	end, err := strconv.ParseUint(endStr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("web.NextGroupLogRevision: parse end: %w", err)
	}
	return uint32(end) + 1, nil
}
