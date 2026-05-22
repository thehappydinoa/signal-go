package libsignal

import (
	"testing"
)

func TestParseUUIDString(t *testing.T) {
	want := [16]byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	got, err := ParseUUIDString("00112233-4455-6677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got=%x want=%x", got, want)
	}
	if s := formatUUID(got); s != "00112233-4455-6677-8899-aabbccddeeff" {
		t.Fatalf("round-trip = %q", s)
	}
}

func TestNewRandomUUID(t *testing.T) {
	u, err := NewRandomUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseUUIDString(u); err != nil {
		t.Fatalf("parse generated uuid: %v", err)
	}
}

func TestGroupSenderKeyRoundTrip(t *testing.T) {
	aliceStore := newInlineSignalStores()
	bobStore := newInlineSignalStores()

	aliceUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	distID := "22222222-3333-4444-5555-666666666666"

	aliceLocal, err := NewAddress(aliceUUID, 1)
	if err != nil {
		t.Fatal(err)
	}
	bobRemote, err := NewAddress(aliceUUID, 1)
	if err != nil {
		t.Fatal(err)
	}

	aliceH := NewStoreHandle(aliceStore)
	defer aliceH.Release()
	bobH := NewStoreHandle(bobStore)
	defer bobH.Release()

	skdm, err := CreateSenderKeyDistributionMessage(aliceLocal, distID, aliceH)
	if err != nil {
		t.Fatalf("CreateSenderKeyDistributionMessage: %v", err)
	}
	skdmBytes, err := skdm.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	inbound, err := DeserializeSenderKeyDistributionMessage(skdmBytes)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProcessSenderKeyDistributionMessage(bobRemote, inbound, bobH); err != nil {
		t.Fatalf("ProcessSenderKeyDistributionMessage: %v", err)
	}

	ptext := []byte("hello group")
	ct, err := GroupEncryptMessage(ptext, aliceLocal, distID, aliceH)
	if err != nil {
		t.Fatalf("GroupEncryptMessage: %v", err)
	}
	typ, err := ct.Type()
	if err != nil {
		t.Fatal(err)
	}
	if typ != CiphertextSenderKey {
		t.Fatalf("ciphertext type = %d, want sender-key", typ)
	}
	ctBytes, err := ct.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	got, err := GroupDecryptMessage(ctBytes, bobRemote, bobH)
	if err != nil {
		t.Fatalf("GroupDecryptMessage: %v", err)
	}
	if string(got) != string(ptext) {
		t.Fatalf("plaintext %q want %q", got, ptext)
	}
}
