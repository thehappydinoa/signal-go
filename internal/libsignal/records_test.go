package libsignal

import (
	"bytes"
	"testing"
)

func TestPreKeyRecordRoundTrip(t *testing.T) {
	priv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	rec, err := NewPreKeyRecord(42, priv, pub)
	if err != nil {
		t.Fatalf("NewPreKeyRecord: %v", err)
	}
	gotID, err := rec.ID()
	if err != nil || gotID != 42 {
		t.Errorf("ID() = %d (err %v), want 42", gotID, err)
	}
	bytesA, err := rec.Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	rec2, err := DeserializePreKeyRecord(bytesA)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}
	bytesB, err := rec2.Serialize()
	if err != nil {
		t.Fatalf("Serialize 2: %v", err)
	}
	if !bytes.Equal(bytesA, bytesB) {
		t.Errorf("PreKeyRecord round-trip mismatch:\n A=%x\n B=%x", bytesA, bytesB)
	}
	if id2, _ := rec2.ID(); id2 != 42 {
		t.Errorf("rec2.ID = %d", id2)
	}
}

func TestDeserializeRejectsEmpty(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		if _, err := DeserializeSessionRecord(nil); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("prekey", func(t *testing.T) {
		if _, err := DeserializePreKeyRecord(nil); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("signed-prekey", func(t *testing.T) {
		if _, err := DeserializeSignedPreKeyRecord(nil); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("kyber-prekey", func(t *testing.T) {
		if _, err := DeserializeKyberPreKeyRecord(nil); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("sender-key", func(t *testing.T) {
		if _, err := DeserializeSenderKeyRecord(nil); err == nil {
			t.Error("expected error")
		}
	})
}

func TestDeserializeRejectsGarbage(t *testing.T) {
	garbage := []byte{0x01, 0x02, 0x03}
	if _, err := DeserializeSessionRecord(garbage); err == nil {
		t.Error("expected error for garbage session record")
	}
	if _, err := DeserializePreKeyRecord(garbage); err == nil {
		t.Error("expected error for garbage prekey record")
	}
}
