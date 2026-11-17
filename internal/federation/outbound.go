package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md (M5 — consume-side)
// Member-originated federation: outbound Follow/Undo, remote WebFinger
// resolution, the pull-at-read remote timeline, and outbound Move —
// the mirror image of everything M2-M4 accepted inbound.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// ErrInvalidTarget marks a follow/move target that can't resolve to a
// remote actor — malformed handle, unsupported scheme, or a local URL.
var ErrInvalidTarget = errors.New("federation: invalid remote target")

// ErrMoveTooSoon marks an outbound Move inside the
// federation.move.cooldown_days window — the handler maps it to 429.
var ErrMoveTooSoon = errors.New("federation: move cooldown active")

// remoteTimelineMaxLimit caps the member-supplied page size (§126 M5).
const remoteTimelineMaxLimit = 50

// RemoteFollowResult is the member-facing view of one outbound edge.
type RemoteFollowResult struct {
	RemoteActorID     pgtype.UUID `json:"remote_actor_id"`
	ActorURL          string      `json:"actor_url"`
	Acct              *string     `json:"acct"`
	PreferredUsername *string     `json:"preferred_username"`
	DisplayName       *string     `json:"display_name"`
	AvatarURL         *string     `json:"avatar_url"`
	Status            string      `json:"status"`
	CreatedAt         time.Time   `json:"created_at"`
}

// memberActor resolves the caller's live Person actor — the outbound
// paths share the "federation enabled and active" gate.
func (s *Service) memberActor(ctx context.Context, userID pgtype.UUID) (gen.FederatedActor, error) {
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return actor, ErrNotFederated
		}
		return actor, err
	}
	if actor.Status != ActorStatusActive {
		return actor, ErrNotFederated
	}
	return actor, nil
}

// resolveTargetURL turns the member's freeform target into an actor URL:
// acct shapes go through remote WebFinger, https URLs pass through.
// Refused shapes and non-remote targets return ErrInvalidTarget.
func (s *Service) resolveTargetURL(ctx context.Context, target string) (string, error) {
	t := strings.TrimSpace(target)
	if t == "" {
		return "", ErrInvalidTarget
	}
	t = strings.TrimPrefix(t, "@")
	t = strings.TrimPrefix(t, "acct:")
	if strings.HasPrefix(t, "https://") {
		if sameURLHost(t, s.apiBase()) || sameURLHost(t, s.siteBase()) {
			return "", ErrInvalidTarget // self-follow is not remote federation
		}
		return t, nil
	}
	if strings.Contains(t, "://") {
		return "", ErrInvalidTarget // only https actor URLs resolve
	}
	// acct shape: user@domain — both halves must be free of URL syntax,
	// whitespace, and control chars before they ride into a remote
	// WebFinger path.
	local, domain, ok := strings.Cut(t, "@")
	if !ok || local == "" || domain == "" ||
		strings.ContainsAny(domain, " \t\r\n/?#@") || strings.ContainsAny(local, " \t\r\n/?#@") {
		return "", ErrInvalidTarget
	}
	if !s.instanceAllowed(ctx, domain) {
		return "", fmt.Errorf("%w: %s", ErrRemoteBlocked, domain)
	}
	wfURL := fmt.Sprintf("https://%s/.well-known/webfinger?resource=acct:%s@%s",
		domain, url.QueryEscape(local), url.QueryEscape(domain))
	body, err := s.fetchAP(ctx, wfURL)
	if err != nil {
		return "", fmt.Errorf("%w: webfinger: %v", ErrRemoteFetch, err)
	}
	var jrd struct {
		Links []struct {
			Rel  string `json:"rel"`
			Type string `json:"type"`
			Href string `json:"href"`
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &jrd); err != nil {
		return "", fmt.Errorf("%w: webfinger parse: %v", ErrRemoteFetch, err)
	}
	for _, l := range jrd.Links {
		if l.Rel == "self" && l.Href != "" &&
			strings.Contains(l.Type, "activity+json") {
			return l.Href, nil
		}
	}
	return "", fmt.Errorf("%w: webfinger advertises no actor", ErrRemoteFetch)
}

// fetchTargetActor fetches + validates + upserts the follow/move target.
// The doc must be an actor type on an unblocked instance advertising an
// https inbox — a Follow that can never be delivered is never ledgered.
func (s *Service) fetchTargetActor(ctx context.Context, actorURL string) (*gen.RemoteActor, string, error) {
	u, err := url.Parse(actorURL)
	if err != nil || u.Hostname() == "" || !strings.EqualFold(u.Scheme, "https") {
		return nil, "", ErrInvalidTarget
	}
	if !s.instanceAllowed(ctx, u.Hostname()) {
		return nil, "", fmt.Errorf("%w: %s", ErrRemoteBlocked, u.Hostname())
	}
	doc, err := s.fetchRemoteActorDoc(ctx, actorURL)
	if err != nil {
		return nil, "", err
	}
	inbox := doc.Endpoints.SharedInbox
	if inbox == "" {
		inbox = doc.Inbox
	}
	iu, ierr := url.Parse(inbox)
	if ierr != nil || !strings.EqualFold(iu.Scheme, "https") || iu.Hostname() == "" {
		return nil, "", fmt.Errorf("%w: actor advertises unusable inbox", ErrRemoteFetch)
	}
	remote, err := s.upsertRemoteActor(ctx, actorURL, doc)
	if err != nil {
		return nil, "", fmt.Errorf("%w: upsert: %v", ErrRemoteFetch, err)
	}
	s.cacheSetRemote(ctx, remote)
	return remote, inbox, nil
}

// FollowRemote drives POST /me/federation/following: resolve the target,
// open (or resurrect) the outbound edge, ledger the signed Follow.
// Idempotent — a live pending/accepted edge answers without re-sending.
func (s *Service) FollowRemote(ctx context.Context, userID pgtype.UUID, target string) (RemoteFollowResult, error) {
	actor, err := s.memberActor(ctx, userID)
	if err != nil {
		return RemoteFollowResult{}, err
	}
	actorURL, err := s.resolveTargetURL(ctx, target)
	if err != nil {
		return RemoteFollowResult{}, err
	}
	remote, inbox, err := s.fetchTargetActor(ctx, actorURL)
	if err != nil {
		return RemoteFollowResult{}, err
	}
	// A live edge is the answer — Mastodon's "already following" POST is
	// equally idempotent. Dead edges (rejected/undone) fall through to a
	// fresh Follow id, mirroring relay resubscribe semantics.
	if edge, eerr := s.store.followByPair(ctx, actor.ID, remote.ID, "outbound"); eerr == nil &&
		(edge.Status == "pending" || edge.Status == "accepted") {
		return remoteFollowResult(edge, remote), nil
	}
	followID := fmt.Sprintf("%s#follow/%s", actor.ActorUrl, uuid.NewString())
	edge, err := s.store.upsertFollow(ctx, gen.UpsertFederationFollowParams{
		LocalActorID:  actor.ID,
		RemoteActorID: remote.ID,
		ApActivityID:  &followID,
		Status:        "pending",
		Direction:     "outbound",
	})
	if err != nil {
		return RemoteFollowResult{}, fmt.Errorf("federation: follow edge: %w", err)
	}
	// The ledgered ap id is authoritative — a resurrected edge carries
	// the fresh id the upsert just wrote.
	apID := followID
	if edge.ApActivityID != nil && *edge.ApActivityID != "" {
		apID = *edge.ApActivityID
	}
	follow := map[string]any{
		"@context": asContext,
		"id":       apID,
		"type":     "Follow",
		"actor":    actor.ActorUrl,
		"object":   actorURL,
		"to":       []string{actorURL},
	}
	row := gen.ListRemoteFollowerInboxesByActorRow{InboxUrl: &inbox, RemoteActorID: remote.ID}
	if _, err := s.insertDeliveries(ctx, actor.ID, follow, apID,
		[]gen.ListRemoteFollowerInboxesByActorRow{row}, true); err != nil {
		return RemoteFollowResult{}, err
	}
	return remoteFollowResult(edge, remote), nil
}

func remoteFollowResult(edge gen.FederationFollow, remote *gen.RemoteActor) RemoteFollowResult {
	return RemoteFollowResult{
		RemoteActorID:     remote.ID,
		ActorURL:          remote.ActorUrl,
		Acct:              remote.Acct,
		PreferredUsername: remote.PreferredUsername,
		DisplayName:       remote.DisplayName,
		AvatarURL:         remote.AvatarUrl,
		Status:            edge.Status,
		CreatedAt:         edge.CreatedAt.Time,
	}
}

// ListOutboundFollowing serves GET /me/federation/following.
func (s *Service) ListOutboundFollowing(ctx context.Context, userID pgtype.UUID) ([]RemoteFollowResult, error) {
	if _, err := s.memberActor(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.store.read.ListOutboundFederationFollowsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]RemoteFollowResult, 0, len(rows))
	for _, r := range rows {
		out = append(out, RemoteFollowResult{
			RemoteActorID:     r.RemoteActorID,
			ActorURL:          r.ActorUrl,
			Acct:              r.Acct,
			PreferredUsername: r.PreferredUsername,
			DisplayName:       r.DisplayName,
			AvatarURL:         r.AvatarUrl,
			Status:            r.Status,
			CreatedAt:         r.CreatedAt.Time,
		})
	}
	return out, nil
}

// UnfollowRemote drives DELETE /me/federation/following/{remote_actor_id}:
// ledger Undo{Follow} (object = the verbatim Follow we sent), flip the
// edge undone, drop the follows mirror. Works against pending (withdraw)
// and accepted alike; already-dead edges are a no-op 200.
func (s *Service) UnfollowRemote(ctx context.Context, userID, remoteActorID pgtype.UUID) error {
	actor, err := s.memberActor(ctx, userID)
	if err != nil {
		return err
	}
	edge, err := s.store.followByPair(ctx, actor.ID, remoteActorID, "outbound")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if edge.Status == "undone" {
		return nil // replayed unfollow converges
	}
	remote, err := s.store.remoteActorByID(ctx, remoteActorID)
	if err != nil {
		return fmt.Errorf("federation: remote actor: %w", err)
	}
	inbox := remote.SharedInboxUrl
	if inbox == nil || *inbox == "" {
		inbox = remote.InboxUrl
	}
	// The Undo embeds the verbatim Follow — remotes match on its @id.
	// An edge that never minted an ap id (corrupt row) undoes by shape.
	followObj := map[string]any{
		"type":   "Follow",
		"actor":  actor.ActorUrl,
		"object": remote.ActorUrl,
	}
	if edge.ApActivityID != nil && *edge.ApActivityID != "" {
		followObj["id"] = *edge.ApActivityID
	}
	undoID := fmt.Sprintf("%s#undo-follow/%s", actor.ActorUrl, uuid.NewString())
	undo := map[string]any{
		"@context": asContext,
		"id":       undoID,
		"type":     "Undo",
		"actor":    actor.ActorUrl,
		"object":   followObj,
		"to":       []string{remote.ActorUrl},
	}
	if inbox != nil && *inbox != "" {
		row := gen.ListRemoteFollowerInboxesByActorRow{InboxUrl: inbox, RemoteActorID: remote.ID}
		// The Undo must be ledgered BEFORE the edge closes — an
		// unledgered unfollow leaves the remote believing the member
		// still follows them (and leaves nothing for the sweeper to
		// retry). A missing/dead inbox is the one case that skips this
		// gate: there is nowhere to deliver anyway.
		if _, derr := s.insertDeliveries(ctx, actor.ID, undo, undoID,
			[]gen.ListRemoteFollowerInboxesByActorRow{row}, true); derr != nil {
			return fmt.Errorf("federation: unfollow delivery: %w", derr)
		}
	}
	if err := s.store.setFollowStatus(ctx, actor.ID, remoteActorID, "outbound", "undone"); err != nil {
		return fmt.Errorf("federation: unfollow edge: %w", err)
	}
	if _, err := s.store.write.DeleteOutboundFollowMirror(ctx, gen.DeleteOutboundFollowMirrorParams{
		FollowerID: userID, RemoteActorID: remoteActorID,
	}); err != nil {
		slog.Warn("federation: unfollow mirror delete failed", "error", err)
	}
	return nil
}

// RemoteTimelineItem is one timeline entry (§126 M5) — remote content
// reaches members as plain text only (the remote-comment posture).
type RemoteTimelineItem struct {
	ID           pgtype.UUID `json:"id"`
	ApID         string      `json:"ap_id"`
	URL          *string     `json:"url"`
	Name         *string     `json:"name"`
	Summary      *string     `json:"summary"`
	Sensitive    bool        `json:"sensitive"`
	ContentText  string      `json:"content_text"`
	InReplyToURL *string     `json:"in_reply_to_url"`
	IsPinned     bool        `json:"is_pinned"`
	ShareCount   int32       `json:"share_count"`
	Hashtags     []string    `json:"hashtags"`
	PublishedAt  *time.Time  `json:"published_at"`
	EditedAt     *time.Time  `json:"edited_at"`
	Actor        struct {
		Acct              *string `json:"acct"`
		ActorURL          string  `json:"actor_url"`
		PreferredUsername *string `json:"preferred_username"`
		DisplayName       *string `json:"display_name"`
		AvatarURL         *string `json:"avatar_url"`
	} `json:"actor"`
}

// RemoteTimelinePage wraps the item page with its keyset cursor.
type RemoteTimelinePage struct {
	Items      []RemoteTimelineItem `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

// RemoteTimeline serves GET /me/federation/timeline — pull-at-read over
// the member's accepted outbound edges, keyset on (published, id) DESC.
func (s *Service) RemoteTimeline(ctx context.Context, userID pgtype.UUID, cursor string, limit int32) (RemoteTimelinePage, error) {
	if _, err := s.memberActor(ctx, userID); err != nil {
		return RemoteTimelinePage{}, err
	}
	if limit <= 0 {
		limit = int32(s.intSetting(ctx, "federation.timeline_page_size", 20))
	}
	if limit > remoteTimelineMaxLimit {
		limit = remoteTimelineMaxLimit
	}
	var cursorKey *time.Time
	var cursorID *pgtype.UUID
	if cursor != "" {
		raw, derr := base64.RawURLEncoding.DecodeString(cursor)
		if derr != nil {
			return RemoteTimelinePage{}, ErrInvalidTarget
		}
		ts, idPart, ok := strings.Cut(string(raw), "|")
		if !ok {
			return RemoteTimelinePage{}, ErrInvalidTarget
		}
		t, terr := time.Parse(time.RFC3339Nano, ts)
		var id pgtype.UUID
		if terr != nil || id.Scan(idPart) != nil || !id.Valid {
			return RemoteTimelinePage{}, ErrInvalidTarget
		}
		cursorKey, cursorID = &t, &id
	}
	var cursorKeyT pgtype.Timestamptz
	if cursorKey != nil {
		cursorKeyT = pgtype.Timestamptz{Time: *cursorKey, Valid: true}
	}
	var cursorIDU pgtype.UUID
	if cursorID != nil {
		cursorIDU = *cursorID
	}
	// +1 row probes for a following page — same trick feeds use.
	rows, err := s.store.read.ListRemoteTimelinePosts(ctx, gen.ListRemoteTimelinePostsParams{
		UserID:    userID,
		CursorKey: cursorKeyT,
		CursorID:  cursorIDU,
		PageLimit: limit + 1,
	})
	if err != nil {
		return RemoteTimelinePage{}, fmt.Errorf("federation: timeline: %w", err)
	}
	page := RemoteTimelinePage{Items: make([]RemoteTimelineItem, 0, len(rows))}
	if int32(len(rows)) > limit {
		last := rows[limit-1]
		key := last.CreatedAt.Time
		if last.PublishedAt.Valid {
			key = last.PublishedAt.Time
		}
		c := base64.RawURLEncoding.EncodeToString(
			[]byte(fmt.Sprintf("%s|%s", key.UTC().Format(time.RFC3339Nano),
				uuid.UUID(last.ID.Bytes).String())))
		page.NextCursor = &c
		rows = rows[:limit]
	}
	for _, r := range rows {
		item := RemoteTimelineItem{
			ID:           r.ID,
			ApID:         r.ApID,
			URL:          r.Url,
			Name:         r.Name,
			Summary:      r.Summary,
			Sensitive:    r.Sensitive,
			ContentText:  r.ContentText,
			InReplyToURL: r.InReplyToUrl,
			IsPinned:     r.IsPinned,
			ShareCount:   r.ShareCount,
			Hashtags:     r.Hashtags,
		}
		if r.PublishedAt.Valid {
			t := r.PublishedAt.Time
			item.PublishedAt = &t
		}
		if r.EditedAt.Valid {
			t := r.EditedAt.Time
			item.EditedAt = &t
		}
		item.Actor.Acct = r.Acct
		item.Actor.ActorURL = r.ActorUrl
		item.Actor.PreferredUsername = r.PreferredUsername
		item.Actor.DisplayName = r.DisplayName
		item.Actor.AvatarURL = r.AvatarUrl
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// moveCooldown reads federation.move.cooldown_days (§126 M5).
func (s *Service) moveCooldown(ctx context.Context) time.Duration {
	return time.Duration(s.intSetting(ctx, "federation.move.cooldown_days", 30)) * 24 * time.Hour
}

// intSetting is the small settings accessor the M5 routes share.
func (s *Service) intSetting(ctx context.Context, key string, fallback int) int {
	if s.settings == nil {
		return fallback
	}
	v, err := s.settings.GetSetting(ctx, key)
	if err != nil || v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

// RequestMove drives POST /me/federation/move — the outbound half of AP
// account migration (§126 M5). The target's own doc must list OUR actor
// URL in alsoKnownAs (their setup half); our signed Move is ours.
func (s *Service) RequestMove(ctx context.Context, userID pgtype.UUID, targetURL string) error {
	actor, err := s.memberActor(ctx, userID)
	if err != nil {
		return err
	}
	t := strings.TrimSpace(targetURL)
	u, perr := url.Parse(t)
	if perr != nil || u.Hostname() == "" || !strings.EqualFold(u.Scheme, "https") ||
		sameURLHost(t, s.apiBase()) || strings.EqualFold(t, actor.ActorUrl) {
		return ErrInvalidTarget
	}
	if !s.instanceAllowed(ctx, u.Hostname()) {
		return fmt.Errorf("%w: %s", ErrRemoteBlocked, u.Hostname())
	}
	doc, err := s.fetchRemoteActorDoc(ctx, t)
	if err != nil {
		return err
	}
	if !doc.advertisesAlsoKnownAs(actor.ActorUrl) {
		return ErrMoveNotConfirmed
	}
	cooldown := s.moveCooldown(ctx)
	if n, err := s.store.write.SetFederatedActorMoved(ctx, gen.SetFederatedActorMovedParams{
		ID:              actor.ID,
		MovedToUrl:      &t,
		CooldownSeconds: cooldown.Seconds(),
	}); err != nil {
		return fmt.Errorf("federation: move stamp: %w", err)
	} else if n == 0 {
		// The UPDATE's own predicate holds the cooldown — a concurrent
		// request that already stamped shows up here as within-window.
		fresh, ferr := s.store.read.GetFederatedActorByID(ctx, actor.ID)
		if ferr != nil {
			return ErrNotFound // actor row vanished between gate and stamp
		}
		if fresh.MovedAt.Valid && time.Since(fresh.MovedAt.Time) < cooldown {
			return ErrMoveTooSoon
		}
		return ErrNotFound
	}
	// The doc cache still serves the pre-move actor — the redirect must
	// appear on the next read.
	s.InvalidateActorIdentity(ctx, actor.PreferredUsername)

	// Advisory fanout: remote followers get Move{actor→target} signed by
	// the member's key. Same inbox expansion as content fanout.
	inboxes, err := s.store.remoteFollowerInboxes(ctx, actor.ID)
	if err != nil {
		slog.Warn("federation: move inbox expansion failed", "error", err)
		return nil
	}
	if len(inboxes) == 0 {
		return nil
	}
	apID := fmt.Sprintf("%s#move/%d", actor.ActorUrl, time.Now().Unix())
	move := map[string]any{
		"@context": asContext,
		"id":       apID,
		"type":     "Move",
		"actor":    actor.ActorUrl,
		"object":   actor.ActorUrl,
		"target":   t,
	}
	if _, err := s.insertDeliveries(ctx, actor.ID, move, apID, inboxes, true); err != nil {
		slog.Warn("federation: move fanout failed — redirect stays stamped", "error", err)
	}
	return nil
}
