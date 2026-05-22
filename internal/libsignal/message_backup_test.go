package libsignal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMessageBackupLegacyFixture(t *testing.T) {
	const (
		testAccountEntropy = "mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"
		testACI            = "11111111-1111-1111-1111-111111111111"
	)

	ciphertext, err := os.ReadFile(filepath.Join("testdata", "legacy-account.binproto.encrypted"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	backupKey, err := DeriveBackupKey(testAccountEntropy)
	if err != nil {
		t.Fatalf("DeriveBackupKey: %v", err)
	}
	backupID, err := DeriveBackupID(backupKey, testACI)
	if err != nil {
		t.Fatalf("DeriveBackupID: %v", err)
	}
	msgKey, err := NewMessageBackupKeyFromBackupKeyAndBackupID(backupKey, backupID, nil)
	if err != nil {
		t.Fatalf("NewMessageBackupKeyFromBackupKeyAndBackupID: %v", err)
	}
	defer msgKey.Close()

	outcome, err := ValidateMessageBackup(msgKey, BackupPurposeRemoteBackup, ciphertext)
	if err != nil {
		t.Fatalf("ValidateMessageBackup: %v", err)
	}
	defer outcome.Close()
	if !outcome.OK() {
		msg, _ := outcome.ErrorMessage()
		t.Fatalf("validation failed: %s", msg)
	}
}

func TestNewMessageBackupKeyForEphemeralTransfer(t *testing.T) {
	var ephemeral [BackupKeyLen]byte
	for i := range ephemeral {
		ephemeral[i] = byte(i + 1)
	}
	key, err := NewMessageBackupKeyForEphemeralTransfer(ephemeral, "00000000-0000-0000-0000-000000000011")
	if err != nil {
		t.Fatalf("NewMessageBackupKeyForEphemeralTransfer: %v", err)
	}
	defer key.Close()
}
