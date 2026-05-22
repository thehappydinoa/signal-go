package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/thehappydinoa/signal-go/internal/attachment"
	sspb "github.com/thehappydinoa/signal-go/internal/proto/gen/signalservicepb"
)

// AttachmentMeta describes one attachment on an inbound or outbound message.
type AttachmentMeta struct {
	ContentType     string
	FileName        string
	Size            uint32
	CDNKey          string
	CDNNumber       uint32
	Key             []byte
	Digest          []byte
	IncrementalMAC  []byte
	ChunkSize       uint32
	UploadTimestamp uint64
}

// SendAttachmentOptions configures [Client.SendAttachment] and
// [Client.SendGroupAttachment].
type SendAttachmentOptions struct {
	ContentType string
	FileName    string
	Caption     string
}

// DownloadAttachment fetches and decrypts an attachment described by meta.
func (c *Client) DownloadAttachment(ctx context.Context, meta AttachmentMeta) ([]byte, error) {
	if c.webc == nil {
		return nil, errors.New("signal.DownloadAttachment: Client was opened without web client")
	}
	if meta.CDNKey == "" {
		return nil, errors.New("signal.DownloadAttachment: missing cdn key")
	}
	if len(meta.Key) != attachment.CombinedKeySize {
		return nil, errors.New("signal.DownloadAttachment: key must be 64 bytes")
	}
	cdnNumber := meta.CDNNumber
	if cdnNumber == 0 {
		cdnNumber = 3
	}
	ct, err := c.webc.DownloadAttachmentCDN(ctx, cdnNumber, meta.CDNKey)
	if err != nil {
		return nil, fmt.Errorf("signal.DownloadAttachment: %w", err)
	}
	plain, err := attachment.DecryptAuto(ct, meta.Key, meta.Digest, meta.IncrementalMAC, meta.ChunkSize, int(meta.Size))
	if err != nil {
		return nil, fmt.Errorf("signal.DownloadAttachment: %w", err)
	}
	return plain, nil
}

func (c *Client) uploadEncryptedAttachment(ctx context.Context, plaintext []byte, opts SendAttachmentOptions) (AttachmentMeta, error) {
	if c.webc == nil {
		return AttachmentMeta{}, errors.New("signal: Client was opened without web client")
	}
	contentType := opts.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	key, err := attachment.NewKey()
	if err != nil {
		return AttachmentMeta{}, err
	}
	enc, err := attachment.EncryptV2(plaintext, key, contentType)
	if err != nil {
		return AttachmentMeta{}, fmt.Errorf("signal: encrypt attachment: %w", err)
	}
	form, err := c.webc.FetchAttachmentUploadForm(ctx, c.credentials(), uint64(len(enc.Ciphertext)))
	if err != nil {
		return AttachmentMeta{}, fmt.Errorf("signal: fetch upload form: %w", err)
	}
	if err := c.webc.UploadAttachment(ctx, form, enc.Ciphertext); err != nil {
		return AttachmentMeta{}, fmt.Errorf("signal: upload attachment: %w", err)
	}
	cdnNumber := form.CDN
	if cdnNumber == 0 {
		cdnNumber = 3
	}
	return AttachmentMeta{
		ContentType:     contentType,
		FileName:        opts.FileName,
		Size:            uint32(len(plaintext)),
		CDNKey:          form.Key,
		CDNNumber:       cdnNumber,
		Key:             key,
		Digest:          enc.Digest,
		IncrementalMAC:  enc.IncrementalMAC,
		ChunkSize:       enc.ChunkSize,
		UploadTimestamp: uint64(time.Now().UnixMilli()),
	}, nil
}

func readAttachmentPlaintext(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, errors.New("signal: nil attachment reader")
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("signal: read attachment: %w", err)
	}
	if len(plain) == 0 {
		return nil, errors.New("signal: empty attachment")
	}
	return plain, nil
}

func attachmentMetaToPointer(meta AttachmentMeta) *sspb.AttachmentPointer {
	ptr := &sspb.AttachmentPointer{
		AttachmentIdentifier: &sspb.AttachmentPointer_CdnKey{CdnKey: meta.CDNKey},
		ContentType:          strPtr(meta.ContentType),
		Key:                  append([]byte(nil), meta.Key...),
		Size:                 &meta.Size,
		Digest:               append([]byte(nil), meta.Digest...),
		CdnNumber:            &meta.CDNNumber,
		UploadTimestamp:      &meta.UploadTimestamp,
	}
	if meta.FileName != "" {
		ptr.FileName = strPtr(meta.FileName)
	}
	if len(meta.IncrementalMAC) > 0 {
		ptr.IncrementalMac = append([]byte(nil), meta.IncrementalMAC...)
		ptr.ChunkSize = &meta.ChunkSize
	}
	return ptr
}

func attachmentFromPointer(p *sspb.AttachmentPointer) AttachmentMeta {
	if p == nil {
		return AttachmentMeta{}
	}
	meta := AttachmentMeta{
		ContentType:     p.GetContentType(),
		FileName:        p.GetFileName(),
		Size:            p.GetSize(),
		Key:             append([]byte(nil), p.GetKey()...),
		Digest:          append([]byte(nil), p.GetDigest()...),
		IncrementalMAC:  append([]byte(nil), p.GetIncrementalMac()...),
		ChunkSize:       p.GetChunkSize(),
		UploadTimestamp: p.GetUploadTimestamp(),
	}
	if k := p.GetCdnKey(); k != "" {
		meta.CDNKey = k
	} else if id := p.GetCdnId(); id != 0 {
		meta.CDNKey = hex.EncodeToString([]byte{
			byte(id >> 56), byte(id >> 48), byte(id >> 40), byte(id >> 32),
			byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id),
		})
	}
	meta.CDNNumber = p.GetCdnNumber()
	return meta
}

func strPtr(s string) *string { return &s }
