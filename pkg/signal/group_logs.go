package signal

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/proto"

	"github.com/thehappydinoa/signal-go/internal/group"
	"github.com/thehappydinoa/signal-go/internal/libsignal"
	groupspb "github.com/thehappydinoa/signal-go/internal/proto/gen/groupspb"
	"github.com/thehappydinoa/signal-go/internal/web"
)

// GroupLogsFetchOptions configures a single [FetchGroupLogs] page.
type GroupLogsFetchOptions struct {
	FromRevision                     uint32
	CachedSendEndorsementsExpiration int64
	Limit                            int
	MaxSupportedChangeEpoch          uint32
	IncludeFirstState                bool
	IncludeLastState                 bool
}

// GroupLogsPage is one decoded page of group change history.
type GroupLogsPage struct {
	Changes      *groupspb.GroupChanges
	Partial      bool
	ContentRange string
	NextRevision uint32
}

// FetchGroupLogs retrieves one page of group change logs starting at
// opts.FromRevision. Requires membership in the group.
func (c *Client) FetchGroupLogs(ctx context.Context, masterKey []byte, opts GroupLogsFetchOptions) (*GroupLogsPage, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return nil, fmt.Errorf("signal.FetchGroupLogs: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	if c.storageWebc == nil {
		return nil, errors.New("signal.FetchGroupLogs: Client was opened without groups storage")
	}
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}
	publicParams, err := libsignal.GroupSecretParamsPublicParams(secretParams)
	if err != nil {
		return nil, err
	}
	authHeader, err := c.groupsV2AuthHeader(ctx, secretParams, publicParams)
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroupLogs: authorize: %w", err)
	}

	page, err := c.storageWebc.FetchGroupLogs(ctx, authHeader, opts.FromRevision, web.GroupLogsOptions{
		CachedSendEndorsementsExpiration: opts.CachedSendEndorsementsExpiration,
		Limit:                            opts.Limit,
		MaxSupportedChangeEpoch:          opts.MaxSupportedChangeEpoch,
		IncludeFirstState:                opts.IncludeFirstState,
		IncludeLastState:                 opts.IncludeLastState,
	})
	if err != nil {
		return nil, fmt.Errorf("signal.FetchGroupLogs: %w", err)
	}

	var changes groupspb.GroupChanges
	if len(page.Body) > 0 {
		if err := proto.Unmarshal(page.Body, &changes); err != nil {
			return nil, fmt.Errorf("signal.FetchGroupLogs: decode: %w", err)
		}
	}

	out := &GroupLogsPage{
		Changes:      &changes,
		Partial:      page.StatusCode == http.StatusPartialContent,
		ContentRange: page.ContentRange,
	}
	if out.Partial {
		next, err := web.NextGroupLogRevision(page.ContentRange)
		if err != nil {
			return nil, fmt.Errorf("signal.FetchGroupLogs: content range: %w", err)
		}
		out.NextRevision = next
	}
	return out, nil
}

// SyncGroup incrementally syncs group state from fromRevision using
// GET /v2/groups/logs/{version}. When log entries include groupState
// snapshots, the latest snapshot is decoded; otherwise [FetchGroup] is used
// as a fallback once paging completes.
func (c *Client) SyncGroup(ctx context.Context, masterKey []byte, fromRevision uint32) (*Group, error) {
	if len(masterKey) != libsignal.GroupMasterKeyLen {
		return nil, fmt.Errorf("signal.SyncGroup: master key length %d, want %d", len(masterKey), libsignal.GroupMasterKeyLen)
	}
	if c.storageWebc == nil {
		return nil, errors.New("signal.SyncGroup: Client was opened without groups storage")
	}

	masterKeyHex := hex.EncodeToString(masterKey)
	secretParams, err := libsignal.GroupSecretParamsFromMasterKey(masterKey)
	if err != nil {
		return nil, err
	}

	var latestWire *groupspb.Group
	var latestGSE []byte
	revision := fromRevision
	cachedGSEExp := c.groupSendEndorsementExpirationUnix(masterKeyHex)

	for pageNum := 0; pageNum < 256; pageNum++ {
		page, err := c.FetchGroupLogs(ctx, masterKey, GroupLogsFetchOptions{
			FromRevision:                     revision,
			CachedSendEndorsementsExpiration: cachedGSEExp,
			IncludeFirstState:                pageNum == 0,
			IncludeLastState:                 true,
			MaxSupportedChangeEpoch:          7, // GROUP_TERMINATION_EPOCH
		})
		if err != nil {
			return nil, fmt.Errorf("signal.SyncGroup: %w", err)
		}
		if page.Changes != nil {
			for _, entry := range page.Changes.GetGroupChanges() {
				if entry.GetGroupState() != nil {
					latestWire = entry.GetGroupState()
				}
			}
			if gse := page.Changes.GetGroupSendEndorsementsResponse(); len(gse) > 0 {
				latestGSE = gse
			}
		}
		if !page.Partial {
			break
		}
		revision = page.NextRevision
		cachedGSEExp = 0
	}

	if latestWire == nil {
		return c.FetchGroup(ctx, masterKey)
	}

	grp, memberACIs, err := c.groupFromWire(masterKeyHex, secretParams, latestWire)
	if err != nil {
		return nil, fmt.Errorf("signal.SyncGroup: %w", err)
	}
	if len(latestGSE) > 0 {
		if err := c.storeGroupSendEndorsements(masterKeyHex, secretParams, latestGSE, memberACIs); err != nil {
			c.log.Warn("group send endorsements unavailable after sync", "group", masterKeyHex, "err", err)
		}
	}
	return grp, nil
}

func (c *Client) groupFromWire(
	masterKeyHex string,
	secretParams [libsignal.GroupSecretParamsLen]byte,
	wire *groupspb.Group,
) (*Group, []string, error) {
	state, err := group.DecodeState(secretParams, wire)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt: %w", err)
	}
	return groupFromDecodedState(masterKeyHex, state)
}

func groupFromDecodedState(masterKeyHex string, state *group.State) (*Group, []string, error) {
	if state == nil {
		return nil, nil, errors.New("signal: nil group state")
	}
	members := make([]GroupMember, len(state.Members))
	memberACIs := make([]string, len(state.Members))
	for i, m := range state.Members {
		members[i] = GroupMember{ACI: m.ACI, Role: m.Role}
		memberACIs[i] = m.ACI
	}
	return &Group{
		ID:          masterKeyHex,
		Title:       state.Title,
		Description: state.Description,
		AvatarURL:   state.AvatarURL,
		Revision:    state.Revision,
		Members:     members,
	}, memberACIs, nil
}

func (c *Client) groupSendEndorsementExpirationUnix(masterKeyHex string) int64 {
	c.groupEndorseMu.Lock()
	cache := c.groupEndorsements[masterKeyHex]
	c.groupEndorseMu.Unlock()
	if cache != nil && !cache.expiration.IsZero() {
		return cache.expiration.Unix()
	}
	if c.groupEndorseStore != nil {
		rec, err := c.groupEndorseStore.LoadGroupEndorsements(masterKeyHex)
		if err == nil && !rec.Expiration.IsZero() {
			return rec.Expiration.Unix()
		}
	}
	return 0
}
