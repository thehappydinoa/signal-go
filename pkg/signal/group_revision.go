package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/thehappydinoa/signal-go/internal/web"
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
		var grp *Group
		var err error

		if from == 0 {
			// No cached revision: fetch current state directly rather than
			// requesting logs from revision 0, which may be below the server's
			// retention window and return 403.
			grp, err = c.FetchGroup(ctx, masterKey)
		} else {
			grp, err = c.SyncGroup(ctx, masterKey, from)
			if err != nil {
				var webErr *web.Error
				if errors.As(err, &webErr) && webErr.StatusCode == http.StatusForbidden {
					// Logs endpoint 403 can mean the cached revision is stale
					// (pruned by the server). Reset and fall back to FetchGroup.
					c.deleteGroupRevision(masterKeyHex)
					grp, err = c.FetchGroup(ctx, masterKey)
				}
			}
		}

		if err != nil {
			var webErr *web.Error
			if errors.As(err, &webErr) && webErr.StatusCode == http.StatusForbidden {
				c.log.Debug("auto group sync skipped: not a member", "group", masterKeyHex)
				return
			}
			c.log.Warn("auto group sync failed", "group", masterKeyHex, "err", err)
			return
		}
		c.storeGroupRevision(masterKeyHex, grp.Revision)
	}()
}
