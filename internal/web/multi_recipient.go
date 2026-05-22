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

// SendMultiRecipientMessage issues PUT /v1/messages/multi_recipient with a
// libsignal SealedSenderMultiRecipientMessage payload. combinedUAK is the
// bitwise XOR of every recipient's 16-byte unidentified access key.
func (c *Client) SendMultiRecipientMessage(
	ctx context.Context,
	combinedUAK []byte,
	payload []byte,
	ts uint64,
	online, urgent bool,
) error {
	if len(combinedUAK) != 16 {
		return errors.New("web.SendMultiRecipientMessage: combined UAK must be 16 bytes")
	}
	if len(payload) == 0 {
		return errors.New("web.SendMultiRecipientMessage: empty payload")
	}
	q := url.Values{
		"ts":     []string{fmt.Sprintf("%d", ts)},
		"online": []string{fmt.Sprintf("%t", online)},
		"urgent": []string{fmt.Sprintf("%t", urgent)},
		"story":  []string{"false"},
	}
	err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   "/v1/messages/multi_recipient",
		Query:  q,
		Headers: http.Header{
			"Content-Type":            []string{MultiRecipientContentType},
			"Unidentified-Access-Key": []string{base64.StdEncoding.EncodeToString(combinedUAK)},
		},
		RawBody: payload,
	})
	if err != nil {
		return mapSendError(err)
	}
	return nil
}
