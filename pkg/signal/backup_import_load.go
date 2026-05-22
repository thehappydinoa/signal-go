package signal

import (
	"encoding/hex"

	"github.com/thehappydinoa/signal-go/internal/libsignal"
	"github.com/thehappydinoa/signal-go/internal/store"
)

func (c *Client) loadImportedBackupData(s store.BackupImportStore) {
	if s == nil {
		return
	}
	contacts, err := s.LoadImportedContacts()
	if err != nil {
		c.log.Warn("load imported contacts failed", "err", err)
		return
	}
	groups, err := s.LoadImportedGroups()
	if err != nil {
		c.log.Warn("load imported groups failed", "err", err)
		return
	}
	if len(contacts) == 0 && len(groups) == 0 {
		return
	}

	storedContacts := make([]StoredContact, 0, len(contacts))
	for _, ct := range contacts {
		storedContacts = append(storedContacts, StoredContact{
			ACI:        ct.ACI,
			PNI:        ct.PNI,
			E164:       ct.E164,
			ProfileKey: append([]byte(nil), ct.ProfileKey...),
			GivenName:  ct.GivenName,
			FamilyName: ct.FamilyName,
			Blocked:    ct.Blocked,
		})
		if ct.ACI != "" && len(ct.ProfileKey) == libsignal.ProfileKeyLen {
			c.SetRecipientProfileKey(ct.ACI, ct.ProfileKey)
		}
	}
	storedGroups := make([]StoredGroup, 0, len(groups))
	for _, g := range groups {
		storedGroups = append(storedGroups, StoredGroup{
			ID:        hex.EncodeToString(g.MasterKey),
			MasterKey: append([]byte(nil), g.MasterKey...),
			Blocked:   g.Blocked,
		})
	}

	c.storageMu.Lock()
	c.storedContacts = storedContacts
	c.storedGroups = storedGroups
	c.storageMu.Unlock()
}
