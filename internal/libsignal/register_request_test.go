package libsignal

import (
	"testing"
)

func TestRegisterAccountRequestLifecycle(t *testing.T) {
	req, err := NewRegisterAccountRequest()
	if err != nil {
		t.Fatalf("NewRegisterAccountRequest: %v", err)
	}
	defer req.Close()
	if req.ptr == nil {
		t.Fatal("expected non-nil ptr")
	}

	if err := req.SetAccountPassword("hunter2"); err != nil {
		t.Fatalf("SetAccountPassword: %v", err)
	}
	if err := req.SetSkipDeviceTransfer(); err != nil {
		t.Fatalf("SetSkipDeviceTransfer: %v", err)
	}
	if err := req.SetOneTimePassword(123456); err != nil {
		t.Fatalf("SetOneTimePassword: %v", err)
	}
	if err := req.SetAPNPushToken("apn-token-ignored-by-no-number-flow"); err != nil {
		t.Fatalf("SetAPNPushToken: %v", err)
	}
}

func TestRegisterAccountRequestSetIdentityKeys(t *testing.T) {
	req, err := NewRegisterAccountRequest()
	if err != nil {
		t.Fatalf("NewRegisterAccountRequest: %v", err)
	}
	defer req.Close()

	aciPriv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	aciPub, err := aciPriv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if err := req.SetIdentityPublicKey(IdentityACI, aciPub); err != nil {
		t.Fatalf("SetIdentityPublicKey ACI: %v", err)
	}

	pniPriv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey pni: %v", err)
	}
	pniPub, err := pniPriv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey pni: %v", err)
	}
	if err := req.SetIdentityPublicKey(IdentityPNI, pniPub); err != nil {
		t.Fatalf("SetIdentityPublicKey PNI: %v", err)
	}
}

func TestRegisterAccountRequestSetSignedPreKeys(t *testing.T) {
	req, err := NewRegisterAccountRequest()
	if err != nil {
		t.Fatalf("NewRegisterAccountRequest: %v", err)
	}
	defer req.Close()

	spkPriv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey spk: %v", err)
	}
	spkPub, err := spkPriv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey spk: %v", err)
	}
	spkSig := make([]byte, 64) // placeholder; the FFI setter does not verify
	err = req.SetIdentitySignedPreKey(IdentityACI, SignedECPreKey{
		KeyID:     1,
		PublicKey: spkPub,
		Signature: spkSig,
	})
	if err != nil {
		t.Fatalf("SetIdentitySignedPreKey: %v", err)
	}

	kyberKP, err := GenerateKyberKeyPair()
	if err != nil {
		t.Fatalf("GenerateKyberKeyPair: %v", err)
	}
	kyberPub, err := kyberKP.Public()
	if err != nil {
		t.Fatalf("Kyber PublicKey: %v", err)
	}
	err = req.SetIdentityPQLastResortPreKey(IdentityACI, SignedKyberPreKey{
		KeyID:     42,
		PublicKey: kyberPub,
		Signature: make([]byte, 64),
	})
	if err != nil {
		t.Fatalf("SetIdentityPQLastResortPreKey: %v", err)
	}
}

func TestRegisterAccountRequestRejectsClosed(t *testing.T) {
	req, err := NewRegisterAccountRequest()
	if err != nil {
		t.Fatalf("NewRegisterAccountRequest: %v", err)
	}
	req.Close()
	req.Close() // idempotent

	if err := req.SetAccountPassword("x"); err == nil {
		t.Fatal("SetAccountPassword on closed: expected error")
	}
	if err := req.SetIdentityPublicKey(IdentityACI, nil); err == nil {
		t.Fatal("SetIdentityPublicKey on closed: expected error")
	}
}
