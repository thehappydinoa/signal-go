package signal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

// SendAttachment delivers a 1:1 attachment to the recipient identified by
// their ACI. The plaintext is encrypted, uploaded to CDN3, and embedded in
// a DataMessage.
func (c *Client) SendAttachment(ctx context.Context, recipientACI string, r io.Reader, opts SendAttachmentOptions) (Receipt, error) {
	if recipientACI == "" {
		return Receipt{}, errors.New("signal.SendAttachment: empty recipient")
	}
	plain, err := readAttachmentPlaintext(r)
	if err != nil {
		return Receipt{}, err
	}
	meta, err := c.uploadEncryptedAttachment(ctx, plain, opts)
	if err != nil {
		return Receipt{}, err
	}
	ts := uint64(time.Now().UnixMilli())
	body := opts.Caption
	contentBytes, err := buildAttachmentDataMessageContent(body, ts, nil, 0, meta)
	if err != nil {
		return Receipt{}, err
	}
	return c.sendContent(ctx, recipientACI, contentBytes, ts, deliveryOpts{Urgent: true})
}

// SendGroupAttachment delivers an attachment to a Groups v2 chat.
func (c *Client) SendGroupAttachment(ctx context.Context, masterKey []byte, r io.Reader, opts SendAttachmentOptions) (Receipt, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return Receipt{}, fmt.Errorf("signal.SendGroupAttachment: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	plain, err := readAttachmentPlaintext(r)
	if err != nil {
		return Receipt{}, err
	}
	if c.webc == nil || c.stores == nil {
		return Receipt{}, errors.New("signal.SendGroupAttachment: Client was opened without send-side dependencies")
	}
	meta, err := c.uploadEncryptedAttachment(ctx, plain, opts)
	if err != nil {
		return Receipt{}, err
	}
	grp, err := c.FetchGroup(ctx, masterKey)
	if err != nil {
		return Receipt{}, fmt.Errorf("signal.SendGroupAttachment: fetch group: %w", err)
	}
	ts := uint64(time.Now().UnixMilli())
	contentBytes, err := buildAttachmentDataMessageContent(opts.Caption, ts, masterKey, grp.Revision, meta)
	if err != nil {
		return Receipt{}, err
	}
	return c.deliverGroupPayload(ctx, masterKey, grp, contentBytes, ts, groupDeliveryOpts{
		online:         false,
		urgent:         true,
		distributeSKDM: true,
	})
}

func buildAttachmentDataMessageContent(body string, tsMillis uint64, masterKey []byte, revision uint32, meta AttachmentMeta) ([]byte, error) {
	timestamp := tsMillis
	ptr := attachmentMetaToPointer(meta)
	dm := &sspb.DataMessage{
		Timestamp:   &timestamp,
		Attachments: []*sspb.AttachmentPointer{ptr},
	}
	if body != "" {
		dm.Body = &body
	}
	if len(masterKey) == libsignal.GroupMasterKeyLen {
		rev := revision
		dm.GroupV2 = &sspb.GroupContextV2{
			MasterKey: masterKey,
			Revision:  &rev,
		}
	}
	content := &sspb.Content{
		Content: &sspb.Content_DataMessage{DataMessage: dm},
	}
	return proto.Marshal(content)
}
