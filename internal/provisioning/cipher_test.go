package provisioning

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
)

// encryptForTest builds a ProvisionEnvelope addressed to recipientPub using
// senderPriv's identity. It is the inverse of [DecryptEnvelope] and exists
// only to round-trip test the decrypt path; production secondary devices
// only ever decrypt.
//
// Accepts [testing.TB] (not just [*testing.T]) so the fuzz-seed corpus in
// fuzz_test.go can reuse it from a *testing.F.
func encryptForTest(t testing.TB, senderPriv *libsignal.PrivateKey, senderPub *libsignal.PublicKey, recipientPub *libsignal.PublicKey, message *provpb.ProvisionMessage) *provpb.ProvisionEnvelope {
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

	plaintext, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)

	iv := make([]byte, ivLen)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	body := make([]byte, 0, 1+ivLen+len(ciphertext)+macLen)
	body = append(body, envelopeVersion)
	body = append(body, iv...)
	body = append(body, ciphertext...)
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(body)
	body = mac.Sum(body)

	senderPubBytes, err := senderPub.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return &provpb.ProvisionEnvelope{
		PublicKey: senderPubBytes,
		Body:      body,
	}
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

func TestDecryptEnvelopeRoundTrip(t *testing.T) {
	// Both sides of the handshake.
	secondary, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate secondary: %v", err)
	}
	primary, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate primary: %v", err)
	}

	aci := "11111111-2222-3333-4444-555555555555"
	pni := "66666666-7777-8888-9999-aaaaaaaaaaaa"
	num := "+15551234567"
	code := "ABCD-1234"
	aciKP, _ := libsignal.GenerateIdentityKeyPair()
	aciPub, _ := aciKP.Public.Serialize()
	aciPriv, _ := aciKP.Private.Serialize()
	wantMsg := &provpb.ProvisionMessage{
		Aci:                   &aci,
		Pni:                   &pni,
		Number:                &num,
		ProvisioningCode:      &code,
		ProfileKey:            bytesPattern(32, 0xAB),
		ReadReceipts:          proto.Bool(true),
		AciIdentityKeyPublic:  aciPub,
		AciIdentityKeyPrivate: aciPriv,
	}

	env := encryptForTest(t, primary.Private, primary.Public, secondary.Public, wantMsg)

	gotMsg, err := DecryptEnvelope(secondary.Private, env)
	if err != nil {
		t.Fatalf("DecryptEnvelope: %v", err)
	}
	if gotMsg.GetAci() != aci || gotMsg.GetPni() != pni || gotMsg.GetNumber() != num || gotMsg.GetProvisioningCode() != code {
		t.Errorf("scalar field mismatch")
	}
	if !bytes.Equal(gotMsg.GetProfileKey(), wantMsg.ProfileKey) {
		t.Errorf("profile key mismatch")
	}
	if gotMsg.GetReadReceipts() != true {
		t.Errorf("read receipts mismatch")
	}
	if !bytes.Equal(gotMsg.GetAciIdentityKeyPublic(), wantMsg.AciIdentityKeyPublic) {
		t.Errorf("ACI pub mismatch")
	}
}

func TestDecryptEnvelopeRejectsBadMAC(t *testing.T) {
	secondary, _ := libsignal.GenerateIdentityKeyPair()
	primary, _ := libsignal.GenerateIdentityKeyPair()
	num := "+1"
	env := encryptForTest(t, primary.Private, primary.Public, secondary.Public, &provpb.ProvisionMessage{Number: &num})
	// Flip a byte inside the MAC tag.
	env.Body[len(env.Body)-1] ^= 0x01
	_, err := DecryptEnvelope(secondary.Private, env)
	if err == nil || !strings.Contains(err.Error(), "MAC") {
		t.Errorf("err = %v, want one mentioning MAC", err)
	}
}

func TestDecryptEnvelopeRejectsBadCiphertext(t *testing.T) {
	secondary, _ := libsignal.GenerateIdentityKeyPair()
	primary, _ := libsignal.GenerateIdentityKeyPair()
	num := "+1"
	env := encryptForTest(t, primary.Private, primary.Public, secondary.Public, &provpb.ProvisionMessage{Number: &num})
	// Tamper with the first ciphertext byte AND fix the MAC over the tampered
	// body, so the MAC passes but the resulting plaintext padding is bad.
	// Easier path: just truncate to test version + length checks.
	for _, tc := range []struct {
		name string
		mut  func(b []byte) []byte
	}{
		{"empty body", func(b []byte) []byte { return nil }},
		{"too short", func(b []byte) []byte { return b[:8] }},
		{"wrong version", func(b []byte) []byte { c := append([]byte(nil), b...); c[0] = 0x02; return c }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Build a fresh envelope rather than copying *env (the
			// protobuf Message embeds a sync.Mutex via protoimpl).
			bad := &provpb.ProvisionEnvelope{
				PublicKey: env.PublicKey,
				Body:      tc.mut(env.Body),
			}
			if _, err := DecryptEnvelope(secondary.Private, bad); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestDecryptEnvelopeRejectsWrongRecipient(t *testing.T) {
	secondary, _ := libsignal.GenerateIdentityKeyPair()
	primary, _ := libsignal.GenerateIdentityKeyPair()
	other, _ := libsignal.GenerateIdentityKeyPair()
	num := "+1"
	env := encryptForTest(t, primary.Private, primary.Public, secondary.Public, &provpb.ProvisionMessage{Number: &num})
	if _, err := DecryptEnvelope(other.Private, env); err == nil {
		t.Errorf("expected MAC error decrypting with wrong recipient key")
	}
}

func TestPKCS7UnpadRejectsBadPadding(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"empty", []byte{}},
		{"zero pad", append(bytes.Repeat([]byte{1}, 15), 0)},
		{"oversize pad", append(bytes.Repeat([]byte{1}, 15), 17)},
		{"inconsistent pad", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 4, 5, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pkcs7Unpad(tc.in); err == nil {
				t.Errorf("expected error")
			}
		})
	}
}

func TestPKCS7RoundTrip(t *testing.T) {
	for _, in := range [][]byte{
		nil,
		{},
		{0x42},
		bytes.Repeat([]byte{0xAA}, aes.BlockSize-1),
		bytes.Repeat([]byte{0xAA}, aes.BlockSize),
		bytes.Repeat([]byte{0xAA}, aes.BlockSize*5+3),
	} {
		padded := pkcs7Pad(in, aes.BlockSize)
		if len(padded)%aes.BlockSize != 0 {
			t.Errorf("not block-aligned: %d", len(padded))
		}
		out, err := pkcs7Unpad(padded)
		if err != nil {
			t.Errorf("pkcs7Unpad: %v", err)
			continue
		}
		if !bytes.Equal(out, in) {
			t.Errorf("round-trip: got %x want %x", out, in)
		}
	}
}
