package backup

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
)

const (
	hmacLen      = 32
	aesIVLen     = 16
	magicNumber  = "SBACKUP\x01"
	aesBlockSize = 16
)

// DecryptArchive decrypts and decompresses an encrypted message backup blob.
// The format matches libsignal's FramesReader (legacy and forward-secrecy
// headers).
func DecryptArchive(ciphertext []byte, aesKey, hmacKey [32]byte) ([]byte, error) {
	if len(ciphertext) < hmacLen+aesIVLen+aesBlockSize {
		return nil, errors.New("backup.DecryptArchive: ciphertext too short")
	}

	extraPrefix, encStart, err := parseHeaderPrefix(ciphertext)
	if err != nil {
		return nil, err
	}

	payload := ciphertext[:len(ciphertext)-hmacLen]
	storedHMAC := ciphertext[len(ciphertext)-hmacLen:]

	mac := hmac.New(sha256.New, hmacKey[:])
	if len(extraPrefix) > 0 && encStart == 0 {
		mac.Write(extraPrefix)
		if len(extraPrefix) < len(payload) {
			mac.Write(payload[len(extraPrefix):])
		}
	} else {
		if len(extraPrefix) > 0 {
			mac.Write(extraPrefix)
		}
		if encStart < len(payload) {
			mac.Write(payload[encStart:])
		}
	}
	if subtle.ConstantTimeCompare(storedHMAC, mac.Sum(nil)) != 1 {
		return nil, errors.New("backup.DecryptArchive: outer HMAC mismatch")
	}

	encBody := payload
	if encStart > 0 {
		encBody = payload[encStart:]
	}
	if len(encBody) < aesIVLen {
		return nil, errors.New("backup.DecryptArchive: missing IV")
	}

	innerMAC := hmac.New(sha256.New, hmacKey[:])
	innerMAC.Write(encBody)
	if subtle.ConstantTimeCompare(storedHMAC, innerMAC.Sum(nil)) != 1 {
		return nil, errors.New("backup.DecryptArchive: inner HMAC mismatch")
	}

	iv := encBody[:aesIVLen]
	cbcCiphertext := encBody[aesIVLen:]
	if len(cbcCiphertext)%aesBlockSize != 0 {
		return nil, errors.New("backup.DecryptArchive: ciphertext not block-aligned")
	}

	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		return nil, fmt.Errorf("backup.DecryptArchive: aes: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	plainPadded := make([]byte, len(cbcCiphertext))
	mode.CryptBlocks(plainPadded, cbcCiphertext)
	plain, err := pkcs7Unpad(plainPadded, aesBlockSize)
	if err != nil {
		return nil, fmt.Errorf("backup.DecryptArchive: %w", err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(plain))
	if err != nil {
		return nil, fmt.Errorf("backup.DecryptArchive: gzip: %w", err)
	}
	defer zr.Close()
	zr.Multistream(false)
	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("backup.DecryptArchive: decompress: %w", err)
	}
	return out, nil
}

func parseHeaderPrefix(ciphertext []byte) (extraPrefix []byte, encStart int, err error) {
	if len(ciphertext) < len(magicNumber) {
		return nil, 0, errors.New("backup.DecryptArchive: truncated header")
	}
	if string(ciphertext[:len(magicNumber)]) == magicNumber {
		r := bytes.NewReader(ciphertext[len(magicNumber):])
		metaLen, varintLen, err := readVarintCounted(r)
		if err != nil {
			return nil, 0, fmt.Errorf("backup.DecryptArchive: metadata length: %w", err)
		}
		if _, err := r.Seek(int64(metaLen), io.SeekCurrent); err != nil {
			return nil, 0, fmt.Errorf("backup.DecryptArchive: skip metadata: %w", err)
		}
		start := len(magicNumber) + varintLen + metaLen
		return nil, start, nil
	}
	// Legacy format: first 8 bytes participate in the outer HMAC.
	if len(ciphertext) >= 8 {
		return ciphertext[:8], 0, nil
	}
	return nil, 0, errors.New("backup.DecryptArchive: invalid legacy header")
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("invalid pkcs7 padding length")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errors.New("invalid pkcs7 padding")
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, errors.New("invalid pkcs7 padding bytes")
		}
	}
	return data[:len(data)-padLen], nil
}
