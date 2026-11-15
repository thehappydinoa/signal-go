package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
)

const storageRecordKeyLen = 16

// NewStorageID returns a fresh random hex-encoded 16-byte storage record key.
func NewStorageID() (string, error) {
	key := make([]byte, storageRecordKeyLen)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("storage.NewStorageID: %w", err)
	}
	return hex.EncodeToString(key), nil
}

// UpsertContactParams updates one contact (DM chat list) entry.
type UpsertContactParams struct {
	ACI         string
	E164        string
	PNI         string
	ProfileKey  []byte
	IdentityKey []byte
	StorageID   string // hex-encoded 16-byte key; empty allocates a new key
}

// UpsertGroupParams updates one Groups v2 chat list entry.
type UpsertGroupParams struct {
	MasterKey []byte
	StorageID string // hex-encoded 16-byte key; empty allocates a new key
}

// WriteParams lists storage records to upsert into the manifest.
type WriteParams struct {
	Contacts []UpsertContactParams
	Groups   []UpsertGroupParams
}

// StorageWriter uploads a [storagepb.WriteOperation].
type StorageWriter interface {
	WriteRecords(ctx context.Context, op *storagepb.WriteOperation) (*storagepb.StorageManifest, error)
}

// Push merges upserts into the current manifest and uploads them. snapshot
// must be the decrypted manifest currently on the server. sourceDevice is the
// linked device id recorded in the new manifest.
func Push(
	ctx context.Context,
	keys Keys,
	snapshot *ManifestSnapshot,
	sourceDevice uint32,
	params WriteParams,
	writer StorageWriter,
) (*ManifestSnapshot, error) {
	if snapshot == nil {
		return nil, errors.New("storage.Push: nil snapshot")
	}
	if writer == nil {
		return nil, errors.New("storage.Push: nil writer")
	}
	if len(params.Contacts) == 0 && len(params.Groups) == 0 {
		return snapshot, nil
	}

	newVersion := snapshot.Version + 1
	identifiers := cloneIdentifiers(snapshot.Identifiers)
	items := make([]*storagepb.StorageItem, 0, len(params.Contacts)+len(params.Groups))

	for _, contact := range params.Contacts {
		if contact.ACI == "" {
			return nil, errors.New("storage.Push: empty contact ACI")
		}
		key, isNew, err := resolveStorageKey(contact.StorageID, identifiers, storagepb.ManifestRecord_Identifier_CONTACT)
		if err != nil {
			return nil, err
		}
		record, err := buildContactRecord(contact)
		if err != nil {
			return nil, err
		}
		item, err := encryptStorageRecord(keys, snapshot.RecordIkm, key, record)
		if err != nil {
			return nil, fmt.Errorf("storage.Push: contact %s: %w", contact.ACI, err)
		}
		items = append(items, item)
		if isNew {
			identifiers = appendIdentifier(identifiers, key, storagepb.ManifestRecord_Identifier_CONTACT)
		}
	}

	for _, group := range params.Groups {
		if len(group.MasterKey) != libsignal.GroupMasterKeyLen {
			return nil, fmt.Errorf("storage.Push: group master key length %d, want %d", len(group.MasterKey), libsignal.GroupMasterKeyLen)
		}
		key, isNew, err := resolveStorageKey(group.StorageID, identifiers, storagepb.ManifestRecord_Identifier_GROUPV2)
		if err != nil {
			return nil, err
		}
		record := &storagepb.StorageRecord{
			Record: &storagepb.StorageRecord_GroupV2{
				GroupV2: &storagepb.GroupV2Record{
					MasterKey: append([]byte(nil), group.MasterKey...),
				},
			},
		}
		item, err := encryptStorageRecord(keys, snapshot.RecordIkm, key, record)
		if err != nil {
			return nil, fmt.Errorf("storage.Push: group %x: %w", group.MasterKey[:4], err)
		}
		items = append(items, item)
		if isNew {
			identifiers = appendIdentifier(identifiers, key, storagepb.ManifestRecord_Identifier_GROUPV2)
		}
	}

	manifestRecord := &storagepb.ManifestRecord{
		Version:      newVersion,
		SourceDevice: sourceDevice,
		RecordIkm:    append([]byte(nil), snapshot.RecordIkm...),
		Identifiers:  identifiers,
	}
	manifestPlain, err := proto.Marshal(manifestRecord)
	if err != nil {
		return nil, fmt.Errorf("storage.Push: marshal manifest: %w", err)
	}
	manifestKey := DeriveManifestKey(keys.StorageServiceKey, newVersion)
	manifestCipher, err := EncryptRecord(manifestKey, manifestPlain)
	if err != nil {
		return nil, fmt.Errorf("storage.Push: encrypt manifest: %w", err)
	}
	op := &storagepb.WriteOperation{
		Manifest: &storagepb.StorageManifest{
			Version: newVersion,
			Value:   manifestCipher,
		},
		InsertItem: items,
	}
	if _, err := writer.WriteRecords(ctx, op); err != nil {
		return nil, fmt.Errorf("storage.Push: %w", err)
	}
	// Successful PUT /v1/storage/ acks without returning encrypted manifest bytes
	// (Signal Desktop ignores the response body). The snapshot we just committed
	// is authoritative until the next pull.
	return &ManifestSnapshot{
		Version:      newVersion,
		SourceDevice: sourceDevice,
		RecordIkm:    append([]byte(nil), snapshot.RecordIkm...),
		Identifiers:  identifiers,
	}, nil
}

func buildContactRecord(p UpsertContactParams) (*storagepb.StorageRecord, error) {
	aciBinary, err := aciBinaryBytes(p.ACI)
	if err != nil {
		return nil, err
	}
	contact := &storagepb.ContactRecord{
		Aci:       p.ACI,
		AciBinary: aciBinary,
		E164:      p.E164,
		Pni:       p.PNI,
		Archived:  false,
		Hidden:    false,
		Blocked:   false,
	}
	if len(p.ProfileKey) == libsignal.ProfileKeyLen {
		contact.ProfileKey = append([]byte(nil), p.ProfileKey...)
	}
	if len(p.IdentityKey) > 0 {
		contact.IdentityKey = append([]byte(nil), p.IdentityKey...)
	}
	return &storagepb.StorageRecord{
		Record: &storagepb.StorageRecord_Contact{Contact: contact},
	}, nil
}

func aciBinaryBytes(aci string) ([]byte, error) {
	uuid, err := libsignal.ParseUUIDString(aci)
	if err != nil {
		return nil, fmt.Errorf("storage.Push: aci: %w", err)
	}
	return uuid[:], nil
}

func encryptStorageRecord(
	keys Keys,
	recordIkm, storageKey []byte,
	record *storagepb.StorageRecord,
) (*storagepb.StorageItem, error) {
	plain, err := proto.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal record: %w", err)
	}
	itemKey, err := DeriveItemKey(keys.StorageServiceKey, recordIkm, storageKey)
	if err != nil {
		return nil, err
	}
	cipher, err := EncryptRecord(itemKey, plain)
	if err != nil {
		return nil, err
	}
	return &storagepb.StorageItem{
		Key:   append([]byte(nil), storageKey...),
		Value: cipher,
	}, nil
}

func resolveStorageKey(
	storageIDHex string,
	identifiers []*storagepb.ManifestRecord_Identifier,
	typ storagepb.ManifestRecord_Identifier_Type,
) (key []byte, isNew bool, err error) {
	if storageIDHex != "" {
		raw, err := hex.DecodeString(storageIDHex)
		if err != nil {
			return nil, false, fmt.Errorf("storage.Push: decode storage id: %w", err)
		}
		if len(raw) != storageRecordKeyLen {
			return nil, false, fmt.Errorf("storage.Push: storage id length %d, want %d", len(raw), storageRecordKeyLen)
		}
		return raw, false, nil
	}
	key = make([]byte, storageRecordKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, false, fmt.Errorf("storage.Push: random storage id: %w", err)
	}
	_ = identifiers
	_ = typ
	return key, true, nil
}

func cloneIdentifiers(in []*storagepb.ManifestRecord_Identifier) []*storagepb.ManifestRecord_Identifier {
	out := make([]*storagepb.ManifestRecord_Identifier, 0, len(in))
	for _, id := range in {
		if id == nil || len(id.GetRaw()) == 0 {
			continue
		}
		out = append(out, &storagepb.ManifestRecord_Identifier{
			Raw:  append([]byte(nil), id.GetRaw()...),
			Type: id.GetType(),
		})
	}
	return out
}

func appendIdentifier(
	identifiers []*storagepb.ManifestRecord_Identifier,
	raw []byte,
	typ storagepb.ManifestRecord_Identifier_Type,
) []*storagepb.ManifestRecord_Identifier {
	keyHex := hex.EncodeToString(raw)
	for _, id := range identifiers {
		if id.GetType() == typ && hex.EncodeToString(id.GetRaw()) == keyHex {
			return identifiers
		}
	}
	return append(identifiers, &storagepb.ManifestRecord_Identifier{
		Raw:  append([]byte(nil), raw...),
		Type: typ,
	})
}
