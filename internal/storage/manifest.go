package storage

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

// ManifestSnapshot is a decrypted storage-service manifest.
type ManifestSnapshot struct {
	Version      uint64
	SourceDevice uint32
	RecordIkm    []byte
	Identifiers  []*storagepb.ManifestRecord_Identifier
}

// DecryptManifest decrypts a [storagepb.StorageManifest] blob.
func DecryptManifest(keys Keys, manifest *storagepb.StorageManifest) (*ManifestSnapshot, error) {
	if manifest == nil {
		return nil, fmt.Errorf("storage.DecryptManifest: nil manifest")
	}
	manifestKey := DeriveManifestKey(keys.StorageServiceKey, manifest.GetVersion())
	plain, err := DecryptRecord(manifestKey, manifest.GetValue())
	if err != nil {
		return nil, fmt.Errorf("storage.DecryptManifest: %w", err)
	}
	var record storagepb.ManifestRecord
	if err := proto.Unmarshal(plain, &record); err != nil {
		return nil, fmt.Errorf("storage.DecryptManifest: unmarshal: %w", err)
	}
	ids := make([]*storagepb.ManifestRecord_Identifier, 0, len(record.GetIdentifiers()))
	for _, id := range record.GetIdentifiers() {
		if id == nil || len(id.GetRaw()) == 0 {
			continue
		}
		ids = append(ids, &storagepb.ManifestRecord_Identifier{
			Raw:  append([]byte(nil), id.GetRaw()...),
			Type: id.GetType(),
		})
	}
	return &ManifestSnapshot{
		Version:      manifest.GetVersion(),
		SourceDevice: record.GetSourceDevice(),
		RecordIkm:    append([]byte(nil), record.GetRecordIkm()...),
		Identifiers:  ids,
	}, nil
}
