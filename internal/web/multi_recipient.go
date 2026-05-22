package web

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// MultiRecipientContentType is the Content-Type for PUT
// /v1/messages/multi_recipient.
const MultiRecipientContentType = "application/vnd.signal-messenger.mrm"

// MultiRecipientAuth carries exactly one authorization mechanism for
// PUT /v1/messages/multi_recipient. Group-Send-Token is preferred for
// Groups v2; combinedUAK is the legacy XOR path.
type MultiRecipientAuth struct {
	GroupSendToken []byte // serialized GroupSendFullToken
	CombinedUAK    []byte // 16-byte XOR of member UAKs (legacy)
}

// SendMultiRecipientMessage issues PUT /v1/messages/multi_recipient.
func (c *Client) SendMultiRecipientMessage(
	ctx context.Context,
	auth MultiRecipientAuth,
	payload []byte,
	ts uint64,
	online, urgent bool,
) error {
	if len(payload) == 0 {
		return errors.New("web.SendMultiRecipientMessage: empty payload")
	}
	hasToken := len(auth.GroupSendToken) > 0
	hasUAK := len(auth.CombinedUAK) == 16
	switch {
	case hasToken && hasUAK:
		return errors.New("web.SendMultiRecipientMessage: Group-Send-Token and UAK are mutually exclusive")
	case !hasToken && !hasUAK:
		return errors.New("web.SendMultiRecipientMessage: Group-Send-Token or combined UAK required")
	}
	if hasUAK && len(auth.CombinedUAK) != 16 {
		return errors.New("web.SendMultiRecipientMessage: combined UAK must be 16 bytes")
	}

	q := url.Values{
		"ts":     []string{fmt.Sprintf("%d", ts)},
		"online": []string{fmt.Sprintf("%t", online)},
		"urgent": []string{fmt.Sprintf("%t", urgent)},
		"story":  []string{"false"},
	}
	headers := http.Header{
		"Content-Type": []string{MultiRecipientContentType},
	}
	if hasToken {
		headers.Set("Group-Send-Token", base64.StdEncoding.EncodeToString(auth.GroupSendToken))
	} else {
		headers.Set("Unidentified-Access-Key", base64.StdEncoding.EncodeToString(auth.CombinedUAK))
	}

	err := c.Do(ctx, Request{
		Method:  http.MethodPut,
		Path:    "/v1/messages/multi_recipient",
		Query:   q,
		Headers: headers,
		RawBody: payload,
	})
	if err != nil {
		return mapSendError(err)
	}
	return nil
}
