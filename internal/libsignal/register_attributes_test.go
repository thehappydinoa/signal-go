package libsignal

import (
	"testing"
)

func TestRegistrationAccountAttributesLifecycle(t *testing.T) {
	uak := make([]byte, UnidentifiedAccessKeyLen)
	for i := range uak {
		uak[i] = byte(i + 1)
	}
	attrs, err := NewRegistrationAccountAttributes(RegistrationAccountAttributesOptions{
		RecoveryPassword:               []byte("recovery-bytes"),
		ACIRegistrationID:              1234,
		PNIRegistrationID:              5678,
		RegistrationLock:               "",
		UnidentifiedAccessKey:          uak,
		UnrestrictedUnidentifiedAccess: false,
		Capabilities:                   []string{"transfer", "storage"},
		DiscoverableByPhoneNumber:      false,
	})
	if err != nil {
		t.Fatalf("NewRegistrationAccountAttributes: %v", err)
	}
	defer attrs.Close()
	if attrs.ptr == nil {
		t.Fatal("expected non-nil ptr")
	}
	attrs.Close()
	attrs.Close() // idempotent
}

func TestRegistrationAccountAttributesEmptyCapabilities(t *testing.T) {
	attrs, err := NewRegistrationAccountAttributes(RegistrationAccountAttributesOptions{
		UnidentifiedAccessKey: make([]byte, UnidentifiedAccessKeyLen),
	})
	if err != nil {
		t.Fatalf("NewRegistrationAccountAttributes empty caps: %v", err)
	}
	defer attrs.Close()
}

func TestRegistrationAccountAttributesRejectsWrongUAKSize(t *testing.T) {
	_, err := NewRegistrationAccountAttributes(RegistrationAccountAttributesOptions{
		UnidentifiedAccessKey: make([]byte, 15), // short by one
	})
	if err == nil {
		t.Fatal("expected error for 15-byte UAK")
	}
	_, err = NewRegistrationAccountAttributes(RegistrationAccountAttributesOptions{
		UnidentifiedAccessKey: make([]byte, 17),
	})
	if err == nil {
		t.Fatal("expected error for 17-byte UAK")
	}
}
