package attachment

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
)

const (
	// CombinedKeySize is the length of AttachmentPointer.key material
	// (32-byte AES key + 32-byte HMAC key).
	CombinedKeySize = 64
	cipherKeySize   = 32
	macKeySize      = 32
	// IVSize is the AES-CBC initialization vector length.
	IVSize = aes.BlockSize
	// MacSize is the trailing HMAC-SHA256 tag length.
	MacSize = sha256.Size
)

var stickerPackInfo = []byte("Sticker Pack")

// NewKey returns a random 64-byte combined attachment key.
func NewKey() ([]byte, error) {
	key := make([]byte, CombinedKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("attachment.NewKey: %w", err)
	}
	return key, nil
}

// ExpandStickerPackKey derives 64-byte attachment key material from a
// 32-byte sticker pack key (HKDF-SHA256, info="Sticker Pack").
func ExpandStickerPackKey(packKey []byte) ([]byte, error) {
	if len(packKey) != 32 {
		return nil, errors.New("attachment.ExpandStickerPackKey: pack key must be 32 bytes")
	}
	out, err := libsignal.HKDFSHA256(CombinedKeySize, packKey, stickerPackInfo, nil)
	if err != nil {
		return nil, fmt.Errorf("attachment.ExpandStickerPackKey: %w", err)
	}
	return out, nil
}

// CiphertextLength returns the on-disk/CDN blob size for a plaintext of
// the given length (IV + PKCS-padded ciphertext + HMAC).
func CiphertextLength(plaintextLen int64) int64 {
	if plaintextLen < 0 {
		return 0
	}
	padded := ((plaintextLen / int64(aes.BlockSize)) + 1) * int64(aes.BlockSize)
	return int64(IVSize) + padded + int64(MacSize)
}

// EncryptResult holds the encrypted attachment blob and its transmitted
// digest (SHA-256 over IV || ciphertext || MAC).
type EncryptResult struct {
	Ciphertext []byte
	Digest     []byte
}

// Encrypt encrypts plaintext with the libsignal-service-java AttachmentCipher
// wire format: random IV, AES-256-CBC/PKCS7, trailing HMAC-SHA256 over
// IV||ciphertext.
func Encrypt(plaintext, combinedKey []byte) (*EncryptResult, error) {
	if len(combinedKey) != CombinedKeySize {
		return nil, fmt.Errorf("attachment.Encrypt: key must be %d bytes", CombinedKeySize)
	}
	cipherKey := combinedKey[:cipherKeySize]
	macKey := combinedKey[cipherKeySize:]

	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("attachment.Encrypt: iv: %w", err)
	}

	block, err := aes.NewCipher(cipherKey)
	if err != nil {
		return nil, fmt.Errorf("attachment.Encrypt: aes: %w", err)
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)

	mac := hmac.New(sha256.New, macKey)
	mac.Write(iv)
	mac.Write(encrypted)
	tag := mac.Sum(nil)

	out := make([]byte, 0, IVSize+len(encrypted)+MacSize)
	out = append(out, iv...)
	out = append(out, encrypted...)
	out = append(out, tag...)

	digest := transmittedDigest(out)
	return &EncryptResult{Ciphertext: out, Digest: digest}, nil
}

// Decrypt verifies the MAC and optional transmitted digest, then decrypts
// an AttachmentCipher blob. When plaintextLen is positive the output is
// truncated to that length (Signal stores plaintext size separately on
// AttachmentPointer.size). digest may be nil for sticker payloads.
func Decrypt(ciphertext, combinedKey, digest []byte, plaintextLen int64) ([]byte, error) {
	if len(combinedKey) != CombinedKeySize {
		return nil, fmt.Errorf("attachment.Decrypt: key must be %d bytes", CombinedKeySize)
	}
	if len(ciphertext) <= IVSize+MacSize {
		return nil, errors.New("attachment.Decrypt: ciphertext too short")
	}
	if digest != nil && len(digest) != sha256.Size {
		return nil, errors.New("attachment.Decrypt: digest must be 32 bytes")
	}

	macKey := combinedKey[cipherKeySize:]
	bodyEnd := len(ciphertext) - MacSize
	theirMAC := ciphertext[bodyEnd:]

	mac := hmac.New(sha256.New, macKey)
	mac.Write(ciphertext[:bodyEnd])
	ourMAC := mac.Sum(nil)
	if subtle.ConstantTimeCompare(ourMAC, theirMAC) != 1 {
		return nil, errors.New("attachment.Decrypt: MAC mismatch")
	}

	gotDigest := transmittedDigest(ciphertext)
	if digest != nil && subtle.ConstantTimeCompare(gotDigest, digest) != 1 {
		return nil, errors.New("attachment.Decrypt: digest mismatch")
	}

	cipherKey := combinedKey[:cipherKeySize]
	iv := ciphertext[:IVSize]
	enc := ciphertext[IVSize:bodyEnd]
	if len(enc)%aes.BlockSize != 0 {
		return nil, errors.New("attachment.Decrypt: invalid ciphertext length")
	}

	block, err := aes.NewCipher(cipherKey)
	if err != nil {
		return nil, fmt.Errorf("attachment.Decrypt: aes: %w", err)
	}
	plain := make([]byte, len(enc))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, enc)
	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("attachment.Decrypt: %w", err)
	}

	if plaintextLen > 0 {
		if int64(len(plain)) < plaintextLen {
			return nil, errors.New("attachment.Decrypt: plaintext shorter than expected size")
		}
		plain = plain[:plaintextLen]
	}
	return plain, nil
}

// DecryptSticker decrypts sticker data encrypted with a 32-byte pack key.
func DecryptSticker(ciphertext, packKey []byte) ([]byte, error) {
	combined, err := ExpandStickerPackKey(packKey)
	if err != nil {
		return nil, err
	}
	return Decrypt(ciphertext, combined, nil, 0)
}

func transmittedDigest(blob []byte) []byte {
	if len(blob) < MacSize {
		return nil
	}
	h := sha256.New()
	h.Write(blob[:len(blob)-MacSize])
	h.Write(blob[len(blob)-MacSize:])
	return h.Sum(nil)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	if padding == 0 {
		padding = blockSize
	}
	out := make([]byte, len(data)+padding)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(padding)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid padding")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, errors.New("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if int(data[i]) != padding {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}
