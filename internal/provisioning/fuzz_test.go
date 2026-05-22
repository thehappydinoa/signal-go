package provisioning

// Fuzz targets for the provisioning cipher. These exercise the parse +
// MAC-check + decrypt + protobuf-unmarshal pipeline against arbitrary
// adversarial inputs, which is exactly the threat model the Phase 8
// audit cares about (a malicious primary device sends a hostile envelope
// to a linking secondary device).
//
// The fuzzers must never panic. Returning typed errors is fine; the
// goal is to surface use-after-free, out-of-bounds, or invariant
// violations in the decrypt path.

import (
	"testing"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
)

// FuzzDecryptEnvelope drives [DecryptEnvelope] with fuzzed envelope.Body
// and envelope.PublicKey bytes against a fresh recipient key. The seed
// corpus mixes well-formed envelopes with a few known-bad shapes so the
// fuzzer starts from interesting coverage.
//
// Run for a fixed budget locally:
//
//	go test -run=^$ -fuzz=FuzzDecryptEnvelope -fuzztime=5m \
//	    ./internal/provisioning
func FuzzDecryptEnvelope(f *testing.F) {
	secondary, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		f.Fatalf("GenerateIdentityKeyPair: %v", err)
	}
	primary, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		f.Fatalf("GenerateIdentityKeyPair: %v", err)
	}
	num := "+15551234567"
	good := encryptForTest(f, primary.Private, primary.Public, secondary.Public, &provpb.ProvisionMessage{Number: &num})
	f.Add(good.GetBody(), good.GetPublicKey())
	f.Add([]byte{0x01, 0x02, 0x03}, good.GetPublicKey())
	f.Add(good.GetBody(), []byte(nil))
	bad := append([]byte(nil), good.GetBody()...)
	if len(bad) > 0 {
		bad[0] = 0xFF
	}
	f.Add(bad, good.GetPublicKey())

	recipientPriv := secondary.Private
	f.Fuzz(func(t *testing.T, body, pub []byte) {
		env := &provpb.ProvisionEnvelope{Body: body, PublicKey: pub}
		// The contract for DecryptEnvelope is "fails closed on every
		// adversarial input"; we only assert that it does not panic.
		_, _ = DecryptEnvelope(recipientPriv, env)
	})
}

// FuzzPKCS7Unpad exercises [pkcs7Unpad] directly. The function must
// never panic regardless of input — it either returns a stripped
// plaintext or an error.
func FuzzPKCS7Unpad(f *testing.F) {
	f.Add([]byte{0x01})
	f.Add([]byte{0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x10})
	f.Add(make([]byte, 32))
	f.Add([]byte{0x42, 0x00})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = pkcs7Unpad(data, 16)
	})
}
