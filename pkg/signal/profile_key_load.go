package signal

import "github.com/thehappydinoa/signal-go/internal/store"

func (c *Client) loadPersistedProfileKeys(s store.ProfileKeyStore) {
	if s == nil {
		return
	}
	keys, err := s.LoadProfileKeys()
	if err != nil {
		c.log.Warn("load persisted profile keys failed", "err", err)
		return
	}
	for aci, key := range keys {
		c.storeRecipientProfileKey(aci, key)
	}
}
