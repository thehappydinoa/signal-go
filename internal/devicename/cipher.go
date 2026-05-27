// Package devicename implements linked-device display name encryption
// compatible with Signal Android's DeviceNameCipher (AES-256-CTR + X25519
// agreement + HMAC-SHA256 key derivation). See ADR 0036.
package devicename

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	devicenamepb "github.com/thehappydinoa/signal-go/internal/proto/gen/devicenamepb"
)

const syntheticIVLen = 16

// Encrypt encodes plaintext (UTF-8) as a signalservice.DeviceName protobuf
// wire blob, encrypted for holders of the given Curve25519 identity public
// key (typically the linked account's ACI identity key).
//
// The return value is suitable for JSON field accountAttributes.name on
// PUT /v1/devices/link (base64-encoded protobuf bytes).
func Encrypt(plaintext string, identityPublic *libsignal.PublicKey) (string, error) {
	if plaintext == "" {
		return "", errors.New("devicename.Encrypt: empty plaintext")
	}
	if !utf8.ValidString(plaintext) {
		return "", errors.New("devicename.Encrypt: plaintext is not valid UTF-8")
	}
	if identityPublic == nil {
		return "", errors.New("devicename.Encrypt: nil identity public key")
	}
	return EncryptBytes([]byte(plaintext), identityPublic)
}

// EncryptBytes is like [Encrypt] but accepts the device name as raw bytes.
func EncryptBytes(plaintext []byte, identityPublic *libsignal.PublicKey) (string, error) {
	if len(plaintext) == 0 {
		return "", errors.New("devicename.EncryptBytes: empty plaintext")
	}
	if identityPublic == nil {
		return "", errors.New("devicename.EncryptBytes: nil identity public key")
	}
	ephemeralPriv, err := libsignal.GeneratePrivateKey()
	if err != nil {
		return "", fmt.Errorf("devicename.EncryptBytes: %w", err)
	}
	ephemeralPub, err := ephemeralPriv.PublicKey()
	if err != nil {
		return "", fmt.Errorf("devicename.EncryptBytes: %w", err)
	}

	masterSecret, err := libsignal.Agree(ephemeralPriv, identityPublic)
	if err != nil {
		return "", fmt.Errorf("devicename.EncryptBytes: %w", err)
	}

	syntheticIV, err := computeSyntheticIV(masterSecret, plaintext)
	if err != nil {
		return "", err
	}
	cipherKey, err := computeCipherKey(masterSecret, syntheticIV)
	if err != nil {
		return "", err
	}

	ciphertext, err := aesCTR(cipherKey, plaintext)
	if err != nil {
		return "", err
	}

	ephemeralPubBytes, err := ephemeralPub.Serialize()
	if err != nil {
		return "", fmt.Errorf("devicename.EncryptBytes: %w", err)
	}

	dn := &devicenamepb.DeviceName{
		EphemeralPublic: ephemeralPubBytes,
		SyntheticIv:     syntheticIV,
		Ciphertext:      ciphertext,
	}
	wire, err := proto.Marshal(dn)
	if err != nil {
		return "", fmt.Errorf("devicename.EncryptBytes: marshal: %w", err)
	}
	return base64.StdEncoding.EncodeToString(wire), nil
}

// Decrypt recovers the UTF-8 device name from an encrypted payload produced
// by [Encrypt]. It is exposed for tests and diagnostics; linked devices
// normally only need encryption at registration time.
func Decrypt(nameCipher string, identity *libsignal.IdentityKeyPair) (string, error) {
	b, err := base64.StdEncoding.DecodeString(nameCipher)
	if err != nil {
		return "", fmt.Errorf("devicename.Decrypt: %w", err)
	}
	var dn devicenamepb.DeviceName
	if err := proto.Unmarshal(b, &dn); err != nil {
		return "", fmt.Errorf("devicename.Decrypt: %w", err)
	}
	if len(dn.GetEphemeralPublic()) == 0 || len(dn.GetSyntheticIv()) == 0 || len(dn.GetCiphertext()) == 0 {
		return "", errors.New("devicename.Decrypt: missing device name fields")
	}
	if identity == nil || identity.Private == nil || identity.Public == nil {
		return "", errors.New("devicename.Decrypt: nil identity key pair")
	}

	ephemeralPub, err := libsignal.DeserializePublicKey(dn.GetEphemeralPublic())
	if err != nil {
		return "", fmt.Errorf("devicename.Decrypt: %w", err)
	}

	masterSecret, err := libsignal.Agree(identity.Private, ephemeralPub)
	if err != nil {
		return "", fmt.Errorf("devicename.Decrypt: %w", err)
	}

	syntheticIV := dn.GetSyntheticIv()
	cipherKey, err := computeCipherKey(masterSecret, syntheticIV)
	if err != nil {
		return "", err
	}

	plaintext, err := aesCTR(cipherKey, dn.GetCiphertext())
	if err != nil {
		return "", err
	}

	if err := verifySyntheticIV(masterSecret, plaintext, syntheticIV); err != nil {
		return "", err
	}
	if !utf8.Valid(plaintext) {
		return "", errors.New("devicename.Decrypt: plaintext is not valid UTF-8")
	}
	return string(plaintext), nil
}

func computeSyntheticIV(masterSecret, plaintext []byte) ([]byte, error) {
	// Signal Android DeviceNameCipher derives an auth key first:
	//   authKey = HMAC(masterSecret, "auth")
	//   ivFull  = HMAC(authKey, plaintext)
	authKey, err := hmacSHA256concat(masterSecret, []byte("auth"))
	if err != nil {
		return nil, err
	}
	full, err := hmacSHA256concat(authKey, plaintext)
	if err != nil {
		return nil, err
	}
	if len(full) < syntheticIVLen {
		return nil, errors.New("devicename: synthetic IV derivation too short")
	}
	return append([]byte(nil), full[:syntheticIVLen]...), nil
}

func computeCipherKey(masterSecret, syntheticIV []byte) ([]byte, error) {
	// Signal Android DeviceNameCipher derives a cipher key first:
	//   cipherKeyKey = HMAC(masterSecret, "cipher")
	//   cipherKey    = HMAC(cipherKeyKey, syntheticIV)
	cipherKeyKey, err := hmacSHA256concat(masterSecret, []byte("cipher"))
	if err != nil {
		return nil, err
	}
	return hmacSHA256concat(cipherKeyKey, syntheticIV)
}

// hmacSHA256concat computes HMAC-SHA256(key, parts...) where all parts are
// written sequentially into the MAC (equivalent to HMAC(key, concat(parts))).
func hmacSHA256concat(key []byte, parts ...[]byte) ([]byte, error) {
	m := hmac.New(sha256.New, key)
	for _, p := range parts {
		if _, err := m.Write(p); err != nil {
			return nil, fmt.Errorf("devicename: hmac: %w", err)
		}
	}
	return m.Sum(nil), nil
}

func aesCTR(key, in []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("devicename: AES key length %d, want 32", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("devicename: aes: %w", err)
	}
	stream := cipher.NewCTR(block, make([]byte, aes.BlockSize))
	out := make([]byte, len(in))
	stream.XORKeyStream(out, in)
	return out, nil
}

func verifySyntheticIV(masterSecret, plaintext, syntheticIV []byte) error {
	authKey, err := hmacSHA256concat(masterSecret, []byte("auth"))
	if err != nil {
		return err
	}
	full, err := hmacSHA256concat(authKey, plaintext)
	if err != nil {
		return err
	}
	if len(full) < syntheticIVLen {
		return errors.New("devicename: verification IV too short")
	}
	our := full[:syntheticIVLen]
	if subtle.ConstantTimeCompare(our, syntheticIV) != 1 {
		return errors.New("devicename: synthetic IV mismatch")
	}
	return nil
}
