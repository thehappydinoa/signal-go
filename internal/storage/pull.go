package storage

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

// Contact is a decrypted storage-service contact record.
type Contact struct {
	StorageID  string
	ACI        string
	E164       string
	PNI        string
	ProfileKey []byte
	GivenName  string
	FamilyName string
	Blocked    bool
	Archived   bool
}

// Group is a decrypted storage-service Groups v2 list entry.
type Group struct {
	StorageID string
	MasterKey []byte
	ID        string // hex-encoded master key
	Blocked   bool
	Archived  bool
}

// PullResult is the outcome of a pull-only storage sync.
type PullResult struct {
	Version   uint64
	Contacts  []Contact
	Groups    []Group
	Unchanged bool
}

// ManifestFetcher loads and decrypts storage manifests and records.
type ManifestFetcher interface {
	FetchManifest(ctx context.Context, localVersion uint64) (*storagepb.StorageManifest, bool, error)
	ReadRecords(ctx context.Context, keys [][]byte) ([]*storagepb.StorageItem, error)
}

// Pull downloads contacts and Groups v2 list entries from the storage
// service. localVersion is the last synced manifest version; pass 0 on
// first sync. Returns PullResult.Unchanged=true when the remote manifest
// has not advanced.
func Pull(ctx context.Context, keys Keys, fetch ManifestFetcher, localVersion uint64) (*PullResult, error) {
	manifest, unchanged, err := fetch.FetchManifest(ctx, localVersion)
	if err != nil {
		return nil, fmt.Errorf("storage.Pull: %w", err)
	}
	if unchanged {
		return &PullResult{Version: localVersion, Unchanged: true}, nil
	}
	if manifest == nil {
		return &PullResult{Version: localVersion}, nil
	}

	manifestKey := DeriveManifestKey(keys.StorageServiceKey, manifest.GetVersion())
	plainManifest, err := DecryptRecord(manifestKey, manifest.GetValue())
	if err != nil {
		return nil, fmt.Errorf("storage.Pull: decrypt manifest: %w", err)
	}
	var manifestRecord storagepb.ManifestRecord
	if err := proto.Unmarshal(plainManifest, &manifestRecord); err != nil {
		return nil, fmt.Errorf("storage.Pull: unmarshal manifest: %w", err)
	}

	recordIkm := manifestRecord.GetRecordIkm()
	var readKeys [][]byte
	idTypes := make(map[string]storagepb.ManifestRecord_Identifier_Type)
	for _, id := range manifestRecord.GetIdentifiers() {
		raw := append([]byte(nil), id.GetRaw()...)
		if len(raw) == 0 {
			continue
		}
		switch id.GetType() {
		case storagepb.ManifestRecord_Identifier_CONTACT,
			storagepb.ManifestRecord_Identifier_GROUPV2:
			readKeys = append(readKeys, raw)
			idTypes[hex.EncodeToString(raw)] = id.GetType()
		}
	}

	if len(readKeys) == 0 {
		return &PullResult{
			Version: manifest.GetVersion(),
		}, nil
	}

	items, err := fetch.ReadRecords(ctx, readKeys)
	if err != nil {
		return nil, fmt.Errorf("storage.Pull: read records: %w", err)
	}

	result := &PullResult{Version: manifest.GetVersion()}
	for _, item := range items {
		keyHex := hex.EncodeToString(item.GetKey())
		itemType, ok := idTypes[keyHex]
		if !ok {
			continue
		}
		itemKey, err := DeriveItemKey(keys.StorageServiceKey, recordIkm, item.GetKey())
		if err != nil {
			return nil, fmt.Errorf("storage.Pull: item key: %w", err)
		}
		plainRecord, err := DecryptRecord(itemKey, item.GetValue())
		if err != nil {
			return nil, fmt.Errorf("storage.Pull: decrypt record %s: %w", keyHex, err)
		}
		var record storagepb.StorageRecord
		if err := proto.Unmarshal(plainRecord, &record); err != nil {
			return nil, fmt.Errorf("storage.Pull: unmarshal record: %w", err)
		}
		switch itemType {
		case storagepb.ManifestRecord_Identifier_CONTACT:
			if c := record.GetContact(); c != nil {
				result.Contacts = append(result.Contacts, contactFromProto(keyHex, c))
			}
		case storagepb.ManifestRecord_Identifier_GROUPV2:
			if g := record.GetGroupV2(); g != nil {
				result.Groups = append(result.Groups, groupFromProto(keyHex, g))
			}
		}
	}
	return result, nil
}

func contactFromProto(storageID string, c *storagepb.ContactRecord) Contact {
	out := Contact{
		StorageID:  storageID,
		ACI:        c.GetAci(),
		E164:       c.GetE164(),
		PNI:        c.GetPni(),
		ProfileKey: append([]byte(nil), c.GetProfileKey()...),
		GivenName:  c.GetGivenName(),
		FamilyName: c.GetFamilyName(),
		Blocked:    c.GetBlocked(),
		Archived:   c.GetArchived(),
	}
	if out.ACI == "" && len(c.GetAciBinary()) == 16 {
		out.ACI = uuidFromBytes(c.GetAciBinary())
	}
	if out.PNI == "" && len(c.GetPniBinary()) == 16 {
		out.PNI = uuidFromBytes(c.GetPniBinary())
	}
	if nn := c.GetNickname(); nn != nil {
		if out.GivenName == "" {
			out.GivenName = nn.GetGiven()
		}
		if out.FamilyName == "" {
			out.FamilyName = nn.GetFamily()
		}
	}
	return out
}

func groupFromProto(storageID string, g *storagepb.GroupV2Record) Group {
	mk := append([]byte(nil), g.GetMasterKey()...)
	return Group{
		StorageID: storageID,
		MasterKey: mk,
		ID:        hex.EncodeToString(mk),
		Blocked:   g.GetBlocked(),
		Archived:  g.GetArchived(),
	}
}

func uuidFromBytes(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3]),
		uint16(b[4])<<8|uint16(b[5]),
		uint16(b[6])<<8|uint16(b[7]),
		uint16(b[8])<<8|uint16(b[9]),
		uint64(b[10])<<40|uint64(b[11])<<32|uint64(b[12])<<24|
			uint64(b[13])<<16|uint64(b[14])<<8|uint64(b[15]),
	)
}

// ErrManifestMissing is returned when the account has no remote manifest yet.
var ErrManifestMissing = errors.New("storage: manifest missing")
