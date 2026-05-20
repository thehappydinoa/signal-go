package signal

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	provpb "github.com/thehappydinoa/signal-go/internal/proto/gen/provisioningpb"
	"github.com/thehappydinoa/signal-go/internal/provisioning"
)

// validProvisionMessage builds a complete ProvisionMessage whose identity
// keys are real (and thus deserialize-able by libsignal).
func validProvisionMessage(t *testing.T) *provpb.ProvisionMessage {
	t.Helper()
	aciKP, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate ACI: %v", err)
	}
	pniKP, err := libsignal.GenerateIdentityKeyPair()
	if err != nil {
		t.Fatalf("Generate PNI: %v", err)
	}
	aciPub, _ := aciKP.Public.Serialize()
	aciPriv, _ := aciKP.Private.Serialize()
	pniPub, _ := pniKP.Public.Serialize()
	pniPriv, _ := pniKP.Private.Serialize()

	aci := "aaaa-1111"
	pni := "bbbb-2222"
	num := "+15555550000"
	code := "code-xyz"
	return &provpb.ProvisionMessage{
		Aci:                   &aci,
		Pni:                   &pni,
		Number:                &num,
		ProvisioningCode:      &code,
		ProfileKey:            make([]byte, 32),
		ReadReceipts:          proto.Bool(true),
		AciIdentityKeyPublic:  aciPub,
		AciIdentityKeyPrivate: aciPriv,
		PniIdentityKeyPublic:  pniPub,
		PniIdentityKeyPrivate: pniPriv,
	}
}

func TestConvertSessionHappyPath(t *testing.T) {
	msg := validProvisionMessage(t)
	ephem, _ := libsignal.GenerateIdentityKeyPair()
	got, err := convertSession(&provisioning.Session{EphemeralKey: ephem, Message: msg})
	if err != nil {
		t.Fatalf("convertSession: %v", err)
	}
	if got.ACI != msg.GetAci() || got.PNI != msg.GetPni() {
		t.Errorf("UUIDs: got ACI=%q PNI=%q", got.ACI, got.PNI)
	}
	if got.Number != msg.GetNumber() {
		t.Errorf("number: %q", got.Number)
	}
	if got.ProvisioningCode != msg.GetProvisioningCode() {
		t.Errorf("code: %q", got.ProvisioningCode)
	}
	if !got.ReadReceipts {
		t.Errorf("read receipts not propagated")
	}
	if len(got.ACIIdentityKey.Public) != 33 || len(got.ACIIdentityKey.Private) != 32 {
		t.Errorf("ACI key lengths: pub=%d priv=%d", len(got.ACIIdentityKey.Public), len(got.ACIIdentityKey.Private))
	}
	if len(got.PNIIdentityKey.Public) != 33 || len(got.PNIIdentityKey.Private) != 32 {
		t.Errorf("PNI key lengths: pub=%d priv=%d", len(got.PNIIdentityKey.Public), len(got.PNIIdentityKey.Private))
	}
}

func TestConvertSessionMissingFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*provpb.ProvisionMessage)
		want   string
	}{
		{
			name:   "missing ACI",
			mutate: func(m *provpb.ProvisionMessage) { m.Aci = proto.String("") },
			want:   "missing required fields",
		},
		{
			name:   "missing number",
			mutate: func(m *provpb.ProvisionMessage) { m.Number = proto.String("") },
			want:   "missing required fields",
		},
		{
			name:   "missing provisioning code",
			mutate: func(m *provpb.ProvisionMessage) { m.ProvisioningCode = proto.String("") },
			want:   "missing required fields",
		},
		{
			name:   "missing ACI pub",
			mutate: func(m *provpb.ProvisionMessage) { m.AciIdentityKeyPublic = nil },
			want:   "ACI identity key missing",
		},
		{
			name:   "missing PNI priv",
			mutate: func(m *provpb.ProvisionMessage) { m.PniIdentityKeyPrivate = nil },
			want:   "PNI identity key missing",
		},
		{
			name:   "garbled ACI pub",
			mutate: func(m *provpb.ProvisionMessage) { m.AciIdentityKeyPublic = []byte{0x99, 0x99} },
			want:   "ACI public key invalid",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := validProvisionMessage(t)
			tc.mutate(msg)
			_, err := convertSession(&provisioning.Session{Message: msg})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestConvertSessionRejectsNil(t *testing.T) {
	if _, err := convertSession(nil); err == nil {
		t.Error("expected error for nil session")
	}
	if _, err := convertSession(&provisioning.Session{Message: nil}); err == nil {
		t.Error("expected error for nil message")
	}
}
