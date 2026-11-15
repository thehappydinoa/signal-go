package storage

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

type fakeWriter struct {
	lastOp *storagepb.WriteOperation
	out    *storagepb.StorageManifest
	err    error
}

func (f *fakeWriter) WriteRecords(_ context.Context, op *storagepb.WriteOperation) (*storagepb.StorageManifest, error) {
	f.lastOp = op
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

func TestPushUpsertsContactAndGroup(t *testing.T) {
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatal(err)
	}
	keys, err := DeriveKeys(pool)
	if err != nil {
		t.Fatal(err)
	}

	accountID := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	snapshot := &ManifestSnapshot{
		Version: 3,
		Identifiers: []*storagepb.ManifestRecord_Identifier{
			{Raw: accountID, Type: storagepb.ManifestRecord_Identifier_ACCOUNT},
		},
	}

	masterKey := []byte("01234567890123456789012345678901")
	writer := &fakeWriter{}
	writer.out = nil // production storage service acks without manifest body

	updated, err := Push(context.Background(), keys, snapshot, 2, WriteParams{
		Contacts: []UpsertContactParams{{
			ACI: "00000000-0000-4000-8000-000000000099",
		}},
		Groups: []UpsertGroupParams{{
			MasterKey: masterKey,
		}},
	}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if writer.lastOp == nil || writer.lastOp.GetManifest().GetVersion() != 4 {
		t.Fatalf("write op = %+v", writer.lastOp)
	}
	if len(writer.lastOp.GetInsertItem()) != 2 {
		t.Fatalf("insert items = %d", len(writer.lastOp.GetInsertItem()))
	}
	if len(updated.Identifiers) != 3 {
		t.Fatalf("identifiers = %d, want 3", len(updated.Identifiers))
	}
	if updated.Version != 4 {
		t.Fatalf("version = %d, want 4", updated.Version)
	}

	snapshot = updated
	_, err = Push(context.Background(), keys, snapshot, 2, WriteParams{
		Contacts: []UpsertContactParams{{
			ACI:       "00000000-0000-4000-8000-000000000099",
			StorageID: "0102030405060708090a0b0c0d0e0f10",
		}},
	}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if len(writer.lastOp.GetInsertItem()) != 1 {
		t.Fatalf("update insert items = %d", len(writer.lastOp.GetInsertItem()))
	}
}

func TestDecryptManifest(t *testing.T) {
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatal(err)
	}
	keys, err := DeriveKeys(pool)
	if err != nil {
		t.Fatal(err)
	}
	manifestRecord := &storagepb.ManifestRecord{Version: 9, SourceDevice: 1}
	plain, err := proto.Marshal(manifestRecord)
	if err != nil {
		t.Fatal(err)
	}
	manifestKey := DeriveManifestKey(keys.StorageServiceKey, 9)
	cipher, err := EncryptRecordForTest(manifestKey, plain, makeIV(0xee))
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptManifest(keys, &storagepb.StorageManifest{Version: 9, Value: cipher})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 9 || got.SourceDevice != 1 {
		t.Fatalf("snapshot = %+v", got)
	}
}

func TestEncryptRecordRoundTrip(t *testing.T) {
	pool, err := libsignal.GenerateAccountEntropyPool()
	if err != nil {
		t.Fatal(err)
	}
	keys, err := DeriveKeys(pool)
	if err != nil {
		t.Fatal(err)
	}
	manifestKey := DeriveManifestKey(keys.StorageServiceKey, 1)
	plain := []byte("hello storage")
	cipher, err := EncryptRecord(manifestKey, plain)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecryptRecord(manifestKey, cipher)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Fatalf("round trip = %q", out)
	}
}
