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
	cfg      Config
	settings SettingsReader
	vk       valkeycompat.Cmdable
	flags    features.FlagEvaluator
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB, cfg Config) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer)),
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
	if u, err := url.Parse(s.cfg.SiteBase); err == nil && u.Host != "" {
		return u.Host
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

var acctResourceRe = regexp.MustCompile(`^acct:([A-Za-z0-9_\-.]{1,64})@([A-Za-z0-9.\-]+)$`)

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

	cacheKey := "jc:fed:webfinger:" + strings.ToLower(username)
	if doc, ok := s.cacheGet(ctx, cacheKey); ok {
		return doc, nil
	}

	actor, err := s.store.actorByUsername(ctx, username)
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
		actor.PreferredUsername, canonical, actor.ActorUrl,
		s.profileURL(actor.PreferredUsername),
	)
	s.cacheSet(ctx, cacheKey, doc, 5*time.Minute)
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
	doc["@context"] = asContext
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
		obj := s.objectFromListRow(row, actor)
		items = append(items, createActivity(actor.ActorUrl, actor.FollowersUrl, obj))
	}

	return orderedCollectionPage(pageID(page), actor.OutboxUrl, items, total,
		int64(page)*int64(size) < total, page > 1, pageID, page), nil
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

	total, err := s.store.countLocalFollowing(ctx, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("federation: following count: %w", err)
	}

	if page <= 0 {
		return orderedCollection(actor.FollowingUrl, total, actor.FollowingUrl+"?page=1"), nil
	}

	size := s.outboxPageSize(ctx)
	offset := int32(page-1) * size

	pageID := func(p int) string { return fmt.Sprintf("%s?page=%d", actor.FollowingUrl, p) }
	if int64(offset) >= total {
		return orderedCollectionPage(pageID(page), actor.FollowingUrl, []any{}, total, false, page > 1, pageID, page), nil
	}

	urls, err := s.store.localFollowingURLs(ctx, actor.UserID, size, offset)
	if err != nil {
		return nil, fmt.Errorf("federation: following page: %w", err)
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
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("federation: object lookup: %w", err)
	}

	actor, err := s.store.actorByUserID(ctx, row.AuthorID)
	if err != nil || actor.Status != ActorStatusActive {
		return nil, ErrNotFound
	}

	obj := s.objectFromGetRow(row, actor)
	obj["@context"] = asContext
	return obj, nil
}

// objectFromListRow maps an outbox row to its AP object document.
func (s *Service) objectFromListRow(row gen.ListFederatableContentByAuthorRow, actor gen.FederatedActor) map[string]any {
	return objectDocument(gen.GetFederatableContentByIDRow{
		ID:          row.ID,
		ContentType: row.ContentType,
		Title:       row.Title,
		Slug:        row.Slug,
		BodyHtml:    row.BodyHtml,
		PublishedAt: row.PublishedAt,
		UpdatedAt:   row.UpdatedAt,
	}, actor.ActorUrl, actor.FollowersUrl, s.siteBase(), s.pageURL(row.ContentType, row.Slug, row.CanonicalUrl))
}

func (s *Service) objectFromGetRow(row gen.GetFederatableContentByIDRow, actor gen.FederatedActor) map[string]any {
	return objectDocument(row, actor.ActorUrl, actor.FollowersUrl, s.siteBase(), s.pageURL(row.ContentType, row.Slug, row.CanonicalUrl))
}

// pageURL resolves the human-facing URL for a contents row: the editorial
// canonical_url when set, else the content-type route path.
func (s *Service) pageURL(contentType, slug string, canonical *string) string {
	if canonical != nil && *canonical != "" {
		return *canonical
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
	return nodeInfoDocument(version, s.apiBase(), usage, open), nil
}

// ---------------------------------------------------------------------------
// Member opt-in
// ---------------------------------------------------------------------------

// FederationStatus is the member-facing view of their actor state.
type FederationStatus struct {
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	ActorURL  string `json:"actor_url,omitempty"`
	Handle    string `json:"handle,omitempty"`
	PublicKey string `json:"-"`
	CreatedAt string `json:"created_at,omitempty"`
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
	return s.statusFor(actor), nil
}

func (s *Service) statusFor(actor gen.FederatedActor) FederationStatus {
	st := FederationStatus{
		Enabled:  actor.Status == ActorStatusActive,
		Status:   actor.Status,
		ActorURL: actor.ActorUrl,
		Handle:   actor.PreferredUsername + "@" + s.handleDomain(),
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
		return FederationStatus{}, fmt.Errorf("federation: lookup user: %w", err)
	}
	username := user.Username

	existing, err := s.store.actorByUserID(ctx, userID)
	if err == nil {
		if existing.Status != ActorStatusActive {
			if uerr := s.store.setActorStatus(ctx, existing.ID, ActorStatusActive); uerr != nil {
				return FederationStatus{}, fmt.Errorf("federation: reactivate: %w", uerr)
			}
			existing.Status = ActorStatusActive
			s.invalidateActorCache(ctx, existing.PreferredUsername)
		}
		return s.statusFor(existing), nil
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
		return FederationStatus{}, fmt.Errorf("federation: create actor: %w", err)
	}

	slog.Info("federation actor enabled", "user_id", userID.String(), "actor_url", actorURL)
	return s.statusFor(actor), nil
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
	if actor.Status != ActorStatusDisabled {
		if err := s.store.setActorStatus(ctx, actor.ID, ActorStatusDisabled); err != nil {
			return FederationStatus{}, fmt.Errorf("federation: disable: %w", err)
		}
	}
	s.invalidateActorCache(ctx, actor.PreferredUsername)
	actor.Status = ActorStatusDisabled
	return s.statusFor(actor), nil
}

// ---------------------------------------------------------------------------
// Admin
// ---------------------------------------------------------------------------

// AdminActorRow is one row of the admin federated-actors list.
type AdminActorRow struct {
	gen.ListFederatedActorsRow
	Handle string `json:"handle"`
}

// ListActors returns the admin DataTable page of federated actors.
func (s *Service) ListActors(ctx context.Context, q string, limit, offset int32) ([]AdminActorRow, int64, error) {
	rows, err := s.store.listActors(ctx, q, limit, offset)
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
			Handle:                 r.PreferredUsername + "@" + s.handleDomain(),
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
	if domain == "" || strings.ContainsAny(domain, "/@ \t") {
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

// invalidateActorCache drops a username's webfinger/actor doc cache on
// status transitions so disabled actors stop serving immediately.
func (s *Service) invalidateActorCache(ctx context.Context, username string) {
	if s.vk == nil {
		return
	}
	u := strings.ToLower(username)
	_ = s.vk.Del(ctx, "jc:fed:webfinger:"+u, "jc:fed:actor:"+u).Err()
}

// RemoteCacheKey is the catalogued jc:fed:remote:{sha256(actor_url)} key.
func RemoteCacheKey(actorURL string) string {
	sum := sha256.Sum256([]byte(actorURL))
	return "jc:fed:remote:" + hex.EncodeToString(sum[:])
}
