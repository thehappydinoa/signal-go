package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

const (
	storageServiceKeyInfo = "Storage Service Encryption"
	itemKeyInfoPrefix     = "20240801_SIGNAL_STORAGE_SERVICE_ITEM_"
	itemKeyLen            = 32
	ivLen                 = 12
)

// Keys holds derived storage-service encryption keys.
type Keys struct {
	MasterKey         [libsignal.SVRKeyLen]byte
	StorageServiceKey [libsignal.SVRKeyLen]byte
}

// DeriveKeys derives the storage-service key chain from an
// AccountEntropyPool string.
func DeriveKeys(accountEntropyPool string) (Keys, error) {
	var keys Keys
	master, err := libsignal.DeriveSVRKey(accountEntropyPool)
	if err != nil {
		return keys, fmt.Errorf("storage.DeriveKeys: %w", err)
	}
	keys.MasterKey = master
	keys.StorageServiceKey = DeriveStorageServiceKey(master)
	return keys, nil
}

// DeriveStorageServiceKey derives the storage-service encryption key from
// the SVR master key (HMAC-SHA256 with "Storage Service Encryption").
func DeriveStorageServiceKey(masterKey [libsignal.SVRKeyLen]byte) [libsignal.SVRKeyLen]byte {
	mac := hmac.New(sha256.New, masterKey[:])
	_, _ = mac.Write([]byte(storageServiceKeyInfo))
	var out [libsignal.SVRKeyLen]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// DeriveManifestKey derives the AES key for a manifest at version.
func DeriveManifestKey(storageServiceKey [libsignal.SVRKeyLen]byte, version uint64) [libsignal.SVRKeyLen]byte {
	info := fmt.Sprintf("Manifest_%d", version)
	mac := hmac.New(sha256.New, storageServiceKey[:])
	_, _ = mac.Write([]byte(info))
	var out [libsignal.SVRKeyLen]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// DeriveItemKey derives the AES key for one storage record. When recordIkm
// is non-nil the rotated HKDF path is used; otherwise the legacy HMAC path
// keyed by base64(storageID) applies.
func DeriveItemKey(storageServiceKey [libsignal.SVRKeyLen]byte, recordIkm, storageID []byte) ([libsignal.SVRKeyLen]byte, error) {
	var out [libsignal.SVRKeyLen]byte
	if len(recordIkm) > 0 {
		info := append([]byte(itemKeyInfoPrefix), storageID...)
		derived, err := libsignal.HKDFSHA256(itemKeyLen, recordIkm, info, nil)
		if err != nil {
			return out, fmt.Errorf("storage.DeriveItemKey: %w", err)
		}
		copy(out[:], derived)
		return out, nil
	}
	itemID := base64.StdEncoding.EncodeToString(storageID)
	info := "Item_" + itemID
	mac := hmac.New(sha256.New, storageServiceKey[:])
	_, _ = mac.Write([]byte(info))
	copy(out[:], mac.Sum(nil))
	return out, nil
}

// DecryptRecord decrypts a storage manifest or item blob (12-byte IV ||
// AES-256-GCM ciphertext). The wire format matches Signal Desktop's
// encryptProfile/decryptProfile helpers.
func DecryptRecord(key [libsignal.SVRKeyLen]byte, data []byte) ([]byte, error) {
	if len(data) < ivLen+16+1 {
		return nil, fmt.Errorf("storage.DecryptRecord: ciphertext too short (%d bytes)", len(data))
	}
	iv := data[:ivLen]
	ct := data[ivLen:]
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("storage.DecryptRecord: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("storage.DecryptRecord: gcm: %w", err)
	}
	plain, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("storage.DecryptRecord: %w", err)
	}
	return plain, nil
}

// EncryptRecordForTest encrypts plaintext with AES-256-GCM for unit tests.
func EncryptRecordForTest(key [libsignal.SVRKeyLen]byte, plaintext, iv []byte) ([]byte, error) {
	if len(iv) != ivLen {
		return nil, fmt.Errorf("storage.EncryptRecordForTest: iv length %d, want %d", len(iv), ivLen)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("storage.EncryptRecordForTest: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("storage.EncryptRecordForTest: gcm: %w", err)
	}
	ct := gcm.Seal(nil, iv, plaintext, nil)
	out := make([]byte, ivLen+len(ct))
	copy(out, iv)
	copy(out[ivLen:], ct)
	return out, nil
}
