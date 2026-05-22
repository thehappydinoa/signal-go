package signal

import (
	"context"
	"encoding/hex"
)

func (c *Client) cachedGroupRevision(masterKeyHex string) uint32 {
	c.groupRevMu.Lock()
	defer c.groupRevMu.Unlock()
	return c.groupRevision[masterKeyHex]
}

func (c *Client) storeGroupRevision(masterKeyHex string, revision uint32) {
	c.groupRevMu.Lock()
	if c.groupRevision == nil {
		c.groupRevision = make(map[string]uint32)
	}
	c.groupRevision[masterKeyHex] = revision
	c.groupRevMu.Unlock()
}

func (c *Client) deleteGroupRevision(masterKeyHex string) {
	c.groupRevMu.Lock()
	delete(c.groupRevision, masterKeyHex)
	c.groupRevMu.Unlock()
}

func (c *Client) maybeAutoSyncGroupUpdate(masterKey []byte, advertisedRevision uint32) {
	if !c.autoSyncGroupUpdates || c.storageWebc == nil {
		return
	}
	masterKeyHex := hex.EncodeToString(masterKey)
	from := c.cachedGroupRevision(masterKeyHex)
	if from == 0 && advertisedRevision > 0 {
		from = advertisedRevision - 1
	}
	go func() {
		ctx := context.Background()
		grp, err := c.SyncGroup(ctx, masterKey, from)
		if err != nil {
			c.log.Warn("auto group sync failed", "group", masterKeyHex, "err", err)
			return
		}
		c.storeGroupRevision(masterKeyHex, grp.Revision)
	}()
}
