package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
)

func TestImportArchiveLegacyFixture(t *testing.T) {
	const (
		testAccountEntropy = "mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"
		testACI            = "11111111-1111-1111-1111-111111111111"
	)

	ciphertext, err := os.ReadFile(filepath.Join("..", "libsignal", "testdata", "legacy-account.binproto.encrypted"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	backupKey, err := libsignal.DeriveBackupKey(testAccountEntropy)
	if err != nil {
		t.Fatalf("DeriveBackupKey: %v", err)
	}
	backupID, err := libsignal.DeriveBackupID(backupKey, testACI)
	if err != nil {
		t.Fatalf("DeriveBackupID: %v", err)
	}
	msgKey, err := libsignal.NewMessageBackupKeyFromBackupKeyAndBackupID(backupKey, backupID, nil)
	if err != nil {
		t.Fatalf("NewMessageBackupKeyFromBackupKeyAndBackupID: %v", err)
	}
	defer msgKey.Close()

	importStore := memstore.NewBackupImportStore()
	signalStores := memstore.NewSignalStores()
	stats, err := ImportArchive(msgKey, ciphertext, libsignal.BackupPurposeRemoteBackup, ImportTarget{
		Identities:   signalStores,
		BackupImport: importStore,
	})
	if err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	if stats.FramesProcessed == 0 {
		t.Fatalf("expected frames processed, stats=%+v", stats)
	}
}

func TestDecryptArchiveLegacyFixture(t *testing.T) {
	const (
		testAccountEntropy = "mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"
		testACI            = "11111111-1111-1111-1111-111111111111"
	)

	ciphertext, err := os.ReadFile(filepath.Join("..", "libsignal", "testdata", "legacy-account.binproto.encrypted"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	backupKey, err := libsignal.DeriveBackupKey(testAccountEntropy)
	if err != nil {
		t.Fatalf("DeriveBackupKey: %v", err)
	}
	backupID, err := libsignal.DeriveBackupID(backupKey, testACI)
	if err != nil {
		t.Fatalf("DeriveBackupID: %v", err)
	}
	msgKey, err := libsignal.NewMessageBackupKeyFromBackupKeyAndBackupID(backupKey, backupID, nil)
	if err != nil {
		t.Fatalf("NewMessageBackupKeyFromBackupKeyAndBackupID: %v", err)
	}
	defer msgKey.Close()

	plain, err := decryptFixture(msgKey, ciphertext)
	if err != nil {
		t.Fatalf("DecryptArchive: %v", err)
	}
	first, err := ReadVarintFrame(bytes.NewReader(plain))
	if err != nil {
		t.Fatalf("backup info frame: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("empty backup info")
	}
}

func decryptFixture(key *libsignal.MessageBackupKey, ciphertext []byte) ([]byte, error) {
	aesKey, err := key.AesKey()
	if err != nil {
		return nil, err
	}
	hmacKey, err := key.HmacKey()
	if err != nil {
		return nil, err
	}
	return DecryptArchive(ciphertext, aesKey, hmacKey)
}
