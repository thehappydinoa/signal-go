package backup

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

func TestDecryptArchiveStepPlainLen(t *testing.T) {
	const (
		testAccountEntropy = "mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"
		testACI            = "11111111-1111-1111-1111-111111111111"
	)
	ciphertext, err := os.ReadFile(filepath.Join("..", "libsignal", "testdata", "legacy-account.binproto.encrypted"))
	if err != nil {
		t.Fatal(err)
	}
	backupKey, _ := libsignal.DeriveBackupKey(testAccountEntropy)
	backupID, _ := libsignal.DeriveBackupID(backupKey, testACI)
	msgKey, _ := libsignal.NewMessageBackupKeyFromBackupKeyAndBackupID(backupKey, backupID, nil)
	defer msgKey.Close()
	aesKey, _ := msgKey.AesKey()
	_, _ = msgKey.HmacKey()

	extraPrefix, encStart, err := parseHeaderPrefix(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	payload := ciphertext[:len(ciphertext)-hmacLen]
	encBody := payload
	if encStart > 0 {
		encBody = payload[encStart:]
	}
	t.Logf("extra=%x encStart=%d encBodyLen=%d", extraPrefix, encStart, len(encBody))

	iv := encBody[:aesIVLen]
	cbcCiphertext := encBody[aesIVLen:]
	block, _ := aes.NewCipher(aesKey[:])
	mode := cipher.NewCBCDecrypter(block, iv)
	plainPadded := make([]byte, len(cbcCiphertext))
	mode.CryptBlocks(plainPadded, cbcCiphertext)
	plain, err := pkcs7Unpad(plainPadded, aesBlockSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("plain len=%d head=%x", len(plain), plain[:16])

	zr, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	zr.Multistream(false)
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}
	t.Logf("decompressed len=%d", len(out))
}
