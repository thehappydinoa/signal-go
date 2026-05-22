package attachment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

// SupportsIncrementalMac reports whether Signal clients emit incremental MAC
// metadata for the given MIME type (currently video/mp4 only).
func SupportsIncrementalMac(contentType string) bool {
	return contentType == "video/mp4"
}

// V2EncryptResult holds encrypted attachment v2 output metadata.
type V2EncryptResult struct {
	Ciphertext     []byte
	Digest         []byte
	PlaintextHash  string
	IncrementalMAC []byte
	ChunkSize      uint32
}

// EncryptV2 encrypts plaintext using Signal Desktop's attachment v2 wire
// format: log padding, AES-256-CBC, trailing HMAC, SHA-256 digest, and
// optional incremental MAC for streaming media types.
func EncryptV2(plaintext, combinedKey []byte, contentType string) (*V2EncryptResult, error) {
	if len(combinedKey) != CombinedKeySize {
		return nil, fmt.Errorf("attachment.EncryptV2: key must be %d bytes", CombinedKeySize)
	}
	aesKey, macKey := splitKeys(combinedKey)

	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("attachment.EncryptV2: iv: %w", err)
	}

	padded := logPad(plaintext)
	pkcs := pkcs7Pad(padded, aes.BlockSize)
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("attachment.EncryptV2: aes: %w", err)
	}
	encrypted := make([]byte, len(pkcs))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, pkcs)

	body := append(append([]byte(nil), iv...), encrypted...)
	mac := hmac.New(sha256.New, macKey)
	mac.Write(body)
	tag := mac.Sum(nil)
	ciphertext := append(append([]byte(nil), body...), tag...)

	digest := sha256.Sum256(ciphertext)
	plainHash := sha256.Sum256(plaintext)

	result := &V2EncryptResult{
		Ciphertext:    ciphertext,
		Digest:        digest[:],
		PlaintextHash: hex.EncodeToString(plainHash[:]),
	}

	if SupportsIncrementalMac(contentType) {
		chunkSize, err := libsignal.CalculateIncrementalMacChunkSize(uint32(len(ciphertext)))
		if err != nil {
			return nil, fmt.Errorf("attachment.EncryptV2: chunk size: %w", err)
		}
		incr, err := libsignal.DigestIncrementalMac(macKey, chunkSize, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("attachment.EncryptV2: incremental mac: %w", err)
		}
		result.IncrementalMAC = incr
		result.ChunkSize = chunkSize
	}
	return result, nil
}

// DecryptV2 decrypts Signal Desktop attachment v2 blobs. When incrementalMAC
// and chunkSize are non-zero, incremental MAC verification runs before the
// trailing HMAC check.
func DecryptV2(ciphertext, combinedKey, digest, incrementalMAC []byte, chunkSize uint32, plaintextLen int) ([]byte, error) {
	if len(combinedKey) != CombinedKeySize {
		return nil, fmt.Errorf("attachment.DecryptV2: key must be %d bytes", CombinedKeySize)
	}
	if len(ciphertext) <= IVSize+MacSize {
		return nil, errors.New("attachment.DecryptV2: ciphertext too short")
	}
	if digest != nil && len(digest) != sha256.Size {
		return nil, errors.New("attachment.DecryptV2: digest must be 32 bytes")
	}

	aesKey, macKey := splitKeys(combinedKey)

	if digest != nil {
		got := sha256.Sum256(ciphertext)
		if subtle.ConstantTimeCompare(got[:], digest) != 1 {
			return nil, errors.New("attachment.DecryptV2: digest mismatch")
		}
	}

	if len(incrementalMAC) > 0 {
		if chunkSize == 0 {
			return nil, errors.New("attachment.DecryptV2: chunk size required with incremental MAC")
		}
		v, err := libsignal.NewValidatingMac(macKey, chunkSize, incrementalMAC)
		if err != nil {
			return nil, fmt.Errorf("attachment.DecryptV2: validating mac: %w", err)
		}
		if _, err := v.Update(ciphertext); err != nil {
			return nil, fmt.Errorf("attachment.DecryptV2: %w", err)
		}
		if _, err := v.Finalize(); err != nil {
			return nil, fmt.Errorf("attachment.DecryptV2: %w", err)
		}
	}

	bodyEnd := len(ciphertext) - MacSize
	theirMAC := ciphertext[bodyEnd:]
	mac := hmac.New(sha256.New, macKey)
	mac.Write(ciphertext[:bodyEnd])
	if subtle.ConstantTimeCompare(mac.Sum(nil), theirMAC) != 1 {
		return nil, errors.New("attachment.DecryptV2: MAC mismatch")
	}

	iv := ciphertext[:IVSize]
	enc := ciphertext[IVSize:bodyEnd]
	if len(enc)%aes.BlockSize != 0 {
		return nil, errors.New("attachment.DecryptV2: invalid ciphertext length")
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("attachment.DecryptV2: aes: %w", err)
	}
	padded := make([]byte, len(enc))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(padded, enc)
	unpadded, err := pkcs7Unpad(padded, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("attachment.DecryptV2: %w", err)
	}
	return trimLogPad(unpadded, plaintextLen), nil
}

// DecryptAuto picks v2 (log padding) or legacy PKCS7 decryption based on
// AttachmentPointer metadata.
func DecryptAuto(ciphertext, combinedKey, digest, incrementalMAC []byte, chunkSize uint32, plaintextLen int) ([]byte, error) {
	if len(incrementalMAC) > 0 || chunkSize > 0 {
		return DecryptV2(ciphertext, combinedKey, digest, incrementalMAC, chunkSize, plaintextLen)
	}
	if out, err := DecryptV2(ciphertext, combinedKey, digest, nil, 0, plaintextLen); err == nil {
		return out, nil
	}
	return Decrypt(ciphertext, combinedKey, digest, int64(plaintextLen))
}

// CiphertextLengthV2 returns encrypted blob size for v2 attachments.
func CiphertextLengthV2(plaintextLen int) int {
	padded := LogPadSize(plaintextLen)
	encLen := ((padded / aes.BlockSize) + 1) * aes.BlockSize
	if padded%aes.BlockSize == 0 {
		encLen = padded + aes.BlockSize
	}
	return IVSize + encLen + MacSize
}

func splitKeys(combined []byte) (aesKey, macKey []byte) {
	return combined[:cipherKeySize], combined[cipherKeySize:]
}
