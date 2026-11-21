package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	storagepb "github.com/thehappydinoa/signal-go/internal/proto/gen/storagepb"
	"github.com/thehappydinoa/signal-go/internal/storage"
	"github.com/thehappydinoa/signal-go/internal/store"
	"github.com/thehappydinoa/signal-go/internal/web"
)

func (f *storageFetcher) FetchLatestManifest(ctx context.Context) (*storagepb.StorageManifest, error) {
	if err := f.ensureStorageCreds(ctx); err != nil {
		return nil, err
	}
	f.credsMu.Lock()
	creds := f.storageCreds
	f.credsMu.Unlock()
	result, err := f.storageWebc.FetchStorageManifest(ctx, creds, 0)
	if err != nil {
		return nil, err
	}
	if result.Missing {
		return nil, storage.ErrManifestMissing
	}
	if result.Manifest == nil {
		return nil, errors.New("signal: storage manifest empty")
	}
	return result.Manifest, nil
}

func (f *storageFetcher) WriteRecords(ctx context.Context, op *storagepb.WriteOperation) (*storagepb.StorageManifest, error) {
	f.credsMu.Lock()
	creds := f.storageCreds
	f.credsMu.Unlock()
	if creds.Username == "" {
		if err := f.ensureStorageCreds(ctx); err != nil {
			return nil, err
		}
		f.credsMu.Lock()
		creds = f.storageCreds
		f.credsMu.Unlock()
	}
	manifest, err := f.storageWebc.WriteStorageRecords(ctx, creds, op)
	if err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (f *storageFetcher) ensureStorageCreds(ctx context.Context) error {
	f.credsMu.Lock()
	if f.storageCreds.Username != "" {
		f.credsMu.Unlock()
		return nil // reuse creds seeded from client cache
	}
	f.credsMu.Unlock()
	auth, err := f.webc.FetchStorageAuth(ctx, f.chatCreds)
	if err != nil {
		return err
	}
	f.credsMu.Lock()
	f.storageCreds = web.Credentials{Username: auth.Username, Password: auth.Password}
	f.credsMu.Unlock()
	return nil
}

func (c *Client) maybeWriteStorageContact(recipientACI string) {
	if c.disableSyncStorageWrites || c.acct == nil || recipientACI == "" || recipientACI == c.acct.ACI {
		return
	}
	key := "c:" + recipientACI
	c.storageMu.Lock()
	if c.storageWriteInFlight == nil {
		c.storageWriteInFlight = make(map[string]struct{})
	}
	_, inFlight := c.storageWriteInFlight[key]
	if !inFlight {
		c.storageWriteInFlight[key] = struct{}{}
	}
	c.storageMu.Unlock()
	if inFlight {
		return
	}
	go c.writeStorageContact(recipientACI)
}

func (c *Client) maybeWriteStorageGroup(masterKey []byte) {
	// c.acct must be non-nil: upsertStorageGroup reads
	// c.acct.AccountEntropyPool, and this runs on a background goroutine
	// where a nil dereference would take down the process rather than fail a
	// call. The caller in dispatch.go reaches this branch precisely when
	// c.acct is nil, so the guard cannot be left to the caller.
	if c.disableSyncStorageWrites || c.acct == nil || len(masterKey) == 0 {
		return
	}
	key := "g:" + hex.EncodeToString(masterKey)
	c.storageMu.Lock()
	if c.storageWriteInFlight == nil {
		c.storageWriteInFlight = make(map[string]struct{})
	}
	_, inFlight := c.storageWriteInFlight[key]
	if !inFlight {
		c.storageWriteInFlight[key] = struct{}{}
	}
	c.storageMu.Unlock()
	if inFlight {
		return
	}
	go c.writeStorageGroup(append([]byte(nil), masterKey...))
}

func (c *Client) writeStorageContact(recipientACI string) {
	defer func() {
		c.storageMu.Lock()
		delete(c.storageWriteInFlight, "c:"+recipientACI)
		c.storageMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), storageSyncTimeout)
	defer cancel()
	if err := c.upsertStorageContact(ctx, recipientACI); err != nil {
		if errors.Is(err, ErrStorageNotConfigured) || errors.Is(err, storage.ErrManifestMissing) {
			c.log.Debug("storage contact write skipped", "aci", recipientACI, "err", err)
			return
		}
		c.log.Warn("storage contact write failed", "aci", recipientACI, "err", err)
	}
}

func (c *Client) writeStorageGroup(masterKey []byte) {
	groupHex := hex.EncodeToString(masterKey)
	defer func() {
		c.storageMu.Lock()
		delete(c.storageWriteInFlight, "g:"+groupHex)
		c.storageMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), storageSyncTimeout)
	defer cancel()
	if err := c.upsertStorageGroup(ctx, masterKey); err != nil {
		if errors.Is(err, ErrStorageNotConfigured) || errors.Is(err, storage.ErrManifestMissing) {
			c.log.Debug("storage group write skipped", "group", groupHex, "err", err)
			return
		}
		c.log.Warn("storage group write failed", "group", groupHex, "err", err)
	}
}

func (c *Client) upsertStorageContact(ctx context.Context, recipientACI string) error {
	if c.webc == nil || c.storageWebc == nil {
		return errors.New("signal: Client was opened without REST clients")
	}
	if c.acct.AccountEntropyPool == "" {
		return ErrStorageNotConfigured
	}
	if err := c.ensureStorageManifestCached(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	profileKey := append([]byte(nil), c.knownProfileKeys[recipientACI]...)
	c.mu.Unlock()

	var storageID, e164, pni string
	c.storageMu.RLock()
	for _, sc := range c.storedContacts {
		if sc.ACI == recipientACI {
			storageID = sc.StorageID
			e164 = sc.E164
			pni = sc.PNI
			if len(profileKey) == 0 {
				profileKey = append([]byte(nil), sc.ProfileKey...)
			}
			break
		}
	}
	c.storageMu.RUnlock()

	// Generate an ID before the write so we can update the local cache
	// immediately on success, without a post-write SyncStorage round trip.
	if storageID == "" {
		var err error
		if storageID, err = storage.NewStorageID(); err != nil {
			return fmt.Errorf("signal: storage contact id: %w", err)
		}
	}

	var identityKey []byte
	if c.stores != nil {
		addr := store.Address{ServiceID: recipientACI, DeviceID: 1}
		if pub, err := c.stores.LoadIdentity(addr); err == nil {
			identityKey = append([]byte(nil), pub...)
		}
	}

	if err := c.pushStorageWrite(ctx, storage.WriteParams{
		Contacts: []storage.UpsertContactParams{{
			ACI:         recipientACI,
			E164:        e164,
			PNI:         pni,
			ProfileKey:  profileKey,
			IdentityKey: identityKey,
			StorageID:   storageID,
		}},
	}); err != nil {
		return err
	}

	c.storageMu.Lock()
	found := false
	for i := range c.storedContacts {
		if c.storedContacts[i].ACI == recipientACI {
			c.storedContacts[i].StorageID = storageID
			found = true
			break
		}
	}
	if !found {
		c.storedContacts = append(c.storedContacts, StoredContact{
			ACI: recipientACI, StorageID: storageID, E164: e164, PNI: pni,
			ProfileKey: append([]byte(nil), profileKey...),
		})
	}
	c.storageMu.Unlock()
	return nil
}

func (c *Client) upsertStorageGroup(ctx context.Context, masterKey []byte) error {
	if c.webc == nil || c.storageWebc == nil {
		return errors.New("signal: Client was opened without REST clients")
	}
	if c.acct.AccountEntropyPool == "" {
		return ErrStorageNotConfigured
	}
	if err := c.ensureStorageManifestCached(ctx); err != nil {
		return err
	}

	groupHex := hex.EncodeToString(masterKey)
	var storageID string
	c.storageMu.RLock()
	for _, sg := range c.storedGroups {
		if sg.ID == groupHex {
			storageID = sg.StorageID
			break
		}
	}
	c.storageMu.RUnlock()

	if storageID == "" {
		var err error
		if storageID, err = storage.NewStorageID(); err != nil {
			return fmt.Errorf("signal: storage group id: %w", err)
		}
	}

	if err := c.pushStorageWrite(ctx, storage.WriteParams{
		Groups: []storage.UpsertGroupParams{{
			MasterKey: append([]byte(nil), masterKey...),
			StorageID: storageID,
		}},
	}); err != nil {
		return err
	}

	c.storageMu.Lock()
	found := false
	for i := range c.storedGroups {
		if c.storedGroups[i].ID == groupHex {
			c.storedGroups[i].StorageID = storageID
			found = true
			break
		}
	}
	if !found {
		c.storedGroups = append(c.storedGroups, StoredGroup{
			ID:        groupHex,
			MasterKey: append([]byte(nil), masterKey...),
			StorageID: storageID,
		})
	}
	c.storageMu.Unlock()
	return nil
}

func (c *Client) ensureStorageManifestCached(ctx context.Context) error {
	c.storageMu.RLock()
	version := c.storageManifestVersion
	c.storageMu.RUnlock()
	if version > 0 {
		return nil
	}
	_, err := c.SyncStorage(ctx)
	return err
}

func (c *Client) pushStorageWrite(ctx context.Context, params storage.WriteParams) error {
	keys, err := storage.DeriveKeys(c.acct.AccountEntropyPool)
	if err != nil {
		return fmt.Errorf("signal: storage write: %w", err)
	}
	c.storageMu.RLock()
	cachedCreds := c.storageCredsCache
	c.storageMu.RUnlock()
	fetcher := &storageFetcher{
		webc:        c.webc,
		storageWebc: c.storageWebc,
		chatCreds: web.Credentials{
			Username: fmt.Sprintf("%s.%d", c.acct.ACI, c.acct.DeviceID),
			Password: c.acct.Password,
		},
		storageCreds: cachedCreds, // skips auth round trip if still valid
	}
	return c.pushStorageWriteWithRetry(ctx, keys, fetcher, params)
}

func (c *Client) pushStorageWriteWithRetry(
	ctx context.Context,
	keys storage.Keys,
	fetcher *storageFetcher,
	params storage.WriteParams,
) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			if _, err := c.SyncStorage(ctx); err != nil {
				return err
			}
		}
		manifest, err := fetcher.FetchLatestManifest(ctx)
		if err != nil {
			return err
		}
		snapshot, err := storage.DecryptManifest(keys, manifest)
		if err != nil {
			return err
		}
		updated, err := storage.Push(ctx, keys, snapshot, c.acct.DeviceID, params, fetcher)
		if err == nil {
			c.storageMu.Lock()
			c.storageManifestVersion = updated.Version
			c.storageMu.Unlock()
			return nil
		}
		lastErr = err
		var werr *web.Error
		if errors.As(err, &werr) {
			switch werr.StatusCode {
			case http.StatusConflict:
				continue
			case http.StatusUnauthorized, http.StatusForbidden:
				// Cached credentials expired — clear and re-fetch on next attempt.
				c.storageMu.Lock()
				c.storageCredsCache = web.Credentials{}
				c.storageMu.Unlock()
				fetcher.credsMu.Lock()
				fetcher.storageCreds = web.Credentials{}
				fetcher.credsMu.Unlock()
				continue
			}
		}
		return err
	}
	return lastErr
}
