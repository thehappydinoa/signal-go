package backup

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	backuppbg "github.com/thehappydinoa/signal-go/internal/proto/gen/backuppbg"
	"github.com/thehappydinoa/signal-go/internal/store"
)

// ImportPurpose selects the libsignal backup validation purpose.
type ImportPurpose = libsignal.BackupPurpose

// ImportTarget receives imported backup data.
type ImportTarget struct {
	Identities   store.IdentityStore
	BackupImport store.BackupImportStore
	// OnChatItem receives the protobuf encoding of each signal.backup.ChatItem
	// frame. Optional.
	OnChatItem func(serializedChatItem []byte) error
}

// ImportStats summarizes a successful import.
type ImportStats struct {
	ContactsImported   int
	GroupsImported     int
	IdentitiesImported int
	FramesProcessed    int
	ChatItemsProcessed int
}

// ImportArchive decrypts ciphertext, validates frames, and writes importable
// data into target.
func ImportArchive(
	key *libsignal.MessageBackupKey,
	ciphertext []byte,
	purpose ImportPurpose,
	target ImportTarget,
) (*ImportStats, error) {
	if key == nil {
		return nil, errors.New("backup.ImportArchive: nil key")
	}
	if len(ciphertext) == 0 {
		return nil, errors.New("backup.ImportArchive: empty ciphertext")
	}
	aesKey, err := key.AesKey()
	if err != nil {
		return nil, fmt.Errorf("backup.ImportArchive: aes key: %w", err)
	}
	hmacKey, err := key.HmacKey()
	if err != nil {
		return nil, fmt.Errorf("backup.ImportArchive: hmac key: %w", err)
	}
	plain, err := DecryptArchive(ciphertext, aesKey, hmacKey)
	if err != nil {
		return nil, fmt.Errorf("backup.ImportArchive: decrypt: %w", err)
	}
	return importPlaintext(plain, purpose, target)
}

func importPlaintext(plain []byte, purpose ImportPurpose, target ImportTarget) (*ImportStats, error) {
	stats := &ImportStats{}
	r := bytes.NewReader(plain)

	first, err := ReadVarintFrame(r)
	if err != nil {
		return nil, fmt.Errorf("backup.ImportArchive: backup info: %w", err)
	}
	validator, err := libsignal.NewOnlineBackupValidator(first, purpose)
	if err != nil {
		return nil, fmt.Errorf("backup.ImportArchive: validator: %w", err)
	}
	defer validator.Close()

	for {
		frameBytes, err := ReadVarintFrame(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("backup.ImportArchive: read frame: %w", err)
		}
		if len(frameBytes) == 0 {
			continue
		}
		if err := validator.AddFrame(frameBytes); err != nil {
			return nil, fmt.Errorf("backup.ImportArchive: validate frame: %w", err)
		}
		stats.FramesProcessed++
		if err := importFrame(frameBytes, target, stats); err != nil {
			return nil, err
		}
	}
	if err := validator.Finalize(); err != nil {
		return nil, fmt.Errorf("backup.ImportArchive: finalize: %w", err)
	}
	return stats, nil
}

func importFrame(frameBytes []byte, target ImportTarget, stats *ImportStats) error {
	var frame backuppbg.Frame
	if err := proto.Unmarshal(frameBytes, &frame); err != nil {
		return fmt.Errorf("backup.ImportArchive: unmarshal frame: %w", err)
	}
	if ci := frame.GetChatItem(); ci != nil {
		if target.OnChatItem != nil {
			itemWire, err := proto.Marshal(ci)
			if err != nil {
				return fmt.Errorf("backup.ImportArchive: marshal chat item: %w", err)
			}
			if err := target.OnChatItem(itemWire); err != nil {
				return fmt.Errorf("backup.ImportArchive: on chat item: %w", err)
			}
		}
		stats.ChatItemsProcessed++
		return nil
	}
	recipient := frame.GetRecipient()
	if recipient == nil {
		return nil
	}
	switch dest := recipient.GetDestination().(type) {
	case *backuppbg.Recipient_Contact:
		return importContact(dest.Contact, target, stats)
	case *backuppbg.Recipient_Group:
		return importGroup(dest.Group, target, stats)
	default:
		return nil
	}
}

func importContact(c *backuppbg.Contact, target ImportTarget, stats *ImportStats) error {
	if c == nil {
		return nil
	}
	aci := rawUUIDString(c.GetAci())
	pni := rawUUIDString(c.GetPni())
	e164 := ""
	if n := c.GetE164(); n != 0 {
		e164 = strconv.FormatUint(n, 10)
	}
	if aci == "" && e164 == "" && pni == "" {
		return nil
	}

	if target.BackupImport != nil && aci != "" {
		entry := store.ImportedContact{
			ACI:        aci,
			PNI:        pni,
			E164:       e164,
			ProfileKey: append([]byte(nil), c.GetProfileKey()...),
			GivenName:  firstNonEmpty(c.GetProfileGivenName(), nicknameField(c.GetNickname(), true)),
			FamilyName: firstNonEmpty(c.GetProfileFamilyName(), nicknameField(c.GetNickname(), false)),
			Blocked:    c.GetBlocked(),
		}
		if err := target.BackupImport.SaveImportedContact(entry); err != nil {
			return fmt.Errorf("backup.ImportArchive: save contact: %w", err)
		}
		stats.ContactsImported++
	}

	if target.Identities != nil && aci != "" && len(c.GetIdentityKey()) > 0 {
		pub, err := libsignal.DeserializePublicKey(c.GetIdentityKey())
		if err != nil {
			return fmt.Errorf("backup.ImportArchive: contact identity key: %w", err)
		}
		pubBytes, err := pub.Serialize()
		if err != nil {
			return fmt.Errorf("backup.ImportArchive: serialize identity key: %w", err)
		}
		addr := store.Address{ServiceID: aci, DeviceID: 1}
		if _, err := target.Identities.SaveIdentity(addr, pubBytes); err != nil {
			return fmt.Errorf("backup.ImportArchive: save identity: %w", err)
		}
		stats.IdentitiesImported++
	}
	return nil
}

func importGroup(g *backuppbg.Group, target ImportTarget, stats *ImportStats) error {
	if g == nil || target.BackupImport == nil {
		return nil
	}
	masterKey := append([]byte(nil), g.GetMasterKey()...)
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return nil
	}
	title := ""
	if snap := g.GetSnapshot(); snap != nil {
		if t := snap.GetTitle(); t != nil {
			title = t.GetTitle()
		}
	}
	entry := store.ImportedGroup{
		MasterKey: masterKey,
		Title:     title,
		Blocked:   g.GetBlocked(),
	}
	if err := target.BackupImport.SaveImportedGroup(entry); err != nil {
		return fmt.Errorf("backup.ImportArchive: save group: %w", err)
	}
	stats.GroupsImported++
	return nil
}

func rawUUIDString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	var raw [16]byte
	copy(raw[:], b)
	return libsignal.FormatRawUUID(raw)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func nicknameField(n *backuppbg.Contact_Name, given bool) string {
	if n == nil {
		return ""
	}
	if given {
		return n.GetGiven()
	}
	return n.GetFamily()
}

// MasterKeyHex returns the hex-encoded group master key ID.
func MasterKeyHex(masterKey []byte) string {
	return hex.EncodeToString(masterKey)
}
