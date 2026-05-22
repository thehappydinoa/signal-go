package storage

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

type fakeFetcher struct {
	manifest  *storagepb.StorageManifest
	unchanged bool
	items     []*storagepb.StorageItem
}

func (f *fakeFetcher) FetchManifest(_ context.Context, _ uint64) (*storagepb.StorageManifest, bool, error) {
	if f.unchanged {
		return nil, true, nil
	}
	return f.manifest, false, nil
}

func (f *fakeFetcher) ReadRecords(_ context.Context, _ [][]byte) ([]*storagepb.StorageItem, error) {
	return f.items, nil
}

func TestPullUnchanged(t *testing.T) {
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatalf("GenerateAccountEntropyPool: %v", err)
	}
	keys, err := DeriveKeys(pool)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Pull(context.Background(), keys, &fakeFetcher{unchanged: true}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Unchanged || result.Version != 42 {
		t.Fatalf("got %+v", result)
	}
}

func TestPullContactsAndGroups(t *testing.T) {
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatalf("GenerateAccountEntropyPool: %v", err)
	}
	keys, err := DeriveKeys(pool)
	if err != nil {
		t.Fatal(err)
	}

	contactID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	groupID := []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20}
	masterKey := []byte("01234567890123456789012345678901")

	manifestRecord := &storagepb.ManifestRecord{
		Version: 7,
		Identifiers: []*storagepb.ManifestRecord_Identifier{
			{Raw: contactID, Type: storagepb.ManifestRecord_Identifier_CONTACT},
			{Raw: groupID, Type: storagepb.ManifestRecord_Identifier_GROUPV2},
		},
	}
	manifestPlain, err := proto.Marshal(manifestRecord)
	if err != nil {
		t.Fatal(err)
	}
	manifestKey := DeriveManifestKey(keys.StorageServiceKey, 7)
	manifestCipher, err := EncryptRecordForTest(manifestKey, manifestPlain, makeIV(0xaa))
	if err != nil {
		t.Fatal(err)
	}

	contactRecord := &storagepb.StorageRecord{
		Record: &storagepb.StorageRecord_Contact{
			Contact: &storagepb.ContactRecord{
				Aci:        "00000000-0000-4000-8000-000000000001",
				GivenName:  "Alice",
				FamilyName: "Example",
			},
		},
	}
	groupRecord := &storagepb.StorageRecord{
		Record: &storagepb.StorageRecord_GroupV2{
			GroupV2: &storagepb.GroupV2Record{
				MasterKey: masterKey,
				Archived:  true,
			},
		},
	}
	contactPlain, err := proto.Marshal(contactRecord)
	if err != nil {
		t.Fatal(err)
	}
	groupPlain, err := proto.Marshal(groupRecord)
	if err != nil {
		t.Fatal(err)
	}
	contactItemKey, err := DeriveItemKey(keys.StorageServiceKey, nil, contactID)
	if err != nil {
		t.Fatal(err)
	}
	groupItemKey, err := DeriveItemKey(keys.StorageServiceKey, nil, groupID)
	if err != nil {
		t.Fatal(err)
	}
	contactCipher, err := EncryptRecordForTest(contactItemKey, contactPlain, makeIV(0xbb))
	if err != nil {
		t.Fatal(err)
	}
	groupCipher, err := EncryptRecordForTest(groupItemKey, groupPlain, makeIV(0xcc))
	if err != nil {
		t.Fatal(err)
	}

	fetcher := &fakeFetcher{
		manifest: &storagepb.StorageManifest{Version: 7, Value: manifestCipher},
		items: []*storagepb.StorageItem{
			{Key: contactID, Value: contactCipher},
			{Key: groupID, Value: groupCipher},
		},
	}
	result, err := Pull(context.Background(), keys, fetcher, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != 7 {
		t.Fatalf("version = %d", result.Version)
	}
	if len(result.Contacts) != 1 || result.Contacts[0].GivenName != "Alice" {
		t.Fatalf("contacts = %+v", result.Contacts)
	}
	if len(result.Groups) != 1 || !result.Groups[0].Archived {
		t.Fatalf("groups = %+v", result.Groups)
	}
}

func makeIV(b byte) []byte {
	iv := make([]byte, ivLen)
	for i := range iv {
		iv[i] = b
	}
	return iv
}
