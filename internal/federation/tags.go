package federation

// Ref: japaconnect-engineering/sections/126-activitypub-federation.md
// (M5 — Tag discovery). Three mechanisms combine:
//   * ?tag= on the remote timeline filters stored posts by hashtag.
//   * federation_tag_subscriptions rows are a second ingest warrant —
//     matching posts land even from actors nobody follows (relay and
//     Follow{Hashtag} traffic), and act as the per-member predicate in
//     the timeline query.
//   * GET /me/federation/search fans out to Mastodon-style public tag
//     timelines on the member's candidate domains — live preview,
//     nothing persists.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// ErrTagSubscriptionLimit — member is at federation.tag_subscription_limit.
var ErrTagSubscriptionLimit = errors.New("federation: tag subscription limit reached")

// tagRe bounds subscription tags — lowercase ASCII like our own tags.
var tagRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

// normalizeTagParam strips '#' and lowercases a caller-supplied tag —
// filter inputs need no strict validation (a non-tag just matches
// nothing); subscription inputs must additionally pass tagRe.
func normalizeTagParam(raw string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
}

// TagSubscription is the member-facing view of one ingest-warrant row.
type TagSubscription struct {
	ID        pgtype.UUID `json:"id"`
	Tag       string      `json:"tag"`
	Domain    string      `json:"domain"`
	CreatedAt time.Time   `json:"created_at"`
}

func tagSubView(r gen.FederationTagSubscription) TagSubscription {
	return TagSubscription{ID: r.ID, Tag: r.Tag, Domain: r.Domain, CreatedAt: r.CreatedAt.Time}
}

// ListTagSubscriptions serves GET /me/federation/tag-subscriptions.
func (s *Service) ListTagSubscriptions(ctx context.Context, userID pgtype.UUID) ([]TagSubscription, error) {
	if _, err := s.memberActor(ctx, userID); err != nil {
		return nil, err
	}
	rows, err := s.store.read.ListTagSubscriptionsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]TagSubscription, 0, len(rows))
	for _, r := range rows {
		out = append(out, tagSubView(r))
	}
	return out, nil
}

// SubscribeTag drives POST /me/federation/tag-subscriptions. The row is
// the ingest warrant; a domain-scoped sub additionally ledgers a signed
// Follow{Hashtag} from the instance actor to that instance's inbox —
// servers that honor hashtag follows then Announce matching posts at us.
func (s *Service) SubscribeTag(ctx context.Context, userID pgtype.UUID, rawTag, rawDomain string) (TagSubscription, error) {
	tag := normalizeTagParam(rawTag)
	if !tagRe.MatchString(tag) {
		return TagSubscription{}, ErrInvalidTarget
	}
	domain := strings.ToLower(strings.TrimSpace(rawDomain))
	if domain != "" {
		if strings.ContainsAny(domain, " \t\r\n/?#@") || !validHost(domain) {
			return TagSubscription{}, ErrInvalidDomain
		}
		if !s.instanceAllowed(ctx, domain) {
			return TagSubscription{}, ErrRemoteBlocked
		}
	}
	if _, err := s.memberActor(ctx, userID); err != nil {
		return TagSubscription{}, err
	}
	limit := s.intSetting(ctx, "federation.tag_subscription_limit", 25)
	if n, err := s.store.read.CountTagSubscriptionsByUser(ctx, userID); err == nil && int(n) >= limit {
		return TagSubscription{}, ErrTagSubscriptionLimit
	}
	// Domain-scoped subs deliver Follow{Hashtag} — mint the activity id
	// up front so the row ledgers it for the Undo on unsubscribe.
	var inst gen.FederatedActor
	var followID *string
	var inbox string
	var remoteID pgtype.UUID
	if domain != "" {
		var err error
		inst, err = s.ensureInstanceActor(ctx)
		if err != nil {
			return TagSubscription{}, err
		}
		id := fmt.Sprintf("%s#tag-follow-%s", inst.ActorUrl, uuid.NewString())
		followID = &id
		if row, rerr := s.store.read.GetSharedInboxByDomain(ctx, domain); rerr == nil && row.Inbox != nil {
			inbox = *row.Inbox
			remoteID = row.RemoteActorID
		} else {
			inbox = "https://" + domain + "/inbox" // Mastodon convention
		}
	}
	row, err := s.store.write.InsertTagSubscription(ctx, gen.InsertTagSubscriptionParams{
		UserID: userID, Tag: tag, Domain: domain, FollowApID: followID,
	})
	if err != nil {
		return TagSubscription{}, fmt.Errorf("federation: tag subscription: %w", err)
	}
	if domain != "" && followID != nil {
		follow := map[string]any{
			"@context": asContext,
			"id":       *followID,
			"type":     "Follow",
			"actor":    inst.ActorUrl,
			"object": map[string]any{
				"type": "Hashtag",
				"id":   fmt.Sprintf("https://%s/tags/%s", domain, tag),
				"name": "#" + tag,
			},
			"to": []string{fmt.Sprintf("https://%s/tags/%s", domain, tag)},
		}
		rows := []gen.ListRemoteFollowerInboxesByActorRow{{InboxUrl: &inbox, RemoteActorID: remoteID}}
		if _, derr := s.insertDeliveries(ctx, inst.ID, follow, *followID, rows, true); derr != nil {
			slog.Warn("federation: tag follow delivery failed — row kept", "error", derr)
		}
	}
	return tagSubView(row), nil
}

// UnsubscribeTag drives DELETE /me/federation/tag-subscriptions/{id} —
// Undo{Follow} to the ledgered inbox (domain-scoped subs), then row delete.
func (s *Service) UnsubscribeTag(ctx context.Context, userID, subID pgtype.UUID) error {
	if _, err := s.memberActor(ctx, userID); err != nil {
		return err
	}
	row, err := s.store.read.GetTagSubscriptionByID(ctx, gen.GetTagSubscriptionByIDParams{
		ID: subID, UserID: userID,
	})
	if err != nil {
		return ErrNotFound
	}
	if row.Domain != "" && row.FollowApID != nil && *row.FollowApID != "" {
		inst, ierr := s.ensureInstanceActor(ctx)
		if ierr != nil {
			slog.Warn("federation: instance actor unavailable — tag Undo skipped", "error", ierr)
		} else {
			tagURI := fmt.Sprintf("https://%s/tags/%s", row.Domain, row.Tag)
			follow := map[string]any{
				"id":   *row.FollowApID,
				"type": "Follow",
				"actor": inst.ActorUrl,
				"object": map[string]any{
					"type": "Hashtag",
					"id":   tagURI,
					"name": "#" + row.Tag,
				},
			}
			undoID := fmt.Sprintf("%s#tag-unfollow-%s", inst.ActorUrl, uuid.NewString())
			undo := map[string]any{
				"id":     undoID,
				"type":   "Undo",
				"actor":  inst.ActorUrl,
				"object": follow,
				"to":     []string{tagURI},
			}
			inbox := "https://" + row.Domain + "/inbox"
			var remoteID pgtype.UUID
			if r, rerr := s.store.read.GetSharedInboxByDomain(ctx, row.Domain); rerr == nil && r.Inbox != nil {
				inbox = *r.Inbox
				remoteID = r.RemoteActorID
			}
			rows := []gen.ListRemoteFollowerInboxesByActorRow{{InboxUrl: &inbox, RemoteActorID: remoteID}}
			if _, derr := s.insertDeliveries(ctx, inst.ID, undo, undoID, rows, true); derr != nil {
				slog.Warn("federation: tag undo delivery failed — row still removed", "error", derr)
			}
		}
	}
	n, err := s.store.write.DeleteTagSubscription(ctx, gen.DeleteTagSubscriptionParams{
		ID: subID, UserID: userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// validHost reports whether s parses as a bare hostname (optionally with
// a port) — no scheme, path, userinfo, or whitespace.
func validHost(s string) bool {
	if u, err := url.Parse("https://" + s); err != nil || u.Host != s {
		return false
	}
	return !strings.ContainsAny(s, "/@")
}

// ---------------------------------------------------------------------------
// Live remote tag search
// ---------------------------------------------------------------------------

// RemoteSearchResult is the GET /me/federation/search payload: live items
// normalized to the timeline shape plus per-domain failures.
type RemoteSearchResult struct {
	Tag     string              `json:"tag"`
	Domains []string            `json:"domains"`
	Items   []RemoteTimelineItem `json:"items"`
	Errors  []RemoteSearchError `json:"errors"`
}

type RemoteSearchError struct {
	Domain  string `json:"domain"`
	Message string `json:"message"`
}

// RemoteTagSearch fans out to Mastodon-style public tag timelines on the
// member's candidate domains — ?domain= pins to one instance; else the
// union of subscription domains, followed-actor domains and accepted
// relays, capped by federation.remote_search_domain_limit.
func (s *Service) RemoteTagSearch(ctx context.Context, userID pgtype.UUID, rawTag, rawDomain string) (RemoteSearchResult, error) {
	tag := normalizeTagParam(rawTag)
	if !tagRe.MatchString(tag) {
		return RemoteSearchResult{}, ErrInvalidTarget
	}
	if _, err := s.memberActor(ctx, userID); err != nil {
		return RemoteSearchResult{}, err
	}
	var domains []string
	if d := strings.ToLower(strings.TrimSpace(rawDomain)); d != "" {
		if !validHost(d) {
			return RemoteSearchResult{}, ErrInvalidDomain
		}
		domains = []string{d}
	} else {
		rows, err := s.store.read.ListRemoteSearchDomains(ctx, gen.ListRemoteSearchDomainsParams{
			UserID:      userID,
			DomainLimit: int32(s.intSetting(ctx, "federation.remote_search_domain_limit", 5)),
		})
		if err != nil {
			return RemoteSearchResult{}, fmt.Errorf("federation: search domains: %w", err)
		}
		for _, r := range rows {
			domains = append(domains, r)
		}
	}
	res := RemoteSearchResult{Tag: tag, Domains: []string{}, Items: []RemoteTimelineItem{}, Errors: []RemoteSearchError{}}
	allowed := make([]string, 0, len(domains))
	for _, d := range domains {
		if s.instanceAllowed(ctx, d) {
			allowed = append(allowed, d)
		} else if rawDomain != "" {
			// A pinned domain that's blocked would otherwise degrade to an
			// indistinguishable "no results" — surface it as a domain error.
			res.Errors = append(res.Errors, RemoteSearchError{Domain: d, Message: "instance is blocked"})
		}
	}
	timeout := time.Duration(s.intSetting(ctx, "federation.remote_search_timeout_ms", 6000)) * time.Millisecond

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, d := range allowed {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			dctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			items, err := s.fetchTagTimeline(dctx, domain, tag)
			mu.Lock()
			defer mu.Unlock()
			res.Domains = append(res.Domains, domain)
			if err != nil {
				res.Errors = append(res.Errors, RemoteSearchError{Domain: domain, Message: err.Error()})
				return
			}
			res.Items = append(res.Items, items...)
		}(d)
	}
	wg.Wait()
	return res, nil
}

// mastodonStatus mirrors the fields of a Mastodon /api/v1/timelines/tag
// status we normalize — remote-advertised data, never trusted markup.
type mastodonStatus struct {
	ID          string `json:"id"`
	URI         string `json:"uri"`
	URL         string `json:"url"`
	Content     string `json:"content"`
	SpoilerText string `json:"spoiler_text"`
	Sensitive   bool   `json:"sensitive"`
	CreatedAt   string `json:"created_at"`
	Reblogs     int32  `json:"reblogs_count"`
	Tags        []struct {
		Name string `json:"name"`
	} `json:"tags"`
	Account struct {
		Acct        string `json:"acct"`
		URL         string `json:"url"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Avatar      string `json:"avatar"`
	} `json:"account"`
}

// fetchTagTimeline GETs one instance's Mastodon-style public tag
// timeline through the SSRF-guarded client and normalizes statuses.
func (s *Service) fetchTagTimeline(ctx context.Context, domain, tag string) ([]RemoteTimelineItem, error) {
	raw := fmt.Sprintf("https://%s/api/v1/timelines/tag/%s?limit=10", domain, url.PathEscape(tag))
	body, err := s.fetchAP(ctx, raw)
	if err != nil {
		return nil, err
	}
	var statuses []mastodonStatus
	if err := json.Unmarshal(body, &statuses); err != nil {
		return nil, fmt.Errorf("bad status payload: %w", err)
	}
	items := make([]RemoteTimelineItem, 0, len(statuses))
	for _, st := range statuses {
		text := htmlToText(st.Content)
		if text == "" {
			continue
		}
		item := RemoteTimelineItem{
			ApID:        firstNonEmpty(st.URI, st.URL),
			ContentText: text,
			Sensitive:   st.Sensitive || st.SpoilerText != "",
			ShareCount:  st.Reblogs,
			ActorDomain: domain,
			Live:        true,
			Hashtags:    []string{},
		}
		if st.URL != "" {
			u := st.URL
			item.URL = &u
		}
		if st.SpoilerText != "" {
			sp := runeCap(st.SpoilerText, maxRemotePostField)
			item.Summary = &sp
		}
		if t, terr := time.Parse(time.RFC3339, st.CreatedAt); terr == nil {
			item.PublishedAt = &t
		}
		for _, tg := range st.Tags {
			if tg.Name != "" && len(item.Hashtags) < maxRemotePostHashtags {
				item.Hashtags = append(item.Hashtags, tg.Name)
			}
		}
		acct := st.Account.Acct
		if acct != "" && !strings.Contains(acct, "@") {
			acct += "@" + domain
		}
		item.Actor.Acct = &acct
		item.Actor.ActorURL = st.Account.URL
		item.Actor.PreferredUsername = &st.Account.Username
		item.Actor.DisplayName = &st.Account.DisplayName
		item.Actor.AvatarURL = &st.Account.Avatar
		items = append(items, item)
	}
	return items, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
