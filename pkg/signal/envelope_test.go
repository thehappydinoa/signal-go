package signal

// This file is part of the same package as link.go but lives outside the
// production code. It exists so link_test.go can build an encrypted
// ProvisionEnvelope locally without exporting test-only helpers from the
// internal/provisioning package.
//
// The encrypt-side logic mirrors internal/provisioning.encryptForTest and
// is the inverse of provisioning.DecryptEnvelope. Keeping it in a _test.go
// file means it is excluded from production builds.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
)

const envelopeVersion byte = 0x01

const (
	ivLen      = 16
	macLen     = 32
	aesKeyLen  = 32
	hmacKeyLen = 32
)

var hkdfInfo = []byte("TextSecure Provisioning Message")

func encryptEnvelope(t *testing.T, senderPriv *libsignal.PrivateKey, senderPub *libsignal.PublicKey, recipientPub *libsignal.PublicKey, message *provpb.ProvisionMessage) *provpb.ProvisionEnvelope {
	t.Helper()
	shared, err := libsignal.Agree(senderPriv, recipientPub)
	if err != nil {
		t.Fatalf("Agree: %v", err)
	}
	keys, err := libsignal.HKDFSHA256(aesKeyLen+hmacKeyLen, shared, hkdfInfo, nil)
	if err != nil {
		t.Fatalf("HKDF: %v", err)
	}
	aesKey, hmacKey := keys[:aesKeyLen], keys[aesKeyLen:]
	plaintext, _ := proto.Marshal(message)
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand: %v", err)
	}
	block, _ := aes.NewCipher(aesKey)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	body := make([]byte, 0, 1+ivLen+len(ct)+macLen)
	body = append(body, envelopeVersion)
	body = append(body, iv...)
	body = append(body, ct...)
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(body)
	body = mac.Sum(body)
	pub, _ := senderPub.Serialize()
	return &provpb.ProvisionEnvelope{PublicKey: pub, Body: body}
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+padLen)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
}
