package signal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// CapabilityBackup3 advertises link-and-sync support during provisioning.
const CapabilityBackup3 = "backup3"

// SyncTransferArchiveResult summarizes a post-link transfer archive receive.
type SyncTransferArchiveResult struct {
	// Validated is true when ciphertext was downloaded and passed libsignal
	// backup validation. Importing frames into the local store is not yet
	// implemented; v1 stops at validation.
	Validated bool
	// ArchiveBytes holds the validated ciphertext when Validated is true.
	ArchiveBytes []byte
	// Skipped is true when the primary chose CONTINUE_WITHOUT_UPLOAD.
	Skipped bool
	// RelinkRequested is true when the primary signalled RELINK_REQUESTED.
	RelinkRequested bool
}

// SyncTransferArchiveOptions configures [SyncTransferArchive].
type SyncTransferArchiveOptions struct {
	// EphemeralBackupKey is the 32-byte one-time key from ProvisionMessage.
	EphemeralBackupKey []byte
	// ACI is the linked account's ACI string.
	ACI string
	// Timeout bounds polling for GET /v1/devices/transfer_archive. Zero uses
	// [web.DefaultTransferArchiveTimeout].
	Timeout time.Duration
}

// SyncTransferArchive polls for a transfer archive from the primary device,
// downloads it from the attachments CDN, and validates the encrypted backup
// bundle via libsignal. v1 does not import backup frames into the store.
func SyncTransferArchive(
	ctx context.Context,
	webc *web.Client,
	creds web.Credentials,
	opts SyncTransferArchiveOptions,
) (*SyncTransferArchiveResult, error) {
	if webc == nil {
		return nil, errors.New("signal.SyncTransferArchive: web client required")
	}
	if len(opts.EphemeralBackupKey) != libsignal.BackupKeyLen {
		return nil, fmt.Errorf("signal.SyncTransferArchive: ephemeral backup key must be %d bytes", libsignal.BackupKeyLen)
	}
	if opts.ACI == "" {
		return nil, errors.New("signal.SyncTransferArchive: ACI required")
	}

	poll, err := webc.FetchTransferArchive(ctx, creds, opts.Timeout)
	if err != nil {
		return nil, fmt.Errorf("signal.SyncTransferArchive: poll: %w", err)
	}
	if poll.Error != "" {
		switch poll.Error {
		case web.TransferArchiveRelinkRequested:
			return &SyncTransferArchiveResult{RelinkRequested: true}, nil
		case web.TransferArchiveContinueWithoutUpload:
			return &SyncTransferArchiveResult{Skipped: true}, nil
		default:
			return nil, fmt.Errorf("signal.SyncTransferArchive: unexpected error %q", poll.Error)
		}
	}
	if poll.Archive == nil {
		return nil, errors.New("signal.SyncTransferArchive: empty archive response")
	}

	ciphertext, err := webc.DownloadAttachmentCDN(ctx, poll.Archive.CDN, poll.Archive.CDNKey)
	if err != nil {
		return nil, fmt.Errorf("signal.SyncTransferArchive: download: %w", err)
	}

	var backupKey [libsignal.BackupKeyLen]byte
	copy(backupKey[:], opts.EphemeralBackupKey)
	msgKey, err := libsignal.NewMessageBackupKeyForEphemeralTransfer(backupKey, opts.ACI)
	if err != nil {
		return nil, fmt.Errorf("signal.SyncTransferArchive: derive key: %w", err)
	}
	defer msgKey.Close()

	outcome, err := libsignal.ValidateMessageBackup(msgKey, libsignal.BackupPurposeDeviceTransfer, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("signal.SyncTransferArchive: validate: %w", err)
	}
	defer outcome.Close()
	if !outcome.OK() {
		msg, _ := outcome.ErrorMessage()
		if msg == "" {
			msg = "validation failed"
		}
		return nil, fmt.Errorf("signal.SyncTransferArchive: %s", msg)
	}

	return &SyncTransferArchiveResult{
		Validated:    true,
		ArchiveBytes: ciphertext,
	}, nil
}
