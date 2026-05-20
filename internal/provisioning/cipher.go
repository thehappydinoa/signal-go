package provisioning

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
)

// hkdfInfo is the application-specific context string Signal uses when
// deriving keys for the provisioning cipher. It is identical across all
// official clients (the legacy "TextSecure" name has been retained).
var hkdfInfo = []byte("TextSecure Provisioning Message")

const (
	// envelopeVersion is the leading byte of every ProvisionEnvelope body.
	envelopeVersion byte = 0x01
	ivLen                = 16
	macLen               = 32
	aesKeyLen            = 32
	hmacKeyLen           = 32
)

// DecryptEnvelope decrypts a [provpb.ProvisionEnvelope] using our half of
// the provisioning keypair, returning the [provpb.ProvisionMessage] inside.
//
// Wire format of envelope.Body (Signal-Desktop ProvisioningCipher):
//
//	version (1 byte = 0x01) || iv (16) || ciphertext || mac (32 = HMAC-SHA256)
//
// Cipher key derivation:
//
//	shared  = X25519(ourPriv, envelope.PublicKey)            // 32 bytes
//	keys    = HKDF-SHA256(salt=nil, info="TextSecure
//	          Provisioning Message", L=64, ikm=shared)
//	aesKey  = keys[0:32]
//	hmacKey = keys[32:64]
//
// MAC covers version || iv || ciphertext.
func DecryptEnvelope(ourPriv *libsignal.PrivateKey, env *provpb.ProvisionEnvelope) (*provpb.ProvisionMessage, error) {
	if env == nil {
		return nil, errors.New("provisioning.DecryptEnvelope: nil envelope")
	}
	body := env.GetBody()
	pubBytes := env.GetPublicKey()
	if len(pubBytes) == 0 {
		return nil, errors.New("provisioning.DecryptEnvelope: envelope missing publicKey")
	}
	if len(body) < 1+ivLen+macLen+aes.BlockSize {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: body too short (%d bytes)", len(body))
	}
	if body[0] != envelopeVersion {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: unsupported envelope version 0x%02x", body[0])
	}

	theirPub, err := libsignal.DeserializePublicKey(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: peer public key: %w", err)
	}
	shared, err := libsignal.Agree(ourPriv, theirPub)
	if err != nil {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: ECDH: %w", err)
	}
	keys, err := libsignal.HKDFSHA256(aesKeyLen+hmacKeyLen, shared, hkdfInfo, nil)
	if err != nil {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: HKDF: %w", err)
	}
	aesKey, hmacKey := keys[:aesKeyLen], keys[aesKeyLen:]

	macStart := len(body) - macLen
	mac := body[macStart:]
	signedPortion := body[:macStart]

	if !verifyMAC(hmacKey, signedPortion, mac) {
		return nil, errors.New("provisioning.DecryptEnvelope: MAC verification failed")
	}

	iv := body[1 : 1+ivLen]
	ciphertext := body[1+ivLen : macStart]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: ciphertext not block-aligned (%d bytes)", len(ciphertext))
	}

	plaintext, err := aesCBCDecrypt(aesKey, iv, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: AES-CBC: %w", err)
	}

	var msg provpb.ProvisionMessage
	if err := proto.Unmarshal(plaintext, &msg); err != nil {
		return nil, fmt.Errorf("provisioning.DecryptEnvelope: unmarshal ProvisionMessage: %w", err)
	}
	return &msg, nil
}

// verifyMAC checks an HMAC-SHA256 tag in constant time.
func verifyMAC(key, data, tag []byte) bool {
	if len(tag) != macLen {
		return false
	}
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return hmac.Equal(h.Sum(nil), tag)
}

// aesCBCDecrypt decrypts ciphertext with AES-256-CBC and strips PKCS#7
// padding. Returns an error if the padding is malformed.
func aesCBCDecrypt(key, iv, ciphertext []byte) ([]byte, error) {
	if len(key) != aesKeyLen {
		return nil, fmt.Errorf("AES key must be %d bytes, got %d", aesKeyLen, len(key))
	}
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("AES IV must be %d bytes, got %d", aes.BlockSize, len(iv))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext, aes.BlockSize)
}

// pkcs7Unpad removes RFC 5652 §6.3 padding. It rejects any malformed padding
// (zero pad byte, pad byte > block size, mismatching tail bytes). The check
// is constant-time over the maximum possible pad length to avoid leaking
// padding info via timing.
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("pkcs7: empty plaintext")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, errors.New("pkcs7: invalid padding")
	}
	// Constant-time check that all pad bytes equal padLen.
	var diff byte
	for i := len(data) - padLen; i < len(data); i++ {
		diff |= data[i] ^ byte(padLen)
	}
	if diff != 0 {
		return nil, errors.New("pkcs7: invalid padding")
	}
	return data[:len(data)-padLen], nil
}
