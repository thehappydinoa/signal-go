package signal

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
	"github.com/thehappydinoa/signal-go/internal/storage"
	"github.com/thehappydinoa/signal-go/internal/web"
)

const storageSyncTimeout = 2 * time.Minute

// ErrStorageNotConfigured is returned by [Client.SyncStorage] when the
// linked account has no AccountEntropyPool (storage sync requires it).
var ErrStorageNotConfigured = errors.New("signal: account has no AccountEntropyPool")

// StoredContact is a contact entry from the storage service manifest.
type StoredContact struct {
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

// StoredGroup is a Groups v2 list entry from the storage service manifest.
type StoredGroup struct {
	StorageID string
	// ID is the hex-encoded 32-byte group master key (matches [Group.ID]).
	ID        string
	MasterKey []byte
	Blocked   bool
	Archived  bool
}

// StorageSyncResult summarizes a pull-only storage sync.
type StorageSyncResult struct {
	Version   uint64
	Contacts  []StoredContact
	Groups    []StoredGroup
	Unchanged bool
}

// SyncStorage pulls the latest storage-service manifest and decrypts
// contact and Groups v2 list records. Profile keys from contacts are
// cached for sealed-sender send via [Client.SetRecipientProfileKey].
//
// Requires a non-empty AccountEntropyPool on the linked account. Pass
// [OpenOptions.AutoSyncStorage] to sync automatically when a linked
// device requests STORAGE_MANIFEST fetch-latest.
func (c *Client) SyncStorage(ctx context.Context) (*StorageSyncResult, error) {
	if c.webc == nil || c.storageWebc == nil {
		return nil, errors.New("signal.SyncStorage: Client was opened without REST clients")
	}
	if c.acct.AccountEntropyPool == "" {
		return nil, ErrStorageNotConfigured
	}
	keys, err := storage.DeriveKeys(c.acct.AccountEntropyPool)
	if err != nil {
		return nil, fmt.Errorf("signal.SyncStorage: %w", err)
	}

	c.storageMu.RLock()
	localVersion := c.storageManifestVersion
	c.storageMu.RUnlock()

	fetcher := &storageFetcher{
		webc:        c.webc,
		storageWebc: c.storageWebc,
		chatCreds: web.Credentials{
			Username: fmt.Sprintf("%s.%d", c.acct.ACI, c.acct.DeviceID),
			Password: c.acct.Password,
		},
	}
	pulled, err := storage.Pull(ctx, keys, fetcher, localVersion)
	if err != nil {
		if errors.Is(err, storage.ErrManifestMissing) {
			return &StorageSyncResult{Version: localVersion}, nil
		}
		return nil, fmt.Errorf("signal.SyncStorage: %w", err)
	}
	result := &StorageSyncResult{
		Version:   pulled.Version,
		Unchanged: pulled.Unchanged,
	}
	for _, contact := range pulled.Contacts {
		sc := storedContactFromInternal(contact)
		result.Contacts = append(result.Contacts, sc)
		if sc.ACI != "" && len(sc.ProfileKey) == libsignal.ProfileKeyLen {
			c.SetRecipientProfileKey(sc.ACI, sc.ProfileKey)
		}
	}
	for _, group := range pulled.Groups {
		result.Groups = append(result.Groups, storedGroupFromInternal(group))
	}

	if !pulled.Unchanged {
		c.storageMu.Lock()
		c.storageManifestVersion = pulled.Version
		c.storedContacts = append([]StoredContact(nil), result.Contacts...)
		c.storedGroups = append([]StoredGroup(nil), result.Groups...)
		c.storageMu.Unlock()
	}
	return result, nil
}

// StoredContacts returns the contact list from the last successful
// [Client.SyncStorage] call.
func (c *Client) StoredContacts() []StoredContact {
	c.storageMu.RLock()
	defer c.storageMu.RUnlock()
	if len(c.storedContacts) == 0 {
		return nil
	}
	out := make([]StoredContact, len(c.storedContacts))
	copy(out, c.storedContacts)
	return out
}

// StoredGroups returns the Groups v2 list from the last successful
// [Client.SyncStorage] call.
func (c *Client) StoredGroups() []StoredGroup {
	c.storageMu.RLock()
	defer c.storageMu.RUnlock()
	if len(c.storedGroups) == 0 {
		return nil
	}
	out := make([]StoredGroup, len(c.storedGroups))
	copy(out, c.storedGroups)
	return out
}

// StorageManifestVersion returns the manifest version from the last
// successful sync (0 before the first sync).
func (c *Client) StorageManifestVersion() uint64 {
	c.storageMu.RLock()
	defer c.storageMu.RUnlock()
	return c.storageManifestVersion
}

func (c *Client) maybeAutoSyncStorage() {
	if !c.autoSyncStorage {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), storageSyncTimeout)
		defer cancel()
		if _, err := c.SyncStorage(ctx); err != nil && !errors.Is(err, ErrStorageNotConfigured) {
			c.log.Warn("auto storage sync failed", "err", err)
		}
	}()
}

func (c *Client) updateAccountEntropyPool(pool string) {
	if pool == "" || pool == c.acct.AccountEntropyPool {
		return
	}
	c.acct.AccountEntropyPool = pool
	if c.accountStore == nil {
		return
	}
	if err := c.accountStore.SaveAccount(c.acct); err != nil {
		c.log.Warn("persist account entropy pool failed", "err", err)
	}
}

type storageFetcher struct {
	webc         *web.Client
	storageWebc  *web.Client
	chatCreds    web.Credentials
	storageCreds web.Credentials
	credsMu      sync.Mutex
}

func (f *storageFetcher) FetchManifest(ctx context.Context, localVersion uint64) (*storagepb.StorageManifest, bool, error) {
	auth, err := f.webc.FetchStorageAuth(ctx, f.chatCreds)
	if err != nil {
		return nil, false, err
	}
	f.credsMu.Lock()
	f.storageCreds = web.Credentials{Username: auth.Username, Password: auth.Password}
	f.credsMu.Unlock()

	result, err := f.storageWebc.FetchStorageManifest(ctx, f.storageCreds, localVersion)
	if err != nil {
		return nil, false, err
	}
	if result.Unchanged {
		return nil, true, nil
	}
	if result.Missing {
		return nil, false, storage.ErrManifestMissing
	}
	return result.Manifest, false, nil
}

func (f *storageFetcher) ReadRecords(ctx context.Context, keys [][]byte) ([]*storagepb.StorageItem, error) {
	f.credsMu.Lock()
	creds := f.storageCreds
	f.credsMu.Unlock()
	items, err := f.storageWebc.ReadStorageRecords(ctx, creds, keys)
	if err != nil {
		return nil, err
	}
	return items.GetItems(), nil
}

func storedContactFromInternal(c storage.Contact) StoredContact {
	return StoredContact{
		StorageID:  c.StorageID,
		ACI:        c.ACI,
		E164:       c.E164,
		PNI:        c.PNI,
		ProfileKey: append([]byte(nil), c.ProfileKey...),
		GivenName:  c.GivenName,
		FamilyName: c.FamilyName,
		Blocked:    c.Blocked,
		Archived:   c.Archived,
	}
}

func storedGroupFromInternal(g storage.Group) StoredGroup {
	return StoredGroup{
		StorageID: g.StorageID,
		ID:        g.ID,
		MasterKey: append([]byte(nil), g.MasterKey...),
		Blocked:   g.Blocked,
		Archived:  g.Archived,
	}
}
