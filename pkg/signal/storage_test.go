package signal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
	"github.com/thehappydinoa/signal-go/internal/storage"
	"github.com/thehappydinoa/signal-go/internal/store/memstore"
	"github.com/thehappydinoa/signal-go/internal/web"
)

func TestSyncStoragePullsContactsAndGroups(t *testing.T) {
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatalf("GenerateAccountEntropyPool: %v", err)
	}
	keys, err := storage.DeriveKeys(pool)
	if err != nil {
		t.Fatal(err)
	}

	contactID := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	groupID := []byte{16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	masterKey := []byte("01234567890123456789012345678901")

	manifestRecord := &storagepb.ManifestRecord{
		Version: 1,
		Identifiers: []*storagepb.ManifestRecord_Identifier{
			{Raw: contactID, Type: storagepb.ManifestRecord_Identifier_CONTACT},
			{Raw: groupID, Type: storagepb.ManifestRecord_Identifier_GROUPV2},
		},
	}
	manifestPlain, err := proto.Marshal(manifestRecord)
	if err != nil {
		t.Fatal(err)
	}
	manifestKey := storage.DeriveManifestKey(keys.StorageServiceKey, 1)
	manifestCipher, err := storage.EncryptRecordForTest(manifestKey, manifestPlain, testIV(0x01))
	if err != nil {
		t.Fatal(err)
	}
	manifest := &storagepb.StorageManifest{Version: 1, Value: manifestCipher}

	contactRecord := &storagepb.StorageRecord{
		Record: &storagepb.StorageRecord_Contact{
			Contact: &storagepb.ContactRecord{
				Aci:        "00000000-0000-4000-8000-000000000099",
				ProfileKey: bytes32(0x42),
			},
		},
	}
	groupRecord := &storagepb.StorageRecord{
		Record: &storagepb.StorageRecord_GroupV2{
			GroupV2: &storagepb.GroupV2Record{MasterKey: masterKey},
		},
	}
	contactPlain, _ := proto.Marshal(contactRecord)
	groupPlain, _ := proto.Marshal(groupRecord)
	contactItemKey, _ := storage.DeriveItemKey(keys.StorageServiceKey, nil, contactID)
	groupItemKey, _ := storage.DeriveItemKey(keys.StorageServiceKey, nil, groupID)
	contactCipher, _ := storage.EncryptRecordForTest(contactItemKey, contactPlain, testIV(0x02))
	groupCipher, _ := storage.EncryptRecordForTest(groupItemKey, groupPlain, testIV(0x03))
	items := &storagepb.StorageItems{
		Items: []*storagepb.StorageItem{
			{Key: contactID, Value: contactCipher},
			{Key: groupID, Value: groupCipher},
		},
	}
	itemsRaw, _ := proto.Marshal(items)
	manifestRaw, _ := proto.Marshal(manifest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/storage/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"username":"storage-user","password":"storage-pass"}`))
		case "/v1/storage/manifest":
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(manifestRaw)
		case "/v1/storage/read":
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(itemsRaw)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	acct := testAccount()
	acct.AccountEntropyPool = pool
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(acct); err != nil {
		t.Fatal(err)
	}
	ss := memstore.NewSignalStores()
	_ = ss

	c := &Client{
		acct:        acct,
		webc:        web.New(srv.URL, "test"),
		storageWebc: web.New(srv.URL, "test"),
	}

	result, err := c.SyncStorage(context.Background())
	if err != nil {
		t.Fatalf("SyncStorage: %v", err)
	}
	if len(result.Contacts) != 1 || result.Contacts[0].ACI == "" {
		t.Fatalf("contacts = %+v", result.Contacts)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].MasterKey) != 32 {
		t.Fatalf("groups = %+v", result.Groups)
	}
	if c.StorageManifestVersion() != 1 {
		t.Fatalf("version = %d", c.StorageManifestVersion())
	}
}

func TestSyncStorageRequiresAccountEntropyPool(t *testing.T) {
	acct := testAccount()
	c := &Client{
		acct:        acct,
		webc:        web.New("http://example.invalid", "test"),
		storageWebc: web.New("http://example.invalid", "test"),
	}
	_, err := c.SyncStorage(context.Background())
	if !errors.Is(err, ErrStorageNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateAccountEntropyPoolPersists(t *testing.T) {
	acct := testAccount()
	acctStore := memstore.New()
	if err := acctStore.SaveAccount(acct); err != nil {
		t.Fatal(err)
	}
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{acct: acct, accountStore: acctStore}
	c.updateAccountEntropyPool(pool)
	loaded, err := acctStore.LoadAccount()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccountEntropyPool != pool {
		t.Fatalf("pool not persisted: %q", loaded.AccountEntropyPool)
	}
}

func testIV(b byte) []byte {
	iv := make([]byte, 12)
	for i := range iv {
		iv[i] = b
	}
	return iv
}

func bytes32(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}
