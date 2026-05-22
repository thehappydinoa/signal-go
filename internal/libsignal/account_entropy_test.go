package libsignal

import "testing"

func TestDeriveSVRKeyRoundTrip(t *testing.T) {
	pool, err := GenerateAccountEntropyPool()
	if err != nil {
		t.Fatalf("GenerateAccountEntropyPool: %v", err)
	}
	if err := ValidateAccountEntropyPool(pool); err != nil {
		t.Fatalf("ValidateAccountEntropyPool: %v", err)
	}
	key1, err := DeriveSVRKey(pool)
	if err != nil {
		t.Fatalf("DeriveSVRKey: %v", err)
	}
	key2, err := DeriveSVRKey(pool)
	if err != nil {
		t.Fatalf("DeriveSVRKey 2: %v", err)
	}
	if key1 != key2 {
		t.Fatal("DeriveSVRKey not deterministic")
	}
}

func TestDeriveSVRKeyRejectsEmpty(t *testing.T) {
	if _, err := DeriveSVRKey(""); err == nil {
		t.Fatal("expected error for empty pool")
	}
}
