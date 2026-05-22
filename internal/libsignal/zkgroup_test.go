package libsignal

import (
	"testing"
)

func TestParseServiceIDStringRoundTrip(t *testing.T) {
	const aci = "00010203-0405-0607-0809-0a0b0c0d0e0f"
	fixed, err := ParseServiceIDString(aci)
	if err != nil {
		t.Fatalf("ParseServiceIDString: %v", err)
	}
	got, err := ServiceIDString(fixed)
	if err != nil {
		t.Fatalf("ServiceIDString: %v", err)
	}
	if got != aci {
		t.Fatalf("round trip = %q, want %q", got, aci)
	}
}

func TestProductionServerPublicParamsDeserialize(t *testing.T) {
	p, err := ProductionServerPublicParams()
	if err != nil {
		t.Fatalf("ProductionServerPublicParams: %v", err)
	}
	if p == nil {
		t.Fatal("nil params")
	}
}

func TestGroupSecretParamsBlobRoundTrip(t *testing.T) {
	master := make([]byte, GroupMasterKeyLen)
	for i := range master {
		master[i] = byte(i + 1)
	}
	secret, err := GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatalf("GroupSecretParamsFromMasterKey: %v", err)
	}

	var randomness [ZKRandomnessLen]byte
	for i := range randomness {
		randomness[i] = byte(i + 3)
	}
	plain := []byte("hello group")
	ct, err := GroupSecretParamsEncryptBlob(secret, plain, randomness)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	out, err := GroupSecretParamsDecryptBlob(secret, ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(out) != string(plain) {
		t.Fatalf("decrypt = %q, want %q", out, plain)
	}
}

func TestGroupSecretParamsServiceIDRoundTrip(t *testing.T) {
	master := make([]byte, GroupMasterKeyLen)
	for i := range master {
		master[i] = byte(0x40 + i)
	}
	secret, err := GroupSecretParamsFromMasterKey(master)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	const aci = "64656667-6869-6a6b-6c6d-6e6f70717273"
	id := MustParseServiceIDString(aci)
	ct, err := GroupSecretParamsEncryptServiceID(secret, id)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := GroupSecretParamsDecryptServiceID(secret, ct[:])
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	s, err := ServiceIDString(got)
	if err != nil {
		t.Fatalf("ServiceIDString: %v", err)
	}
	if s != aci {
		t.Fatalf("aci = %q, want %q", s, aci)
	}
}
