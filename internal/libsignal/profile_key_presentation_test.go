package libsignal

import "testing"

func TestProfileKeyPresentationRoundTrip(t *testing.T) {
	profileKey := make([]byte, ProfileKeyLen)
	for i := range profileKey {
		profileKey[i] = byte(i + 3)
	}
	master := make([]byte, GroupMasterKeyLen)
	for i := range master {
		master[i] = byte(i + 2)
	}
	const aci = "00010203-0405-0607-0809-0a0b0c0d0e0f"

	presentation, err := TestingProfileKeyPresentationRoundTrip(aci, profileKey, master)
	if err != nil {
		t.Fatal(err)
	}

	secretParams, err := GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatal(err)
	}
	uuidCT, err := ProfileKeyPresentationUUIDCiphertext(presentation)
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := GroupSecretParamsDecryptServiceID(secretParams, uuidCT[:])
	if err != nil {
		t.Fatal(err)
	}
	gotACI, err := ServiceIDString(gotID)
	if err != nil {
		t.Fatal(err)
	}
	if gotACI != aci {
		t.Fatalf("aci = %q, want %q", gotACI, aci)
	}
}
