// Command group-crawler is an example that walks Signal group membership
// starting from your linked account's storage list (and optional seed invite).
//
// For each group it logs metadata, scans descriptions and member profile bios
// for signal.group invite links, joins new groups, and logs inbound group
// events as they arrive. Message history is not available over the API — only
// live traffic after connect is logged.
//
// Use only on accounts and communities you are allowed to automate.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/thehappydinoa/signal-go/internal/cliargs"
	sg "github.com/thehappydinoa/signal-go/pkg/signal"
)

const logBanner = "════════════════════════════════════════════════════════════════"

var inviteLinkRE = regexp.MustCompile(`(?i)(?:https://signal\.group/#[A-Za-z0-9_-]+|sgnl://signal\.group/#[A-Za-z0-9_-]+)`)

func main() {
	log.SetFlags(0)
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("group-crawler", flag.ExitOnError)
	storeDir, passphraseFile, plaintext := cliargs.StoreBind(fs, ".signal-group-crawler")
	clientPreset, userAgent := cliargs.ClientBind(fs)
	seedInvite := fs.String("seed-invite", "", "optional signal.group invite URL to join first")
	maxGroups := fs.Int("max-groups", 0, "stop after visiting this many groups (0 = no limit)")
	joinCooldown := fs.Duration("join-cooldown", 3*time.Second, "minimum delay between invite joins")
	dryRun := fs.Bool("dry-run", false, "log invite links but do not join new groups")
	_ = fs.Parse(args)

	client, err := cliargs.ClientFromFlags(clientPreset, userAgent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client: %v\n", err)
		return 2
	}

	store, err := cliargs.StoreFromFlags(storeDir, passphraseFile, plaintext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "store: %v\n", err)
		return 1
	}

	db, err := store.OpenSQLStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	sgClient, err := sg.Open(ctx, sg.OpenOptions{
		AccountStore:           db,
		SignalStores:           db.SignalStores(),
		GroupDistributionStore: db.GroupDistributionStore(),
		GroupEndorsementStore:  db.GroupEndorsementStore(),
		ClientProfile:          client.Profile,
		UserAgent:              client.UserAgent,
		AutoSyncStorage:        true,
		AutoSyncGroupUpdates:   true,
	})
	if err != nil {
		if errors.Is(err, sg.ErrNotLinked) {
			fmt.Fprintln(os.Stderr, "store is not linked; run: signal-go link -store <path>")
			return 1
		}
		fmt.Fprintf(os.Stderr, "open client: %v\n", err)
		return 1
	}
	defer func() { _ = sgClient.Close() }()

	c := &crawler{
		client:       sgClient,
		selfACI:      sgClient.Account().ACI,
		maxGroups:    *maxGroups,
		joinCooldown: *joinCooldown,
		dryRun:       *dryRun,
		profileKeys:  map[string][]byte{},
	}
	c.log("START", "group crawler connected aci=%s device=%d store=%s dry_run=%v",
		c.selfACI, sgClient.Account().DeviceID, store.Dir, *dryRun)

	if err := c.seedProfileKeys(ctx); err != nil {
		c.log("WARN", "storage sync: %v", err)
	}

	if strings.TrimSpace(*seedInvite) != "" {
		c.enqueueInvites(ctx, "seed-flag", *seedInvite)
	}

	c.startWorkers(ctx)

	for {
		select {
		case <-ctx.Done():
			c.log("STOP", "shutting down (visited %d groups, joined %d via invite)", c.groupsVisited, c.groupsJoined)
			return 0
		case <-sgClient.Done():
			c.log("STOP", "connection closed")
			return 0
		case ev, ok := <-sgClient.Events():
			if !ok {
				c.log("STOP", "event channel closed")
				return 0
			}
			c.handleEvent(ctx, ev)
		}
	}
}

type crawler struct {
	client       *sg.Client
	selfACI      string
	maxGroups    int
	joinCooldown time.Duration
	dryRun       bool

	mu            sync.Mutex
	visitedGroup  map[string]struct{}
	visitedInvite map[string]struct{} // hex-encoded group master key from invite
	profileKeys   map[string][]byte
	groupQueue    []string
	inviteQueue   []string
	groupsVisited int
	groupsJoined  int
	lastJoin      time.Time
}

func (c *crawler) log(kind, format string, args ...any) {
	prefix := fmt.Sprintf("CRAWL|%s|", kind)
	log.Printf(prefix+format, args...)
}

func (c *crawler) seedProfileKeys(ctx context.Context) error {
	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result, err := c.client.SyncStorage(syncCtx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	for _, contact := range result.Contacts {
		if contact.ACI != "" && len(contact.ProfileKey) > 0 {
			c.profileKeys[contact.ACI] = append([]byte(nil), contact.ProfileKey...)
		}
	}
	c.mu.Unlock()
	c.log("STORAGE", "manifest version=%d contacts=%d groups=%d unchanged=%v",
		result.Version, len(result.Contacts), len(result.Groups), result.Unchanged)
	for _, g := range result.Groups {
		c.enqueueGroup(g.ID)
	}
	return nil
}

func (c *crawler) profileKeyFor(aci string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.profileKeys[aci]...)
}

func (c *crawler) enqueueGroup(groupID string) {
	groupID = strings.ToLower(strings.TrimSpace(groupID))
	if groupID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.visitedGroup == nil {
		c.visitedGroup = make(map[string]struct{})
	}
	if _, ok := c.visitedGroup[groupID]; ok {
		return
	}
	c.visitedGroup[groupID] = struct{}{}
	c.groupQueue = append(c.groupQueue, groupID)
	c.log("QUEUE", "group %s (pending groups=%d)", shortID(groupID), len(c.groupQueue))
}

func (c *crawler) enqueueInvites(ctx context.Context, source, text string) {
	for _, raw := range extractInviteLinks(text) {
		url := strings.TrimSpace(raw)
		dedupeKey := inviteDedupeKey(url)
		if dedupeKey == "" {
			continue
		}
		c.mu.Lock()
		if c.visitedInvite == nil {
			c.visitedInvite = make(map[string]struct{})
		}
		if _, ok := c.visitedInvite[dedupeKey]; ok {
			c.mu.Unlock()
			continue
		}
		c.visitedInvite[dedupeKey] = struct{}{}
		c.inviteQueue = append(c.inviteQueue, url)
		pending := len(c.inviteQueue)
		c.mu.Unlock()
		c.log("INVITE|FOUND", "source=%s url=%s (pending invites=%d)", source, url, pending)
	}
}

func (c *crawler) startWorkers(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			url := c.popInvite()
			if url == "" {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			c.processInvite(ctx, url)
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			groupID := c.popGroup()
			if groupID == "" {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			if c.maxGroups > 0 && c.groupsVisited >= c.maxGroups {
				c.log("LIMIT", "max-groups=%d reached; not visiting more", c.maxGroups)
				time.Sleep(time.Second)
				continue
			}
			c.visitGroup(ctx, groupID)
		}
	}()
}

func (c *crawler) popInvite() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.inviteQueue) == 0 {
		return ""
	}
	url := c.inviteQueue[0]
	c.inviteQueue = c.inviteQueue[1:]
	return url
}

func (c *crawler) popGroup() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.groupQueue) == 0 {
		return ""
	}
	id := c.groupQueue[0]
	c.groupQueue = c.groupQueue[1:]
	return id
}

func (c *crawler) processInvite(ctx context.Context, inviteURL string) {
	c.waitJoinCooldown()

	previewCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	preview, err := c.client.PreviewGroupJoin(previewCtx, inviteURL)
	cancel()
	if err != nil {
		c.log("INVITE|PREVIEW_FAIL", "url=%s err=%v", inviteURL, err)
		return
	}
	c.log("INVITE|PREVIEW", logBanner)
	c.log("INVITE|PREVIEW", "url=%s", inviteURL)
	c.log("INVITE|PREVIEW", "title=%q members=%d revision=%d admin_approval=%v",
		preview.Title, preview.MemberCount, preview.Revision, preview.RequiresAdminApproval)
	c.log("INVITE|PREVIEW", "description=%q", preview.Description)
	c.log("INVITE|PREVIEW", logBanner)

	c.enqueueInvites(ctx, "invite-preview:"+inviteURL, preview.Description)

	if c.dryRun {
		c.log("INVITE|SKIP", "dry-run: not joining %s", inviteURL)
		return
	}

	joinCtx, cancelJoin := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelJoin()
	grp, err := c.client.JoinGroupViaInviteLink(joinCtx, inviteURL)
	if err != nil {
		c.log("INVITE|JOIN_FAIL", "url=%s err=%v", inviteURL, err)
		return
	}
	c.mu.Lock()
	c.groupsJoined++
	c.mu.Unlock()
	c.log("INVITE|JOIN_OK", "joined group_id=%s title=%q members=%d", grp.ID, grp.Title, len(grp.Members))
	c.enqueueGroup(grp.ID)
}

func (c *crawler) waitJoinCooldown() {
	if c.joinCooldown <= 0 {
		return
	}
	c.mu.Lock()
	wait := c.joinCooldown - time.Since(c.lastJoin)
	c.mu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
	c.mu.Lock()
	c.lastJoin = time.Now()
	c.mu.Unlock()
}

func (c *crawler) visitGroup(ctx context.Context, groupID string) {
	c.mu.Lock()
	c.groupsVisited++
	visitNum := c.groupsVisited
	c.mu.Unlock()

	masterKey, err := hex.DecodeString(groupID)
	if err != nil || len(masterKey) == 0 {
		c.log("GROUP|ERROR", "invalid group id %q: %v", groupID, err)
		return
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	grp, err := c.client.SyncGroup(fetchCtx, masterKey, 0)
	cancel()
	if err != nil {
		// Fallback snapshot if log sync fails.
		fetchCtx2, cancel2 := context.WithTimeout(ctx, 45*time.Second)
		grp, err = c.client.FetchGroup(fetchCtx2, masterKey)
		cancel2()
	}
	if err != nil {
		c.log("GROUP|ERROR", "fetch group_id=%s err=%v", groupID, err)
		return
	}

	c.log("GROUP|VISIT", logBanner)
	c.log("GROUP|VISIT", "#%d group_id=%s", visitNum, grp.ID)
	c.log("GROUP|VISIT", "title=%q", grp.Title)
	c.log("GROUP|VISIT", "description=%q", grp.Description)
	c.log("GROUP|VISIT", "avatar_url=%q revision=%d member_count=%d",
		grp.AvatarURL, grp.Revision, len(grp.Members))
	c.log("GROUP|VISIT", "admins=%v", grp.Admins())
	for _, m := range grp.Members {
		role := "member"
		if m.Role == sg.GroupRoleAdministrator {
			role = "admin"
		}
		c.log("GROUP|MEMBER", "aci=%s role=%s", m.ACI, role)
	}
	c.log("GROUP|VISIT", logBanner)

	c.enqueueInvites(ctx, "group-description:"+groupID, grp.Description)
	c.scanMemberBios(ctx, grp)
}

func (c *crawler) scanMemberBios(ctx context.Context, grp *sg.Group) {
	for _, m := range grp.Members {
		if m.ACI == c.selfACI {
			continue
		}
		key := c.profileKeyFor(m.ACI)
		if len(key) == 0 {
			c.log("PROFILE|SKIP", "aci=%s (no profile key yet)", m.ACI)
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		prof, err := c.client.FetchProfile(pctx, m.ACI, key)
		cancel()
		if err != nil {
			c.log("PROFILE|FAIL", "aci=%s err=%v", m.ACI, err)
			continue
		}
		c.log("PROFILE|OK", "aci=%s name=%q about=%q about_emoji=%q",
			m.ACI, prof.DisplayName(), prof.About, prof.AboutEmoji)
		c.enqueueInvites(ctx, "profile-about:"+m.ACI, prof.About)
	}
}

func (c *crawler) handleEvent(ctx context.Context, ev sg.Event) {
	switch e := ev.(type) {
	case *sg.MessageEvent:
		extra := ""
		if n := len(e.Attachments); n > 0 {
			extra = fmt.Sprintf("attachments=%d", n)
		}
		c.logEvent("MESSAGE", e.GroupID, e.Sender, e.Timestamp, e.Body, extra)
		if e.GroupID != "" {
			c.enqueueInvites(ctx, "message:"+e.GroupID, e.Body)
		}
	case *sg.EditMessageEvent:
		c.logEvent("EDIT", e.GroupID, e.Sender, e.Timestamp, e.NewBody, fmt.Sprintf("target_ts=%s", e.TargetTimestamp.Format(time.RFC3339)))
		if e.GroupID != "" {
			c.enqueueInvites(ctx, "edit:"+e.GroupID, e.NewBody)
		}
	case *sg.ReactionEvent:
		c.log("EVENT|REACTION", "group=%s sender=%s emoji=%q remove=%v target_author=%s target_ts=%s",
			shortID(e.GroupID), e.Sender, e.Emoji, e.Remove, e.TargetAuthorACI, e.TargetTimestamp.Format(time.RFC3339))
	case *sg.GroupUpdateEvent:
		c.log("EVENT|GROUP_UPDATE", "group=%s sender=%s revision=%d change_bytes=%d",
			shortID(e.GroupID), e.Sender, e.Revision, len(e.GroupChange))
		if e.GroupID != "" {
			c.enqueueGroup(e.GroupID)
		}
	case *sg.TypingEvent:
		c.log("EVENT|TYPING", "group=%s sender=%s action=%d", shortID(e.GroupID), e.Sender, e.Action)
	case *sg.ReceiptEvent:
		c.log("EVENT|RECEIPT", "sender=%s type=%d acks=%d", e.Sender, e.Type, len(e.Timestamps))
	case *sg.SyncMessageEvent:
		if e.SentBody != "" {
			c.log("EVENT|SYNC_SENT", "to=%s body=%q", e.SentTo, truncate(e.SentBody, 200))
			c.enqueueInvites(ctx, "sync-sent", e.SentBody)
		}
	case *sg.DecryptionErrorEvent:
		c.log("EVENT|DECRYPT_ERR", "sender=%s err=%v", e.Sender, e.Err)
	case *sg.QueueEmptyEvent:
		c.log("EVENT|QUEUE_EMPTY", "server queue drained at %s", e.Timestamp.Format(time.RFC3339))
	default:
		c.log("EVENT|OTHER", "%T", ev)
	}
}

func (c *crawler) logEvent(kind, groupID, sender string, ts time.Time, body, extra string) {
	g := "dm"
	if groupID != "" {
		g = shortID(groupID)
	}
	line := fmt.Sprintf("group=%s sender=%s ts=%s body=%q", g, sender, ts.Format(time.RFC3339Nano), truncate(body, 400))
	if extra != "" {
		line += " " + extra
	}
	c.log("EVENT|"+kind, "%s", line)
}

func extractInviteLinks(text string) []string {
	if text == "" {
		return nil
	}
	raw := inviteLinkRE.FindAllString(text, -1)
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, s := range raw {
		s = strings.TrimRight(s, ".,;:!?)\"']")
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func inviteDedupeKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	mk, _, err := sg.ParseGroupInviteLink(raw)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(mk)
}

func shortID(hexID string) string {
	if len(hexID) <= 16 {
		return hexID
	}
	return hexID[:8] + "…" + hexID[len(hexID)-4:]
}

func truncate(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit] + "…"
}
