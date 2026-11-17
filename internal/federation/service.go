package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	jcrypto "github.com/jedafe/japa/internal/crypto"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/features"
	"github.com/jedafe/japa/internal/rss/model"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

// Config carries the public URLs the federation surface is built from.
// Values come from internal/config (JC_API_PUBLIC_URL, JC_SITE_URL,
// JC_FEDERATION_DOMAIN) — never hardcoded (Rule 3).
type Config struct {
	// APIBase is the canonical public API origin — actor/object @id host
	// (e.g. https://api.japa.to). Unversioned /federation/* paths hang here.
	APIBase string
	// SiteBase is the apex origin serving human profiles (e.g.
	// https://japa.to).
	SiteBase string
	// HandleDomain is the acct domain for handles (e.g. japa.to). Empty →
	// derived from SiteBase's host.
	HandleDomain string
	// Version is the platform release reported in NodeInfo software.
	Version string
}

// SettingsReader is the subset of config.Settings the service needs
// (injected via SetSettings — optional, tests may leave nil).
type SettingsReader interface {
	GetSetting(ctx context.Context, key string) (string, error)
	GetSettingBool(ctx context.Context, key string) (bool, error)
	GetSettingInt(ctx context.Context, key string) (int, error)
}

// Service implements the §126 federation surface.
type Service struct {
	store    *Store
	db       *db.DB
	cfg      Config
	settings SettingsReader
	vk       valkeycompat.Cmdable
	flags    features.FlagEvaluator
	notifier Notifier
	enqueue  TaskEnqueuer
	spam     SpamChecker
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, cfg Config) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer)),
		db:    database,
		cfg:   cfg,
	}
}

// SetSettings wires the hot-reloadable system settings reader.
func (s *Service) SetSettings(settings SettingsReader) { s.settings = settings }

// SetValkey wires the Valkey client for webfinger/actor doc caching.
// Optional — nil disables caching (tests/development).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// SetFlags wires the feature-flag evaluator used to prune flag-gated
// content types (§42) from federation projections. Optional — nil serves
// the full federatable allowlist.
func (s *Service) SetFlags(c features.FlagEvaluator) { s.flags = c }

// federatableTypesFor returns the allowlist minus content types whose
// feature flag is unreachable for this caller (discussion/forum_thread
// die with feature.forum, story with story.enabled, blog_post with
// feature.blog) — the same pruning RSS and search apply.
func (s *Service) federatableTypesFor(ctx context.Context) []string {
	gated := features.GatedContentTypes(ctx, s.flags)
	if len(gated) == 0 {
		return federatableTypes
	}
	hide := make(map[string]bool, len(gated))
	for _, ct := range gated {
		hide[ct] = true
	}
	out := make([]string, 0, len(federatableTypes))
	for _, ct := range federatableTypes {
		if !hide[ct] {
			out = append(out, ct)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// URL helpers
// ---------------------------------------------------------------------------

func (s *Service) apiBase() string  { return strings.TrimRight(s.cfg.APIBase, "/") }
func (s *Service) siteBase() string { return strings.TrimRight(s.cfg.SiteBase, "/") }

// handleDomain resolves the acct handle domain: configured domain wins,
// else the SiteBase host. An empty result means misconfiguration — callers
// fall back to the request host (local dev).
func (s *Service) handleDomain() string {
	if s.cfg.HandleDomain != "" {
		return s.cfg.HandleDomain
	}
	if u, err := url.Parse(s.cfg.SiteBase); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return ""
}

func (s *Service) actorURL(username string) string {
	return s.apiBase() + "/federation/actors/" + username
}

func (s *Service) objectURL(contentID string) string {
	return s.apiBase() + "/federation/objects/contents/" + contentID
}

func (s *Service) profileURL(username string) string {
	return s.siteBase() + "/u/" + username
}

// subscribeTemplateURL is the OStatus remote-follow template the JRD
// advertises — the apex /authorize_interaction page bounces the visitor
// to their own instance's remote-follow UI with our actor as the target.
// The literal {uri} placeholder is the spec substitution point — it must
// NOT be escaped.
func (s *Service) subscribeTemplateURL() string {
	if b := s.siteBase(); b != "" {
		return b + "/authorize_interaction?uri={uri}"
	}
	return ""
}

func (s *Service) outboxPageSize(ctx context.Context) int32 {
	const fallback = 20
	if s.settings == nil {
		return fallback
	}
	if n, err := s.settings.GetSettingInt(ctx, "federation.outbox_page_size"); err == nil && n > 0 && n <= 200 {
		return int32(n)
	}
	return fallback
}

// ---------------------------------------------------------------------------
// WebFinger
// ---------------------------------------------------------------------------

// acctResourceRe accepts the person localpart plus the M3 forms: the
// '!'-prefixed Group handle (Mastodon group convention) and the bare
// handle domain (instance actor, acct:japa.to@japa.to).
var acctResourceRe = regexp.MustCompile(`^acct:(!?[A-Za-z0-9_\-.]{1,64})@([A-Za-z0-9.\-]+)$`)

// Webfinger resolves an acct: resource to a JRD. Only the configured
// handle domain is served; other domains return ErrHandleDomain.
func (s *Service) Webfinger(ctx context.Context, resource string) (map[string]any, error) {
	m := acctResourceRe.FindStringSubmatch(resource)
	if m == nil {
		return nil, ErrHandleDomain
	}
	username, domain := m[1], m[2]

	if hd := s.handleDomain(); hd != "" && !strings.EqualFold(domain, hd) {
		return nil, ErrHandleDomain
	}

	// M3 non-person handles resolve before the person path: a '!'-prefixed
	// localpart names a space's Group actor; the bare handle domain names
	// the instance Application actor.
	if strings.HasPrefix(username, "!") {
		return s.groupWebfinger(ctx, username[1:], domain)
	}
	if hd := s.handleDomain(); hd != "" && strings.EqualFold(username, hd) {
		return s.instanceWebfinger(ctx, domain)
	}

	// Unconfigured handle domain (dev fallback) serves ANY acct domain and
	// embeds the request domain in the subject — caching under the bare
	// username would poison every other domain's lookup, so only the
	// configured-domain path uses the cache.
	cacheable := s.handleDomain() != ""
	cacheKey := "jc:fed:webfinger:" + strings.ToLower(username)
	if cacheable {
		if doc, ok := s.cacheGet(ctx, cacheKey); ok {
			return doc, nil
		}
	}

	// The profile query carries the live users.username — the JRD subject
	// echoes it, not the (possibly stale) preferred_username snapshot, so a
	// renamed member's old handle resolves to their current acct address.
	profile, err := s.store.actorProfile(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: webfinger lookup: %w", err)
	}

	// Subject echoes the canonical handle domain, not the (possibly
	// differently-cased) request domain; fall back to the request domain
	// only when no handle domain is configured.
	canonical := s.handleDomain()
	if canonical == "" {
		canonical = domain
	}
	doc := webfingerJRD(
		profile.Username, canonical, profile.ActorUrl,
		s.profileURL(profile.Username), s.subscribeTemplateURL(),
	)
	if cacheable {
		s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
	}
	return doc, nil
}

// groupWebfinger serves acct:!{slug}@{domain} — the Group actor JRD for
// a federated space. Group handles only resolve while the space is
// public/active and the actor is live (same gates as the doc route —
// the envelope join runs live on every request; the cache only elides
// the JRD build).
func (s *Service) groupWebfinger(ctx context.Context, slug, domain string) (map[string]any, error) {
	row, err := s.groupDocRow(ctx, slug)
	if err != nil {
		return nil, err
	}
	if row.Status != ActorStatusActive {
		return nil, ErrNotFound
	}
	cacheable := s.handleDomain() != ""
	cacheKey := "jc:fed:webfinger:!" + strings.ToLower(slug)
	if cacheable {
		if doc, ok := s.cacheGet(ctx, cacheKey); ok {
			return doc, nil
		}
	}
	canonical := s.handleDomain()
	if canonical == "" {
		canonical = domain
	}
	// Subject echoes the actor's provision-time handle (immutable, like
	// actor_url) — a space slug rename must not make acct:!{oldslug}
	// answer with an unresolvable new handle. The profile link tracks
	// the live slug.
	doc := webfingerJRD(
		"!"+row.PreferredUsername, canonical, row.ActorUrl,
		s.siteBase()+"/spaces/"+row.SpaceSlug, s.subscribeTemplateURL(),
	)
	if cacheable {
		s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
	}
	return doc, nil
}

// instanceWebfinger serves acct:{domain}@{domain} — the instance
// Application actor JRD.
func (s *Service) instanceWebfinger(ctx context.Context, domain string) (map[string]any, error) {
	cacheable := s.handleDomain() != ""
	cacheKey := "jc:fed:webfinger:" + strings.ToLower(s.handleDomain())
	if cacheable {
		if doc, ok := s.cacheGet(ctx, cacheKey); ok {
			return doc, nil
		}
	}
	actor, err := s.ensureInstanceActor(ctx)
	if err != nil {
		return nil, err
	}
	if actor.Status != ActorStatusActive {
		return nil, ErrNotFound
	}
	canonical := s.handleDomain()
	if canonical == "" {
		canonical = domain
	}
	doc := webfingerJRD(canonical, canonical, actor.ActorUrl, s.siteBase(), s.subscribeTemplateURL())
	if cacheable {
		s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
	}
	return doc, nil
}

// ---------------------------------------------------------------------------
// Actor + collections
// ---------------------------------------------------------------------------

// ActorDocument returns the Person document for a federated username.
func (s *Service) ActorDocument(ctx context.Context, username string) (map[string]any, error) {
	cacheKey := "jc:fed:actor:" + strings.ToLower(username)
	if doc, ok := s.cacheGet(ctx, cacheKey); ok {
		return doc, nil
	}

	profile, err := s.store.actorProfile(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: actor lookup: %w", err)
	}

	doc := actorDocument(profile, s.siteBase())
	s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
	return doc, nil
}

// Outbox returns the OrderedCollection (page==0) or OrderedCollectionPage
// of Create activities for the actor's federatable contents.
func (s *Service) Outbox(ctx context.Context, username string, page int) (map[string]any, error) {
	actor, err := s.store.actorByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: outbox actor: %w", err)
	}

	total, err := s.store.countFederatableContent(ctx, actor.UserID, s.federatableTypesFor(ctx))
	if err != nil {
		return nil, fmt.Errorf("federation: outbox count: %w", err)
	}

	if page <= 0 {
		return orderedCollection(actor.OutboxUrl, total, actor.OutboxUrl+"?page=1"), nil
	}

	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size

	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", actor.OutboxUrl, p) }
	// Past-the-end pages answer an empty page without scanning — remote
	// crawlers walk next-links until items dry up, so the bound matters.
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), actor.OutboxUrl, []any{}, total, false, page > 1, pageID, page), nil
	}

	rows, err := s.store.federatableContent(ctx, actor.UserID, s.federatableTypesFor(ctx), size, offset)
	if err != nil {
		return nil, fmt.Errorf("federation: outbox page: %w", err)
	}

	items := make([]any, 0, len(rows))
	for _, row := range rows {
		obj := s.objectFromListRow(ctx, row, actor)
		items = append(items, createActivity(actor.ActorUrl, actor.FollowersUrl, obj))
	}

	return orderedCollectionPage(pageID(page), actor.OutboxUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// Featured returns the §126 M4 featured collection — the member's pinned
// federatable contents as an unpaginated OrderedCollection with the
// object documents embedded (the Mastodon /collections/featured shape
// remote profile surfaces walk). totalItems counts exactly the rows
// listed: the collection is bounded by outbox_page_size and never
// advertises items it does not show.
func (s *Service) Featured(ctx context.Context, username string) (map[string]any, error) {
	actor, err := s.store.actorByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: featured actor: %w", err)
	}

	rows, err := s.store.featuredContent(ctx, actor.UserID, s.federatableTypesFor(ctx), s.outboxPageSize(ctx))
	if err != nil {
		return nil, fmt.Errorf("federation: featured items: %w", err)
	}

	items := make([]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.objectFromFeaturedRow(ctx, row, actor))
	}

	return map[string]any{
		"@context":     asContext[0],
		"id":           actor.ActorUrl + "/collections/featured",
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	}, nil
}

// FeaturedTags returns the §126 M4 featuredTags collection — the hashtags
// the member uses across their federatable contents as Hashtag items.
func (s *Service) FeaturedTags(ctx context.Context, username string) (map[string]any, error) {
	actor, err := s.store.actorByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: featuredTags actor: %w", err)
	}

	tags, err := s.store.actorFederatedTags(ctx, actor.UserID, s.federatableTypesFor(ctx), s.outboxPageSize(ctx))
	if err != nil {
		return nil, fmt.Errorf("federation: featuredTags items: %w", err)
	}

	items := make([]any, 0, len(tags))
	for _, t := range tags {
		items = append(items, map[string]any{
			"type": "Hashtag",
			"name": "#" + t.Name,
			"href": s.siteBase() + "/tags/" + t.Slug,
		})
	}

	return map[string]any{
		"@context":     asContext[0],
		"id":           actor.ActorUrl + "/collections/tags",
		"type":         "OrderedCollection",
		"totalItems":   len(items),
		"orderedItems": items,
	}, nil
}

// Followers returns the OrderedCollection(+Page) of follower actor URLs:
// remote accepted follows plus local followers who are federated.
func (s *Service) Followers(ctx context.Context, username string, page int) (map[string]any, error) {
	actor, err := s.store.actorByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: followers actor: %w", err)
	}

	remote, err := s.store.countRemoteFollowers(ctx, actor.ID)
	if err != nil {
		return nil, fmt.Errorf("federation: followers count: %w", err)
	}
	local, err := s.store.countLocalFollowers(ctx, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("federation: followers local count: %w", err)
	}
	total := remote + local

	if page <= 0 {
		return orderedCollection(actor.FollowersUrl, total, actor.FollowersUrl+"?page=1"), nil
	}

	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size

	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", actor.FollowersUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), actor.FollowersUrl, []any{}, total, false, page > 1, pageID, page), nil
	}

	// Remote followers fill the first pages; local federated followers follow.
	items := make([]any, 0, size)
	if int64(offset) < remote {
		urls, err := s.store.remoteFollowerURLs(ctx, actor.ID, size, offset)
		if err != nil {
			return nil, fmt.Errorf("federation: remote followers: %w", err)
		}
		for _, u := range urls {
			items = append(items, u)
		}
	}
	if int32(len(items)) < size {
		localOff := int32(0)
		if int64(offset) > remote {
			localOff = offset - int32(remote)
		}
		urls, err := s.store.localFollowerURLs(ctx, actor.UserID, size-int32(len(items)), localOff)
		if err != nil {
			return nil, fmt.Errorf("federation: local followers: %w", err)
		}
		for _, u := range urls {
			items = append(items, u)
		}
	}

	return orderedCollectionPage(pageID(page), actor.FollowersUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// Following returns the OrderedCollection(+Page) of actor URLs the member
// follows that are themselves federated.
func (s *Service) Following(ctx context.Context, username string, page int) (map[string]any, error) {
	actor, err := s.store.actorByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: following actor: %w", err)
	}

	// M5: the collection unions remote accepted outbound edges (remote
	// actor URLs first — mirrors the followers collection's remote→local
	// ordering) with the local-federated following set.
	remoteTotal, err := s.store.read.CountRemoteFollowingByActor(ctx, actor.ID)
	if err != nil {
		return nil, fmt.Errorf("federation: remote following count: %w", err)
	}
	localTotal, err := s.store.countLocalFollowing(ctx, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("federation: following count: %w", err)
	}
	total := remoteTotal + localTotal

	if page <= 0 {
		return orderedCollection(actor.FollowingUrl, total, actor.FollowingUrl+"?page=1"), nil
	}

	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size

	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", actor.FollowingUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), actor.FollowingUrl, []any{}, total, false, page > 1, pageID, page), nil
	}

	var urls []string
	if int64(offset) < remoteTotal {
		take := int64(size)
		if rem := remoteTotal - int64(offset); rem < take {
			take = rem
		}
		urls, err = s.store.read.ListRemoteFollowingURLsByActor(ctx, gen.ListRemoteFollowingURLsByActorParams{
			LocalActorID: actor.ID, PageLimit: int32(take), PageOffset: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("federation: remote following page: %w", err)
		}
		if rem := int64(size) - int64(len(urls)); rem > 0 {
			local, lerr := s.store.localFollowingURLs(ctx, actor.UserID, int32(rem), 0)
			if lerr != nil {
				return nil, fmt.Errorf("federation: following page: %w", lerr)
			}
			urls = append(urls, local...)
		}
	} else {
		urls, err = s.store.localFollowingURLs(ctx, actor.UserID, size, offset-int32(remoteTotal))
		if err != nil {
			return nil, fmt.Errorf("federation: following page: %w", err)
		}
	}
	items := make([]any, 0, len(urls))
	for _, u := range urls {
		items = append(items, u)
	}

	return orderedCollectionPage(pageID(page), actor.FollowingUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
}

// ObjectDocument returns the Note/Article for a federatable contents id —
// served only while the author holds an active federated actor.
func (s *Service) ObjectDocument(ctx context.Context, contentID pgtype.UUID) (map[string]any, error) {
	row, err := s.store.federatableContentByID(ctx, contentID, s.federatableTypesFor(ctx))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A deleted or once-published PUBLIC row gets 410
			// Tombstone, not 404 — remotes distinguish "gone" from
			// "never existed" and drop their cached copy. Non-public
			// rows were never remotely addressable: 404 must keep
			// hiding their existence entirely. published_at is the
			// durable was-live signal (it survives unpublish).
			if lite, lerr := s.store.contentLiteByID(ctx, contentID); lerr == nil &&
				lite.Visibility == "public" &&
				(lite.DeletedAt.Valid || lite.PublishedAt.Valid) {
				return nil, ErrGone
			}
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: object lookup: %w", err)
	}

	actor, err := s.store.actorByUserID(ctx, row.AuthorID)
	if err == nil && actor.Status == "tombstone" {
		return nil, ErrGone // erased actor — its objects are gone for good
	}
	if err != nil || actor.Status != ActorStatusActive {
		return nil, ErrNotFound
	}

	obj := s.objectFromGetRow(ctx, row, actor)
	obj["@context"] = asContext
	return obj, nil
}

// objectFromListRow maps an outbox row to its AP object document.
func (s *Service) objectFromListRow(ctx context.Context, row gen.ListFederatableContentByAuthorRow, actor gen.FederatedActor) map[string]any {
	obj := objectDocument(gen.GetFederatableContentByIDRow{
		ID:             row.ID,
		ContentType:    row.ContentType,
		Title:          row.Title,
		Slug:           row.Slug,
		BodyHtml:       row.BodyHtml,
		PublishedAt:    row.PublishedAt,
		UpdatedAt:      row.UpdatedAt,
		FeaturedBucket: row.FeaturedBucket,
		FeaturedPath:   row.FeaturedPath,
		FeaturedMime:   row.FeaturedMime,
		HeaderBucket:   row.HeaderBucket,
		HeaderPath:     row.HeaderPath,
		HeaderMime:     row.HeaderMime,
	}, actor.ActorUrl, actor.FollowersUrl, s.siteBase(), s.pageURL(row.ContentType, row.Slug, row.CanonicalUrl))
	s.decorateTags(ctx, obj, row.ID)
	return obj
}

func (s *Service) objectFromGetRow(ctx context.Context, row gen.GetFederatableContentByIDRow, actor gen.FederatedActor) map[string]any {
	obj := objectDocument(row, actor.ActorUrl, actor.FollowersUrl, s.siteBase(), s.pageURL(row.ContentType, row.Slug, row.CanonicalUrl))
	s.decorateTags(ctx, obj, row.ID)
	return obj
}

// objectFromFeaturedRow renders a pinned content row for the featured
// collection — same objectDocument shape as the outbox items.
func (s *Service) objectFromFeaturedRow(ctx context.Context, row gen.ListFeaturedFederatableContentByAuthorRow, actor gen.FederatedActor) map[string]any {
	return s.objectFromGetRow(ctx, gen.GetFederatableContentByIDRow{
		ID:             row.ID,
		ContentType:    row.ContentType,
		Title:          row.Title,
		Slug:           row.Slug,
		BodyHtml:       row.BodyHtml,
		CanonicalUrl:   row.CanonicalUrl,
		PublishedAt:    row.PublishedAt,
		UpdatedAt:      row.UpdatedAt,
		FeaturedBucket: row.FeaturedBucket,
		FeaturedPath:   row.FeaturedPath,
		FeaturedMime:   row.FeaturedMime,
		HeaderBucket:   row.HeaderBucket,
		HeaderPath:     row.HeaderPath,
		HeaderMime:     row.HeaderMime,
	}, actor)
}

// pageURL resolves the human-facing URL for a contents row: the editorial
// canonical_url when set, else the content-type route path. A relative
// canonical ("/blog/x") is anchored to siteBase — AP urls must be absolute.
func (s *Service) pageURL(contentType, slug string, canonical *string) string {
	if canonical != nil && *canonical != "" {
		c := *canonical
		if strings.HasPrefix(c, "http://") || strings.HasPrefix(c, "https://") {
			return c
		}
		return s.siteBase() + "/" + strings.TrimPrefix(c, "/")
	}
	return s.siteBase() + model.LinkPathFor(contentType) + slug
}

// ---------------------------------------------------------------------------
// NodeInfo
// ---------------------------------------------------------------------------

// WellKnownNodeInfo returns the discovery doc linking the 2.1 endpoint.
func (s *Service) WellKnownNodeInfo() map[string]any {
	return wellKnownNodeInfo(s.apiBase() + "/federation/nodeinfo/2.1")
}

// NodeInfo returns the 2.1 stats document.
func (s *Service) NodeInfo(ctx context.Context) (map[string]any, error) {
	usage, err := s.store.nodeInfoUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("federation: nodeinfo usage: %w", err)
	}

	open := true
	if s.settings != nil {
		if v, serr := s.settings.GetSettingBool(ctx, "general.registration_open"); serr == nil {
			open = v
		}
	}

	version := s.cfg.Version
	if version == "" {
		version = "1.0.0"
	}
	return nodeInfoDocument(version, s.siteBase(), usage, open), nil
}

// ---------------------------------------------------------------------------
// Member opt-in
// ---------------------------------------------------------------------------

// FederationStatus is the member-facing view of their actor state.
type FederationStatus struct {
	Enabled       bool   `json:"enabled"`
	Status        string `json:"status"`
	ActorURL      string `json:"actor_url,omitempty"`
	Handle        string `json:"handle,omitempty"`
	FollowerCount int64  `json:"follower_count"`
	PublicKey     string `json:"-"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// GetMyFederation returns the caller's federation state.
func (s *Service) GetMyFederation(ctx context.Context, userID pgtype.UUID) (FederationStatus, error) {
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FederationStatus{Enabled: false, Status: "none"}, nil
		}
		return FederationStatus{}, fmt.Errorf("federation: my actor: %w", err)
	}
	// A tombstone (M2 erasure) reads as "no federation" to the member —
	// the erased identity is gone, not merely disabled.
	if actor.Status == ActorStatusTombstone {
		return FederationStatus{Enabled: false, Status: "none"}, nil
	}
	user, err := s.store.userByID(ctx, userID)
	if err != nil {
		return FederationStatus{}, fmt.Errorf("federation: my user: %w", err)
	}
	return s.statusFor(ctx, actor, user.Username), nil
}

// statusFor builds the member-facing actor view. The handle uses the live
// username — preferred_username is a creation-time snapshot that can lag
// a rename. follower_count mirrors the followers collection total (remote
// accepted + local federated); a count error degrades to 0 rather than
// failing the status read.
func (s *Service) statusFor(ctx context.Context, actor gen.FederatedActor, username string) FederationStatus {
	st := FederationStatus{
		Enabled:  actor.Status == ActorStatusActive,
		Status:   actor.Status,
		ActorURL: actor.ActorUrl,
		Handle:   username + "@" + s.handleDomain(),
	}
	if s.store != nil {
		if remote, err := s.store.countRemoteFollowers(ctx, actor.ID); err == nil {
			st.FollowerCount += remote
		}
		if local, err := s.store.countLocalFollowers(ctx, actor.UserID); err == nil {
			st.FollowerCount += local
		}
	}
	if actor.CreatedAt.Valid {
		st.CreatedAt = actor.CreatedAt.Time.UTC().Format(time.RFC3339)
	}
	return st
}

// EnableFederation mints (or reactivates) the caller's Person actor.
// Keypair generation + row insert happen together; the private key is
// stored only as internal/crypto ciphertext.
func (s *Service) EnableFederation(ctx context.Context, userID pgtype.UUID) (FederationStatus, error) {
	user, err := s.store.userByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FederationStatus{}, ErrNotFound
		}
		return FederationStatus{}, fmt.Errorf("federation: lookup user: %w", err)
	}
	if user.Status != "active" {
		return FederationStatus{}, ErrNotFound
	}
	username := user.Username

	existing, err := s.store.actorByUserID(ctx, userID)
	if err == nil {
		switch existing.Status {
		case ActorStatusActive:
			// Idempotent no-op.
		case ActorStatusDisabled:
			// Conditional transition — if a concurrent write flipped the
			// status (e.g. an M2 erasure tombstoned between our read and
			// this write), we must not clobber it back.
			n, uerr := s.store.setActorStatusFrom(ctx, existing.ID, ActorStatusActive, ActorStatusDisabled)
			if uerr != nil {
				return FederationStatus{}, fmt.Errorf("federation: reactivate: %w", uerr)
			}
			if n == 0 {
				cur, lerr := s.store.actorByUserID(ctx, userID)
				switch {
				case errors.Is(lerr, pgx.ErrNoRows):
					// Member hard-deleted mid-flight — the row is gone.
					return FederationStatus{}, ErrNotFound
				case lerr != nil:
					return FederationStatus{}, fmt.Errorf("federation: re-check actor: %w", lerr)
				case cur.Status == ActorStatusTombstone:
					return FederationStatus{}, ErrNotFound
				default:
					// active (concurrent enable won) or disabled (a
					// disable re-applied after an enable) — return the
					// honest current state.
					return s.statusFor(ctx, cur, username), nil
				}
			}
			existing.Status = ActorStatusActive
			s.invalidateActorCache(ctx, existing.PreferredUsername, username)
		default:
			// A tombstone row (M2 erasure) is terminal — re-opting-in must
			// never resurrect an erased identity.
			return FederationStatus{}, ErrNotFound
		}
		return s.statusFor(ctx, existing, username), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return FederationStatus{}, fmt.Errorf("federation: lookup actor: %w", err)
	}

	pubPEM, privPEM, err := generateActorKeyPair()
	if err != nil {
		return FederationStatus{}, err
	}
	privEnc, err := jcrypto.Encrypt(privPEM)
	if err != nil {
		return FederationStatus{}, fmt.Errorf("federation: encrypt key: %w", err)
	}

	actorURL := s.actorURL(username)
	actor, err := s.store.createActor(ctx, gen.CreateFederatedActorParams{
		UserID:            userID,
		ActorUrl:          actorURL,
		PreferredUsername: username,
		PublicKeyPem:      pubPEM,
		PrivateKeyEnc:     privEnc,
		PublicKeyID:       actorURL + "#main-key",
		InboxUrl:          actorURL + "/inbox",
		OutboxUrl:         actorURL + "/outbox",
		FollowersUrl:      actorURL + "/followers",
		FollowingUrl:      actorURL + "/following",
		// AP analogue of profile_visibility='followers': the actor doc
		// derives this live, the column seeds the M2 Accept/Reject path.
		ManuallyApprovesFollowers: s.approvesFollowers(ctx, userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The upsert's WHERE guard declined — either a tombstone row
			// (erasure, terminal) or a concurrent enable that already
			// wrote 'active' between our lookup and insert. Re-read to
			// tell them apart.
			if existing, lerr := s.store.actorByUserID(ctx, userID); lerr == nil && existing.Status == ActorStatusActive {
				return s.statusFor(ctx, existing, username), nil
			}
			return FederationStatus{}, ErrNotFound
		}
		return FederationStatus{}, fmt.Errorf("federation: create actor: %w", err)
	}

	slog.Info("federation actor enabled", "user_id", userID.String(), "actor_url", actorURL)
	return s.statusFor(ctx, actor, username), nil
}

// AutoProvision creates the caller's actor only when none exists — the
// §126 auto opt-in path fired by federation:auto_optin at registration.
// Unlike EnableFederation it never reactivates: an existing row of any
// status (disabled opt-out, tombstone) is left alone so a member's
// explicit opt-out can't be silently resurrected. Ineligibility is not a
// failure: a suspended/deleted member (ErrNotFound) has nothing to
// provision, so the task ends clean instead of burning retries.
func (s *Service) AutoProvision(ctx context.Context, userID pgtype.UUID) error {
	if _, err := s.store.actorByUserID(ctx, userID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("federation: auto-provision lookup: %w", err)
	}
	_, err := s.EnableFederation(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// approvesFollowers maps the member's user_privacy_settings onto the AP
// follow-approval flag: 'followers' visibility means only approved
// followers see the profile, so remote follows need approval too.
func (s *Service) approvesFollowers(ctx context.Context, userID pgtype.UUID) bool {
	privacy, err := s.store.userPrivacy(ctx, userID)
	if err != nil {
		return false
	}
	return privacy.ProfileVisibility == "followers"
}

// DisableFederation deactivates the caller's actor. The row is kept so a
// later re-enable preserves the handle; M2 additionally fans out
// Delete{actor} to remote inboxes before tombstoning.
func (s *Service) DisableFederation(ctx context.Context, userID pgtype.UUID) (FederationStatus, error) {
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FederationStatus{}, ErrNotFound
		}
		return FederationStatus{}, fmt.Errorf("federation: lookup actor: %w", err)
	}
	user, err := s.store.userByID(ctx, userID)
	if err != nil {
		return FederationStatus{}, fmt.Errorf("federation: lookup user: %w", err)
	}
	// Only an active actor deactivates: disabled is an idempotent no-op,
	// and a tombstone (M2 erasure) is terminal — flipping it back to
	// disabled would reopen the resurrection path the upsert guards.
	if actor.Status == ActorStatusTombstone {
		return FederationStatus{}, ErrNotFound
	}
	if actor.Status == ActorStatusActive {
		// Conditional transition — a concurrent write (enable, erasure)
		// that landed between our read and this write must not be
		// clobbered. Re-read on 0 rows and dispatch on the winner.
		n, err := s.store.setActorStatusFrom(ctx, actor.ID, ActorStatusDisabled, ActorStatusActive)
		if err != nil {
			return FederationStatus{}, fmt.Errorf("federation: disable: %w", err)
		}
		if n == 0 {
			cur, lerr := s.store.actorByUserID(ctx, userID)
			switch {
			case errors.Is(lerr, pgx.ErrNoRows):
				return FederationStatus{}, ErrNotFound
			case lerr != nil:
				return FederationStatus{}, fmt.Errorf("federation: re-check actor: %w", lerr)
			case cur.Status == ActorStatusTombstone:
				return FederationStatus{}, ErrNotFound
			case cur.Status == ActorStatusActive:
				// A concurrent enable won — the member's disable lost;
				// return the honest current state.
				return s.statusFor(ctx, cur, user.Username), nil
			default:
				// disabled — a concurrent disable already landed,
				// idempotent success.
				actor.Status = ActorStatusDisabled
			}
		} else {
			actor.Status = ActorStatusDisabled
		}
	}
	// Drop docs cached under both the creation-time handle and the live
	// username — a renamed member's old keys must not keep serving.
	s.invalidateActorCache(ctx, actor.PreferredUsername, user.Username)
	actor.Status = ActorStatusDisabled

	// §126 M2: tell remote followers the actor is gone. Ledgers deliveries
	// synchronously (bounded by follower count); HTTP posts ride the queue.
	// The "#delete" ap_id dedup makes a repeat disable harmless.
	if delErr := s.FanoutActorDelete(ctx, actor.ID); delErr != nil {
		slog.Warn("federation: actor delete fanout failed", "actor_id", actor.ID.String(), "error", delErr)
	}

	// Disable lapses every warrant this actor held — follows require an
	// active local actor — so orphaned remote posts get reaped.
	s.reapOrphanRemotePosts(ctx)

	return s.statusFor(ctx, actor, user.Username), nil
}

// KeyRotationResult is the rotate-key response — the new keyId the
// actor document now advertises plus the rotation stamp.
type KeyRotationResult struct {
	KeyID     string `json:"key_id"`
	RotatedAt string `json:"rotated_at"`
}

// keyRotationCooldown reads the §126 member-rotation interval
// (federation.key_rotation.cooldown_hours, default 24). Admin-triggered
// rotation bypasses it entirely — compromise response can't wait.
func (s *Service) keyRotationCooldown(ctx context.Context) time.Duration {
	if s.settings != nil {
		if n, err := s.settings.GetSettingInt(ctx, "federation.key_rotation.cooldown_hours"); err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	}
	return 24 * time.Hour
}

// RotateActorKey swaps the caller's signing keypair (§126 M3a) — member
// path, cooldown-gated. Disabled and tombstoned actors read as
// ErrNotFound: the former has nothing serving, the latter is terminal.
func (s *Service) RotateActorKey(ctx context.Context, userID pgtype.UUID) (KeyRotationResult, error) {
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KeyRotationResult{}, ErrNotFound
		}
		return KeyRotationResult{}, fmt.Errorf("federation: lookup actor: %w", err)
	}
	switch actor.Status {
	case ActorStatusTombstone:
		return KeyRotationResult{}, ErrNotFound
	case ActorStatusDisabled:
		return KeyRotationResult{}, ErrNotFederated
	}
	if actor.KeyRotatedAt.Valid && time.Since(actor.KeyRotatedAt.Time) < s.keyRotationCooldown(ctx) {
		return KeyRotationResult{}, ErrRotationCooldown
	}
	return s.rotateActorKey(ctx, actor)
}

// AdminRotateActorKey is the incident-response path — same swap, no
// cooldown: a compromised key can't wait on the member's clock.
func (s *Service) AdminRotateActorKey(ctx context.Context, actorID pgtype.UUID) (KeyRotationResult, error) {
	actor, err := s.store.actorByID(ctx, actorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return KeyRotationResult{}, ErrNotFound
		}
		return KeyRotationResult{}, fmt.Errorf("federation: lookup actor: %w", err)
	}
	switch actor.Status {
	case ActorStatusTombstone:
		return KeyRotationResult{}, ErrNotFound
	case ActorStatusDisabled:
		return KeyRotationResult{}, ErrNotFederated
	}
	return s.rotateActorKey(ctx, actor)
}

// rotateActorKey performs the swap shared by the member and admin
// paths: fresh RSA-2048 pair → conditional active-only write → doc
// cache drop → Update{Person} fanout. Ordering matters — the cache
// must be gone before the first Update delivery lands, or a remote
// re-fetching the actor doc keeps a stale snapshot (carrying the
// retired key) for up to the cache TTL.
func (s *Service) rotateActorKey(ctx context.Context, actor gen.FederatedActor) (KeyRotationResult, error) {
	pubPEM, privPEM, err := generateActorKeyPair()
	if err != nil {
		return KeyRotationResult{}, err
	}
	privEnc, err := jcrypto.Encrypt(privPEM)
	if err != nil {
		return KeyRotationResult{}, fmt.Errorf("federation: encrypt key: %w", err)
	}
	// Nanosecond stamp: two rotations inside the same wall-clock second
	// (member rotate + immediate admin incident rotate) must still mint
	// distinct keyIds — a repeated id would let a remote's stale
	// keyId→pem binding reject signatures the new key actually signed.
	keyID := fmt.Sprintf("%s#main-key-%d", actor.ActorUrl, time.Now().UnixNano())
	rotated, err := s.store.rotateActorKey(ctx, gen.RotateFederatedActorKeyParams{
		ID:            actor.ID,
		PublicKeyPem:  pubPEM,
		PrivateKeyEnc: privEnc,
		PublicKeyID:   keyID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Lost a race with disable/erasure between read and write —
			// the conditional update's status guard declined.
			return KeyRotationResult{}, ErrNotFound
		}
		return KeyRotationResult{}, fmt.Errorf("federation: rotate key: %w", err)
	}
	// Doc caches drop per kind — a remote re-fetching the actor must not
	// see the retired key for the cache TTL. Person keys ride the
	// user-identity path; non-person actors invalidate their own keys.
	switch actor.Kind {
	case ActorKindGroup:
		s.invalidateGroupDoc(ctx, actor.PreferredUsername)
	case ActorKindApplication:
		if s.vk != nil {
			_ = s.vk.Del(ctx, "jc:fed:instance:doc").Err()
			_ = s.vk.Del(ctx, "jc:fed:webfinger:"+strings.ToLower(s.handleDomain())).Err()
		}
	default:
		s.InvalidateActorIdentityForUser(ctx, actor.UserID)
	}
	if err := s.fanoutActorUpdate(ctx, actor.ID, true); err != nil {
		// Fanout failure must not undo the rotation — the keyId-miss
		// refetch path already propagates the new key; the Update only
		// accelerates it.
		slog.Warn("federation: key-rotation update fanout failed", "actor_id", actor.ID.String(), "error", err)
	}
	slog.Info("federation actor key rotated", "actor_id", actor.ID.String(), "key_id", keyID)
	return KeyRotationResult{
		KeyID:     keyID,
		RotatedAt: rotated.KeyRotatedAt.Time.UTC().Format(time.RFC3339),
	}, nil
}

// EraseFederation is the GDPR erasure seam (§126 M2, §19). Confirmed
// erasure runs this BEFORE the user-row DELETE cascades federated_actors
// away: Delete{actor} fans out to known remote inboxes while the key pair
// still exists to sign, then the row tombstones. The hard delete removes
// it physically. Satisfies gdpr.FederationEraser.
func (s *Service) EraseFederation(ctx context.Context, userID pgtype.UUID) error {
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // never federated — nothing to retract
		}
		return fmt.Errorf("federation erase: lookup actor: %w", err)
	}
	if actor.Status != ActorStatusTombstone {
		if _, err := s.fanoutActorDelete(ctx, actor.ID, false); err != nil {
			return fmt.Errorf("federation erase: delete fanout: %w", err)
		}
		// Synchronous best-effort flush BEFORE tombstoning: the user-row
		// cascade moments after this call drops federated_actors (key
		// custody included), and every queued deliver_activity then dead-
		// letters on "local actor missing". Signing can only happen inside
		// this call — the Delete{actor} spec wants fanned out gets its one
		// real attempt now, and any other still-undelivered row for this
		// actor (stranded retries, an earlier disable's Delete) flushes too.
		ids, err := s.store.undeliveredDeliveriesByActor(ctx, actor.ID)
		if err != nil {
			return fmt.Errorf("federation erase: undelivered sweep: %w", err)
		}
		s.flushDeliveriesInline(ctx, ids)
		// Unconditional write — tombstone is terminal and wins every race;
		// a concurrent enable landing in between must not resurrect an
		// account whose user row is about to be hard-deleted.
		if err := s.store.setActorStatus(ctx, actor.ID, ActorStatusTombstone); err != nil {
			return fmt.Errorf("federation erase: tombstone: %w", err)
		}
	}
	s.InvalidateActorIdentityForUser(ctx, userID)
	return nil
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

// AdminActorRow is one row of the admin federated-actors list.
type AdminActorRow struct {
	gen.ListFederatedActorsRow
	Handle string `json:"handle"`
}

// ListActors returns the admin DataTable page of federated actors. sortKey
// is handler-whitelisted; sortDir is 'asc'|'desc'.
func (s *Service) ListActors(ctx context.Context, q, sortKey, sortDir string, limit, offset int32) ([]AdminActorRow, int64, error) {
	rows, err := s.store.listActors(ctx, q, sortKey, sortDir, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("federation: list actors: %w", err)
	}
	total, err := s.store.countActors(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("federation: count actors: %w", err)
	}
	out := make([]AdminActorRow, 0, len(rows))
	for _, r := range rows {
		// The encrypted private key is server-only material — it is never
		// serialized in any response, admin surfaces included (§126).
		r.PrivateKeyEnc = ""
		out = append(out, AdminActorRow{
			ListFederatedActorsRow: r,
			// The handle tracks the live username, not the creation-time
			// preferred_username snapshot — renamed members answer at the
			// new handle.
			Handle: r.Username + "@" + s.handleDomain(),
		})
	}
	return out, total, nil
}

// Stats returns the federation counters document for admins.
func (s *Service) Stats(ctx context.Context) (gen.GetFederationStatsRow, error) {
	return s.store.stats(ctx)
}

// CreateBlock blocks a remote instance domain (normalized lowercase).
func (s *Service) CreateBlock(ctx context.Context, domain, reason string, blockedBy pgtype.UUID) (gen.FederationInstanceBlock, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	// ':' rejects ports — GetInstanceBlockByDomain matches on URL-derived
	// hosts (no port), so a stored 'evil.com:443' would never fire. 253 is
	// the DNS name limit; longer input is rejected rather than erroring
	// on the column at insert time.
	if domain == "" || len(domain) > 253 || strings.ContainsAny(domain, "/@ \t:") {
		return gen.FederationInstanceBlock{}, ErrInvalidDomain
	}
	return s.store.createBlock(ctx, domain, reason, blockedBy)
}

// DeleteBlock removes an instance block by id.
func (s *Service) DeleteBlock(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteBlock(ctx, id)
}

// ListBlocks returns the instance-block page for the admin DataTable.
func (s *Service) ListBlocks(ctx context.Context, q string, limit, offset int32) ([]gen.FederationInstanceBlock, int64, error) {
	rows, err := s.store.listBlocks(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("federation: list blocks: %w", err)
	}
	total, err := s.store.countBlocks(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("federation: count blocks: %w", err)
	}
	return rows, total, nil
}

// ---------------------------------------------------------------------------
// Valkey doc cache (jc:fed:* — spec keys)
// ---------------------------------------------------------------------------

func (s *Service) cacheGet(ctx context.Context, key string) (map[string]any, bool) {
	if s.vk == nil {
		return nil, false
	}
	raw, err := s.vk.Get(ctx, key).Bytes()
	if err != nil || len(raw) == 0 {
		return nil, false
	}
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return nil, false
	}
	return doc, true
}

func (s *Service) cacheSet(ctx context.Context, key string, doc map[string]any, ttl time.Duration) {
	if s.vk == nil {
		return
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return
	}
	_ = s.vk.Set(ctx, key, raw, ttl).Err()
}

// invalidateActorCache drops webfinger/actor doc cache entries for every
// supplied username so status transitions stop serving immediately — both
// the creation-time preferred_username and the live username must be
// passed, since docs are keyed under whichever handle resolved.
// One DEL per key: valkey-go rejects multi-key commands spanning
// different hash slots (panics) — same convention as users/handlers.go.
func (s *Service) invalidateActorCache(ctx context.Context, usernames ...string) {
	if s.vk == nil {
		return
	}
	seen := make(map[string]bool, len(usernames))
	for _, username := range usernames {
		u := strings.ToLower(username)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		for _, k := range [2]string{"jc:fed:webfinger:" + u, "jc:fed:actor:" + u} {
			_ = s.vk.Del(ctx, k).Err()
		}
	}
}

// InvalidateActorIdentity drops cached WebFinger/actor docs for each given
// handle. Called by the users service on rename so neither the vacated nor
// the newly claimed username serves a stale document (jc:fed:* keys).
func (s *Service) InvalidateActorIdentity(ctx context.Context, usernames ...string) {
	s.invalidateActorCache(ctx, usernames...)
}

// InvalidateActorIdentityForUser resolves the member's actor handles —
// creation-time preferred_username + live username — plus any extra
// handles supplied by the caller, and drops their cached WebFinger/actor
// docs. Fired by lifecycle sync on transitions OUT of active (the
// status-gated lookups go blind at exactly that moment, so handles
// resolve without status filters) and by users.Service on rename
// (passing the vacated + claimed usernames as extras so an earlier
// preferred_username can't leave a stale doc behind either). A member
// with no actor row is a cheap no-op.
func (s *Service) InvalidateActorIdentityForUser(ctx context.Context, userID pgtype.UUID, extraHandles ...string) {
	if s.store == nil {
		return
	}
	row, err := s.store.actorHandlesByUserID(ctx, userID)
	if err != nil {
		return
	}
	handles := make([]string, 0, len(extraHandles)+2)
	handles = append(handles, row.PreferredUsername, row.Username)
	handles = append(handles, extraHandles...)
	s.invalidateActorCache(ctx, handles...)
}

// FanoutActorUpdateForUser pushes Update{Person} to the member's remote
// followers — the §126 M4 identity-change push. Called by the users
// service after doc-rendered fields change (display_name/avatar/bio,
// rename, public identity). No-ops for unfederated or non-active actors.
// Satisfies users.federationUpdateFanouter.
func (s *Service) FanoutActorUpdateForUser(ctx context.Context, userID pgtype.UUID) {
	if s.store == nil {
		return
	}
	actor, err := s.store.actorByUserID(ctx, userID)
	if err != nil || actor.Status != ActorStatusActive {
		return
	}
	if err := s.fanoutActorUpdate(ctx, actor.ID, true); err != nil {
		slog.Warn("federation: identity-update fanout failed", "actor_id", actor.ID.String(), "error", err)
	}
}

// RemoteCacheKey is the catalogued jc:fed:remote:{sha256(actor_url)} key.
func RemoteCacheKey(actorURL string) string {
	sum := sha256.Sum256([]byte(actorURL))
	return "jc:fed:remote:" + hex.EncodeToString(sum[:])
}

// InboxDedupKey is the catalogued jc:fed:inbox:dedup:{sha256(activity_ap_id)}
// marker — SET NX EX 48h, the fast-path dedup ahead of the DB unique index.
func InboxDedupKey(activityApID string) string {
	sum := sha256.Sum256([]byte(activityApID))
	return "jc:fed:inbox:dedup:" + hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// M2 — inbox pipeline support
// ---------------------------------------------------------------------------

// ActorExists returns the actor row for a per-actor inbox target —
// ErrNotFound when no federated actor holds the handle (the handler maps
// non-active statuses to 410 itself, so this returns the row unfiltered
// on fa.status; the user-status gate stays — suspended members hide).
func (s *Service) ActorExists(ctx context.Context, username string) (gen.FederatedActor, error) {
	actor, err := s.store.actorByUsernameAnyStatus(ctx, username)
	if err != nil {
		return gen.FederatedActor{}, ErrNotFound
	}
	return actor, nil
}

// rejectInbound ledgers a blocked-domain activity as 'rejected' for audit —
// the §126 contract keeps blocked traffic visible without processing it.
// Best-effort: a DB hiccup here must not change the handler's 403.
// The activity id is only stored when it shares the declared actor's
// host — ledgering a foreign-domain id would itself squat the global
// ap_id dedup the provenance rule exists to protect.
func (s *Service) rejectInbound(ctx context.Context, act map[string]any, actorURL, reason string, body []byte) {
	rejected := "rejected"
	apID := apStr(act["id"])
	if apID != "" && !sameURLHost(apID, actorURL) {
		apID = ""
	}
	row, err := s.store.insertInbound(ctx, gen.InsertFederationInboundParams{
		ActivityApID:   stringPtrOrNil(apID),
		ActivityType:   apStr(act["type"]),
		ActorUrl:       stringPtrOrNil(actorURL),
		SignatureValid: false,
		Payload:        body,
		Status:         &rejected,
	})
	if err != nil {
		return // duplicate or transient — audit loss is acceptable
	}
	if err := s.store.updateInboundStatus(ctx, row.ID, "rejected", &reason); err != nil {
		slog.Warn("federation: rejected-row stamp failed", "error", err)
	}
}

// ActorTombstone returns the Tombstone document for a handle whose actor
// row exists but is non-active (disabled/tombstone) — nil when no actor
// holds the handle at all. Served at 410 Gone per §126 M2. The lookup is
// deliberately ungated on fa.status: the status-gated actorByUsername
// would go blind at exactly the moment a tombstone must answer.
func (s *Service) ActorTombstone(ctx context.Context, username string) map[string]any {
	actor, err := s.store.actorByUsernameAnyStatus(ctx, username)
	if err != nil || actor.Status == "active" {
		return nil
	}
	return tombstoneDocument(actor.ActorUrl)
}
