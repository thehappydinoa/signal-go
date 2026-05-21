package libsignal

import (
	"testing"
	"time"
)

func TestStripVersionByte(t *testing.T) {
	in := []byte{0x33, 0x01, 0x02}
	out, stripped := StripVersionByte(in)
	if !stripped || len(out) != 2 {
		t.Fatalf("stripped=%v out=%x", stripped, out)
	}
}

func TestDecryptRoundTripPreKeyThenWhisper(t *testing.T) {
	aliceStore := newInlineSignalStores()
	bobStore := newInlineSignalStores()

	aliceID, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	bobID, err := GenerateIdentityKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	alicePub, _ := aliceID.Public.Serialize()
	alicePriv, _ := aliceID.Private.Serialize()
	bobPub, _ := bobID.Public.Serialize()
	bobPriv, _ := bobID.Private.Serialize()

	const aliceReg, bobReg uint32 = 4242, 4343
	aliceStore.SetLocalIdentity(alicePub, alicePriv, aliceReg)
	bobStore.SetLocalIdentity(bobPub, bobPriv, bobReg)

	bobSPKPriv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	bobSPKPub, err := bobSPKPriv.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	bobSPKPubBytes, _ := bobSPKPub.Serialize()
	bobSPKSig, err := Sign(bobID.Private, bobSPKPubBytes)
	if err != nil {
		t.Fatal(err)
	}
	bobKyberKP, err := GenerateKyberKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	kyberPub, _ := bobKyberKP.Public()
	kyberPubBytes, _ := kyberPub.Serialize()
	kyberSig, err := Sign(bobID.Private, kyberPubBytes)
	if err != nil {
		t.Fatal(err)
	}

	spkBlob, err := NewSignedPreKeyRecordBlob(1, uint64(time.Now().UnixMilli()), bobSPKPub, bobSPKPriv, bobSPKSig)
	if err != nil {
		t.Fatal(err)
	}
	if err := bobStore.StoreSignedPreKey(1, spkBlob); err != nil {
		t.Fatal(err)
	}

	kyberBlob, err := NewKyberPreKeyRecordBlob(1, uint64(time.Now().UnixMilli()), bobKyberKP, kyberSig)
	if err != nil {
		t.Fatal(err)
	}
	if err := bobStore.StoreKyberPreKey(1, kyberBlob); err != nil {
		t.Fatal(err)
	}

	otPriv, err := GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	otPub, err := otPriv.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	const otPreKeyID uint32 = 2
	otRec, err := NewPreKeyRecord(otPreKeyID, otPriv, otPub)
	if err != nil {
		t.Fatal(err)
	}
	otBlob, err := otRec.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	if err := bobStore.StorePreKey(otPreKeyID, otBlob); err != nil {
		t.Fatal(err)
	}

	bundle, err := NewPreKeyBundle(PreKeyBundleParams{
		RegistrationID:        bobReg,
		DeviceID:              1,
		PreKeyID:              otPreKeyID,
		PreKeyPublic:          otPub,
		SignedPreKeyID:        1,
		SignedPreKeyPublic:    bobSPKPub,
		SignedPreKeySignature: bobSPKSig,
		IdentityKey:           bobID.Public,
		KyberPreKeyID:         1,
		KyberPreKeyPublic:     kyberPub,
		KyberPreKeySignature:  kyberSig,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Destroy()

	aliceUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	bobUUID := "11111111-2222-3333-4444-555555555555"
	now := time.Now()

	aliceRemote, _ := NewAddress(bobUUID, 1)
	aliceLocal, _ := NewAddress(aliceUUID, 1)
	bobRemote, _ := NewAddress(aliceUUID, 1)
	bobLocal, _ := NewAddress(bobUUID, 1)

	aliceH := NewStoreHandle(aliceStore)
	defer aliceH.Release()
	if err := ProcessPreKeyBundle(bundle, aliceRemote, aliceLocal, aliceH, now); err != nil {
		t.Fatalf("ProcessPreKeyBundle: %v", err)
	}

	ptext := []byte("hello signal-go")
	ct, err := EncryptMessage(ptext, aliceRemote, aliceLocal, aliceH, now)
	if err != nil {
		t.Fatalf("EncryptMessage: %v", err)
	}
	ctBytes, err := ct.Serialize()
	if err != nil {
		t.Fatal(err)
	}
	ctType, err := ct.Type()
	if err != nil {
		t.Fatal(err)
	}

	bobH := NewStoreHandle(bobStore)
	defer bobH.Release()

	var got []byte
	switch ctType {
	case CiphertextPreKey:
		pkm, err := DeserializePreKeySignalMessage(ctBytes)
		if err != nil {
			t.Fatal(err)
		}
		got, err = DecryptPreKeySignalMessage(pkm, bobRemote, bobLocal, bobH)
	case CiphertextWhisper:
		sm, err := DeserializeSignalMessage(ctBytes)
		if err != nil {
			t.Fatal(err)
		}
		got, err = DecryptSignalMessage(sm, bobRemote, bobLocal, bobH)
	default:
		t.Fatalf("unexpected ciphertext type %d", ctType)
	}
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != string(ptext) {
		t.Fatalf("plaintext %q want %q", got, ptext)
	}
}
