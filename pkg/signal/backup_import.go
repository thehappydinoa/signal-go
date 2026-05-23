package signal

import (
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/backup"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/store"
)

// ImportTransferArchiveOptions configures [ImportTransferArchive].
type ImportTransferArchiveOptions struct {
	// ArchiveBytes is validated encrypted backup ciphertext.
	ArchiveBytes []byte
	// EphemeralBackupKey is the 32-byte one-time key from ProvisionMessage.
	EphemeralBackupKey []byte
	// ACI is the linked account's ACI string.
	ACI string
	// Purpose selects the libsignal validation purpose. Zero uses
	// [libsignal.BackupPurposeDeviceTransfer].
	Purpose libsignal.BackupPurpose
	// Identities receives imported contact identity keys.
	Identities store.IdentityStore
	// BackupImport receives imported contact/group list entries.
	BackupImport store.BackupImportStore
	// OnChatItem receives each ChatItem frame as protobuf bytes (optional).
	OnChatItem func(serializedChatItem []byte) error
}

// ImportTransferArchive decrypts and imports transfer-archive frames into the
// local store after validation.
func ImportTransferArchive(opts ImportTransferArchiveOptions) (*backup.ImportStats, error) {
	if len(opts.ArchiveBytes) == 0 {
		return nil, errors.New("signal.ImportTransferArchive: empty archive")
	}
	if len(opts.EphemeralBackupKey) != libsignal.BackupKeyLen {
		return nil, fmt.Errorf("signal.ImportTransferArchive: ephemeral backup key must be %d bytes", libsignal.BackupKeyLen)
	}
	if opts.ACI == "" {
		return nil, errors.New("signal.ImportTransferArchive: ACI required")
	}
	if opts.Identities == nil && opts.BackupImport == nil && opts.OnChatItem == nil {
		return nil, errors.New("signal.ImportTransferArchive: at least one import target required")
	}

	var backupKey [libsignal.BackupKeyLen]byte
	copy(backupKey[:], opts.EphemeralBackupKey)
	msgKey, err := libsignal.NewMessageBackupKeyForEphemeralTransfer(backupKey, opts.ACI)
	if err != nil {
		return nil, fmt.Errorf("signal.ImportTransferArchive: derive key: %w", err)
	}
	defer msgKey.Close()

	purpose := opts.Purpose
	if purpose == 0 {
		purpose = libsignal.BackupPurposeDeviceTransfer
	}
	stats, err := backup.ImportArchive(msgKey, opts.ArchiveBytes, purpose, backup.ImportTarget{
		Identities:   opts.Identities,
		BackupImport: opts.BackupImport,
		OnChatItem:   opts.OnChatItem,
	})
	if err != nil {
		return nil, fmt.Errorf("signal.ImportTransferArchive: %w", err)
	}
	return stats, nil
}
