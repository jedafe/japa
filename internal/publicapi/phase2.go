package publicapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/feed"
	"github.com/jedafe/japa/internal/rag"
	"github.com/jedafe/japa/internal/scamalert"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/visitor"
)

// ErrInvalidFeedName is returned when an explore feed name is not one of
// popular|latest|rising. Handlers map it to 400 invalid_feed_name.
var ErrInvalidFeedName = errors.New("invalid_feed_name")

// ErrSearchUnavailable is returned when semantic search cannot run because
// rag.enabled is false or no embedder is configured. Handlers map it to
// 503 search_unavailable.
var ErrSearchUnavailable = errors.New("search_unavailable")

// ErrMissingQuery is returned when the semantic search query is empty.
// Handlers map it to 400 missing_query.
var ErrMissingQuery = errors.New("missing_query")

// ---------------------------------------------------------------------------
// Reader interfaces — narrow slices of the domain services (agent data API
// Phase 2). Ref: docs/superpowers/specs/2026-09-24-agent-data-api-phase2-design.md
// ---------------------------------------------------------------------------

// FeedReader is the subset of feed.Service the public API needs.
type FeedReader interface {
	GetFeed(ctx context.Context, timeline string, userID, spaceID, authorID *pgtype.UUID,
		sort string, limit, offset int32, visitorCountry, visitorRegion string) ([]feed.FeedItem, error)
	GetForYouFeed(ctx context.Context, userID pgtype.UUID, sortMode string,
		limit, offset int32, vc *visitor.VisitorContext) ([]feed.FeedItem, error)
}

// SafetyReader is the subset of scamalert.Service the public API needs.
// Get/ListByOffset only ever return status='confirmed' reports (enforced
// in SQL).
type SafetyReader interface {
	Enabled(ctx context.Context) bool
	Schema() scamalert.SchemaOptions
	Get(ctx context.Context, id pgtype.UUID) (scamalert.ScamReportDTO, error)
	ListByOffset(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) ([]scamalert.ScamReportDTO, int64, error)
}

// SemanticSearcher is the subset of rag.Service the public API needs.
// Retrieve is caller-scoped (§69 scoped-retrieval amendment); ScopeHash
// digests the viewer's scope so the result cache never crosses viewers.
type SemanticSearcher interface {
	Retrieve(ctx context.Context, query, language string, geoSpaceID *pgtype.UUID, viewerID pgtype.UUID) ([]rag.RetrievedChunk, error)
	ScopeHash(ctx context.Context, viewerID pgtype.UUID) string
	EmbedderConfigured() bool
}

// ---------------------------------------------------------------------------
// Public DTOs
// ---------------------------------------------------------------------------

// PublicFeedItem is the stable feed-card shape for API consumers.
type PublicFeedItem struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Slug         string   `json:"slug"`
	ContentType  string   `json:"content_type"`
	Excerpt      string   `json:"excerpt,omitempty"`
	SpaceID      string   `json:"space_id,omitempty"`
	SpaceSlug    string   `json:"space_slug,omitempty"`
	SpaceName    string   `json:"space_name,omitempty"`
	AuthorName   string   `json:"author_name,omitempty"`
	AuthorHandle string   `json:"author_handle,omitempty"`
	VoteScore    int32    `json:"vote_score"`
	CommentCount int32    `json:"comment_count"`
	ViewCount    int32    `json:"view_count"`
	Tags         []string `json:"tags,omitempty"`
	PublishedAt  string   `json:"published_at,omitempty"`
}

// PublicScamReport is the curated public DTO — scamalert.ScamReportDTO
// carries moderation internals (reporter_id, reviewed_by, moderator_note,
// ai_model, hitl_*) that must not enter the public contract.
type PublicScamReport struct {
	ID                    string `json:"id"`
	ScamType              string `json:"scam_type"`
	DestinationSpaceName  string `json:"destination_space_name,omitempty"`
	DestinationSpaceCode  string `json:"destination_space_code,omitempty"`
	OriginSpaceName       string `json:"origin_space_name,omitempty"`
	OriginSpaceCode       string `json:"origin_space_code,omitempty"`
	OriginCommunity       string `json:"origin_community,omitempty"`
	PlatformUsed          string `json:"platform_used,omitempty"`
	ApproximateDate       string `json:"approximate_date,omitempty"`
	AmountRange           string `json:"amount_range,omitempty"`
	Description           string `json:"description"`
	ReportedToAuthorities bool   `json:"reported_to_authorities"`
	IsPinned              bool   `json:"is_pinned"`
	CreatedAt             string `json:"created_at"`
}

// SemanticSearchResult is the envelope for GET /public/search/semantic.
type SemanticSearchResult struct {
	Data     []PublicSemanticHit `json:"data"`
	Query    string              `json:"query"`
	Language string              `json:"language"`
	Space    *string             `json:"space"`
	Cached   bool                `json:"cached"`
}

// PublicSemanticHit is one retrieved chunk in the public contract. Title and
// Href are best-effort resolutions and stay null when the source entity
// cannot be loaded.
type PublicSemanticHit struct {
	EntityType  string  `json:"entity_type"`
	EntityID    string  `json:"entity_id"`
	ContentType string  `json:"content_type"`
	Snippet     string  `json:"snippet"`
	Similarity  float64 `json:"similarity"`
	Tier        string  `json:"tier,omitempty"`
	Title       *string `json:"title"`
	Href        *string `json:"href"`
}

// ---------------------------------------------------------------------------
// Service wiring
// ---------------------------------------------------------------------------

// SetFeedReader injects the feed.Service subset used by the public feeds.
func (s *Service) SetFeedReader(r FeedReader) { s.feedReader = r }

// SetSafetyReader injects the scamalert.Service subset used by the public
// scam-alert surface.
func (s *Service) SetSafetyReader(r SafetyReader) { s.safetyReader = r }

// SetSemanticSearcher injects the rag.Service subset used by semantic search.
func (s *Service) SetSemanticSearcher(r SemanticSearcher) { s.semanticSearcher = r }

// SetQueries injects the sqlc queries used by the tag feed cursor query and
// semantic-search title/href resolution — the same dependency
// Handlers.SetQueries wires for the v1 public routes.
func (s *Service) SetQueries(q *gen.Queries) { s.queries = q }

// boolSettingGetter exposes GetBoolOr — *config.Settings satisfies it. The
// shared rateLimitSettingsReader only declares GetIntOr, so boolean settings
// (rag.enabled) are reached through this structural interface.
type boolSettingGetter interface {
	GetBoolOr(ctx context.Context, key string, fallback bool) bool
}

// boolSetting reads a boolean system setting through the injected settings
// reader, falling back when the reader is absent or int-only.
func (s *Service) boolSetting(ctx context.Context, key string, fallback bool) bool {
	if br, ok := s.settingsReader.(boolSettingGetter); ok {
		return br.GetBoolOr(ctx, key, fallback)
	}
	return fallback
}

// ---------------------------------------------------------------------------
// Feeds
// ---------------------------------------------------------------------------

// exploreFeedSorts maps public feed names to feed sort modes.
var exploreFeedSorts = map[string]string{
	"popular": feed.SortTop,
	"latest":  feed.SortLatest,
	"rising":  feed.SortRising,
}

// ExploreFeed serves GET /public/feeds/{name}. name ∈ {popular, latest,
// rising}; an explicit valid sort overrides the name mapping while an
// invalid one falls back to it.
func (s *Service) ExploreFeed(ctx context.Context, name, sort string, limit, offset int32) ([]PublicFeedItem, error) {
	if s.feedReader == nil {
		return nil, ErrReaderUnconfigured
	}

	mapped, ok := exploreFeedSorts[name]
	if !ok {
		return nil, ErrInvalidFeedName
	}

	switch sort {
	case feed.SortLatest, feed.SortTop, feed.SortRising, feed.SortRelevant:
		mapped = sort
	}

	items, err := s.feedReader.GetFeed(ctx, "explore", nil, nil, nil, mapped, limit, offset, "", "")
	if err != nil {
		return nil, err
	}

	return mapFeedItems(items), nil
}

// TagFeed serves GET /public/feeds/tag/{tag}. The cursor is the opaque
// composite base64("{created_at_rfc3339nano},{uuid}") — decode failures are
// 400s. next_cursor is emitted only when the page is full.
func (s *Service) TagFeed(ctx context.Context, tagSlug, cursor string, limit int32) ([]PublicFeedItem, string, error) {
	if s.queries == nil {
		return nil, "", ErrReaderUnconfigured
	}

	tag, err := s.queries.GetTagBySlug(ctx, tagSlug)
	if err != nil {
		// pgx.ErrNoRows surfaces as 404 via HandleServiceError.
		return nil, "", err
	}

	var cursorCreatedAt pgtype.Timestamptz
	var cursorID pgtype.UUID

	if cursor != "" {
		cursorCreatedAt, cursorID, err = decodeTagFeedCursor(cursor)
		if err != nil {
			return nil, "", err
		}
	}

	if limit <= 0 {
		limit = defaultPageSize
	}

	// Public surface: no unpublished, private, or members-only visibility and
	// an anonymous (zero) requester.
	rows, err := s.queries.ListContentByTag(ctx, gen.ListContentByTagParams{
		TagID:             tag.ID,
		CanSeeUnpublished: false,
		CanSeePrivate:     false,
		RequesterID:       pgtype.UUID{},
		CanSeeMembers:     false,
		CursorCreatedAt:   cursorCreatedAt,
		CursorID:          cursorID,
		PageLimit:         limit,
	})
	if err != nil {
		return nil, "", err
	}

	items := make([]PublicFeedItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, tagRowToFeedItem(r))
	}

	nextCursor := ""
	if int32(len(rows)) == limit && len(rows) > 0 {
		last := rows[len(rows)-1]
		nextCursor = encodeTagFeedCursor(last.CreatedAt, last.ID)
	}

	return items, nextCursor, nil
}

// SpaceFeed serves GET /public/feeds/space/{slug}. The space must resolve via
// GetPublicSpaceBySlug (public + active + not deleted, enforced in SQL);
// viewerID is the key owner's ID so member-only items resolve exactly as the
// web feed does (mirrors feed.Handlers.GetSpaceFeed).
func (s *Service) SpaceFeed(ctx context.Context, viewerID pgtype.UUID, slug, sort string, limit, offset int32) ([]PublicFeedItem, error) {
	if s.feedReader == nil || s.moat == nil || s.moat.Spaces == nil {
		return nil, ErrReaderUnconfigured
	}

	space, err := s.moat.Spaces.GetPublicSpaceBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	items, err := s.feedReader.GetFeed(ctx, "space", &viewerID, &space.ID, nil, sort, limit, offset, "", "")
	if err != nil {
		return nil, err
	}

	return mapFeedItems(items), nil
}

// MyFeed serves GET /public/me/feed — the key owner's ranked for-you feed.
// A nil VisitorContext is handled inside the feed service (geo
// personalization skipped).
func (s *Service) MyFeed(ctx context.Context, userID pgtype.UUID, sort string, limit, offset int32) ([]PublicFeedItem, error) {
	if s.feedReader == nil {
		return nil, ErrReaderUnconfigured
	}

	items, err := s.feedReader.GetForYouFeed(ctx, userID, sort, limit, offset, nil)
	if err != nil {
		return nil, err
	}

	return mapFeedItems(items), nil
}

// ---------------------------------------------------------------------------
// Scam alerts
// ---------------------------------------------------------------------------

// ListScamAlerts serves GET /public/scam-alerts. Enablement is covered by
// the route's features.Gate("scam_alert.enabled") middleware.
func (s *Service) ListScamAlerts(ctx context.Context, destSpaceID pgtype.UUID, scamType string, limit, offset int32) ([]PublicScamReport, int64, error) {
	if s.safetyReader == nil {
		return nil, 0, ErrReaderUnconfigured
	}

	if limit <= 0 {
		limit = defaultPageSize
	}
	if offset < 0 {
		offset = 0
	}

	// The public contract is limit/offset — ListByOffset is offset-native so
	// misaligned offsets cannot silently shift the result window.
	dtos, total, err := s.safetyReader.ListByOffset(ctx, destSpaceID, scamType, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	out := make([]PublicScamReport, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, mapPublicScamReport(d))
	}

	return out, total, nil
}

// GetScamAlert serves GET /public/scam-alerts/{id}. pgx.ErrNoRows from the
// domain service surfaces as 404 via HandleServiceError.
func (s *Service) GetScamAlert(ctx context.Context, id pgtype.UUID) (PublicScamReport, error) {
	if s.safetyReader == nil {
		return PublicScamReport{}, ErrReaderUnconfigured
	}

	dto, err := s.safetyReader.Get(ctx, id)
	if err != nil {
		return PublicScamReport{}, err
	}

	return mapPublicScamReport(dto), nil
}

// ScamAlertSchema serves GET /public/scam-alerts/schema — the canonical
// scam_type/platform enum options.
func (s *Service) ScamAlertSchema() scamalert.SchemaOptions {
	if s.safetyReader == nil {
		return scamalert.SchemaOptions{}
	}

	return s.safetyReader.Schema()
}

// ---------------------------------------------------------------------------
// Semantic search
// ---------------------------------------------------------------------------

// SemanticSearch serves GET /public/search/semantic (premium). Results are
// cached in Valkey under jc:api_semantic:{sha256(query)}:{lang}:{spaceSlug}
// with the api.semantic_search.cache_ttl_seconds TTL (default 300).
func (s *Service) SemanticSearch(ctx context.Context, viewerID pgtype.UUID, query, lang, spaceSlug string, limit int32) (*SemanticSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, ErrMissingQuery
	}
	// lang defaults here (not in the handlers) so the REST and MCP paths
	// share it — Retrieve filters on the embedding language and an empty
	// lang would silently return no rows.
	if lang == "" {
		lang = "en"
	}

	if s.semanticSearcher == nil {
		return nil, ErrReaderUnconfigured
	}

	// Retrieve lies by omission — it returns nil, nil both when rag.enabled
	// is false and when no embedder is wired — so availability is checked
	// explicitly here to answer 503 honestly.
	if !s.boolSetting(ctx, "rag.enabled", true) || !s.semanticSearcher.EmbedderConfigured() {
		return nil, ErrSearchUnavailable
	}

	var geoSpaceID *pgtype.UUID
	if spaceSlug != "" {
		if s.moat == nil || s.moat.Spaces == nil {
			return nil, ErrReaderUnconfigured
		}
		space, err := s.moat.Spaces.GetPublicSpaceBySlug(ctx, spaceSlug)
		if err != nil {
			return nil, err // pgx.ErrNoRows → 404 via HandleServiceError
		}
		geoSpaceID = &space.ID
	}

	// The cache key folds in the caller's scope hash — scoped results must
	// never cross viewers (§69 scoped-retrieval amendment).
	cacheKey := valkey.MustKey("api_semantic:%s:%s:%s:%s", semanticQueryHash(query), lang, spaceSlug,
		s.semanticSearcher.ScopeHash(ctx, viewerID))
	if s.cache != nil {
		if raw, err := s.cache.Get(ctx, cacheKey).Bytes(); err == nil {
			var cached SemanticSearchResult
			if json.Unmarshal(raw, &cached) == nil {
				cached.Cached = true
				sliceSemanticHits(&cached, limit)
				return &cached, nil
			}
		}
	}

	chunks, err := s.semanticSearcher.Retrieve(ctx, query, lang, geoSpaceID, viewerID)
	if err != nil {
		return nil, err
	}

	result := &SemanticSearchResult{
		Data:     make([]PublicSemanticHit, 0, len(chunks)),
		Query:    query,
		Language: lang,
	}
	if spaceSlug != "" {
		result.Space = &spaceSlug
	}

	for _, c := range chunks {
		if hit, ok := s.semanticHit(ctx, c); ok {
			result.Data = append(result.Data, hit)
		}
	}

	// Write-through cache (Rule 3: TTL from system_settings, fallback 300).
	// The cache stores the full filtered set — limit is applied after the
	// cache read so a small-limit request cannot poison later callers.
	if s.cache != nil {
		ttl := 300
		if s.settingsReader != nil {
			ttl = s.settingsReader.GetIntOr(ctx, "api.semantic_search.cache_ttl_seconds", 300)
		}
		if raw, err := json.Marshal(result); err == nil {
			_ = s.cache.Set(ctx, cacheKey, raw, time.Duration(ttl)*time.Second).Err()
		}
	}

	sliceSemanticHits(result, limit)
	return result, nil
}

// sliceSemanticHits truncates result.Data to limit (no-op when limit <= 0).
func sliceSemanticHits(result *SemanticSearchResult, limit int32) {
	if limit > 0 && int32(len(result.Data)) > limit {
		result.Data = result.Data[:limit]
	}
}

// semanticHit maps a retrieved chunk to the public DTO. Retrieval is
// caller-scoped (§69 scoped-retrieval amendment), so hits are authorized
// by the query itself — this resolver attaches title/href and drops hits
// whose entity can no longer be resolved (deleted or inactive), which is
// the remaining way a stale embedding row could leak.
func (s *Service) semanticHit(ctx context.Context, c rag.RetrievedChunk) (PublicSemanticHit, bool) {
	hit := PublicSemanticHit{
		EntityType:  c.EntityType,
		EntityID:    c.EntityID,
		ContentType: c.ContentType,
		Snippet:     c.ChunkText,
		Similarity:  c.Similarity,
		Tier:        c.Tier,
	}

	switch c.EntityType {
	case "space", "content", "forum_thread":
		if s.queries == nil {
			return hit, false
		}
		var entityID pgtype.UUID
		if err := entityID.Scan(c.EntityID); err != nil {
			return hit, false
		}
		if c.EntityType == "space" {
			space, err := s.queries.GetSpaceByID(ctx, entityID)
			if err != nil || space.Status != "active" {
				return hit, false
			}
			title := space.Name
			hit.Title = &title
			href := "/space/" + space.Slug
			hit.Href = &href
		} else if c.EntityType == "forum_thread" {
			thread, err := s.queries.GetForumThreadByID(ctx, entityID)
			if err != nil {
				return hit, false
			}
			title := thread.Title
			hit.Title = &title
			href := "/forum/" + thread.Slug
			hit.Href = &href
		} else {
			row, err := s.queries.GetContentByID(ctx, entityID)
			if err != nil || row.Status != "published" {
				return hit, false
			}
			title := row.Title
			hit.Title = &title
			href := "/content/" + row.Slug
			hit.Href = &href
		}
	default:
		// wiki_page and academy_track corpora are public-facing by design
		// (approval-gated wiki, public course catalog); they surface without
		// title/href since there is no cheap per-entity resolver.
	}

	return hit, true
}

// semanticQueryHash keys the Valkey cache on the query text.
func semanticQueryHash(query string) string {
	sum := sha256.Sum256([]byte(query))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Mapping helpers
// ---------------------------------------------------------------------------

func mapFeedItems(items []feed.FeedItem) []PublicFeedItem {
	out := make([]PublicFeedItem, 0, len(items))
	for _, it := range items {
		out = append(out, feedItemToPublic(it))
	}
	return out
}

func feedItemToPublic(it feed.FeedItem) PublicFeedItem {
	p := PublicFeedItem{
		ID:           it.ID,
		Title:        it.Title,
		Slug:         it.Slug,
		ContentType:  it.ContentType,
		Excerpt:      it.Excerpt,
		SpaceSlug:    it.SpaceSlug,
		SpaceName:    it.SpaceName,
		AuthorName:   it.AuthorName,
		AuthorHandle: it.AuthorUsername,
		VoteScore:    it.VoteScore,
		CommentCount: it.CommentCount,
		ViewCount:    it.ViewCount,
		Tags:         it.Tags,
		PublishedAt:  it.CreatedAt,
	}
	if it.SpaceID != nil {
		p.SpaceID = *it.SpaceID
	}
	return p
}

// tagRowToFeedItem maps a ListContentByTagRow (which carries title, slug,
// excerpt, tags, space, and author joins) onto the stable feed-card shape.
func tagRowToFeedItem(r gen.ListContentByTagRow) PublicFeedItem {
	p := PublicFeedItem{
		ID:           r.ID.String(),
		Title:        r.Title,
		Slug:         r.Slug,
		ContentType:  r.ContentType,
		Excerpt:      feed.CleanExcerpt(r.Excerpt, r.Title),
		SpaceSlug:    r.SpaceSlug,
		SpaceName:    r.SpaceName,
		AuthorName:   r.AuthorName,
		CommentCount: r.CommentCount,
		ViewCount:    r.ViewCount,
		Tags:         splitTagList(r.Tags),
	}
	if r.SpaceID.Valid {
		p.SpaceID = r.SpaceID.String()
	}
	switch {
	case r.PublishedAt.Valid:
		p.PublishedAt = r.PublishedAt.Time.Format(time.RFC3339)
	case r.CreatedAt.Valid:
		p.PublishedAt = r.CreatedAt.Time.Format(time.RFC3339)
	}
	return p
}

// mapPublicScamReport copies only the curated public fields — moderation
// internals (reporter_id, reviewed_by, moderator_note, ai_*, hitl_*) are
// dropped here.
func mapPublicScamReport(d scamalert.ScamReportDTO) PublicScamReport {
	return PublicScamReport{
		ID:                    d.ID,
		ScamType:              d.ScamType,
		DestinationSpaceName:  d.DestinationSpaceName,
		DestinationSpaceCode:  d.DestinationSpaceCode,
		OriginSpaceName:       d.OriginSpaceName,
		OriginSpaceCode:       d.OriginSpaceCode,
		OriginCommunity:       d.OriginCommunity,
		PlatformUsed:          d.PlatformUsed,
		ApproximateDate:       d.ApproximateDate,
		AmountRange:           d.AmountRange,
		Description:           d.Description,
		ReportedToAuthorities: d.ReportedToAuthorities,
		IsPinned:              d.IsPinned,
		CreatedAt:             d.CreatedAt,
	}
}

// splitTagList converts the comma-separated string_agg tag list into a
// string slice (same decoding the web tag handler applies).
func splitTagList(v any) []string {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// encodeTagFeedCursor builds the opaque composite cursor
// base64("{created_at_rfc3339nano},{uuid}").
func encodeTagFeedCursor(createdAt pgtype.Timestamptz, id pgtype.UUID) string {
	raw := createdAt.Time.Format(time.RFC3339Nano) + "," + id.String()
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// decodeTagFeedCursor parses a cursor produced by encodeTagFeedCursor.
// Malformed input maps to a 400 via errpkg.ErrInvalid.
func decodeTagFeedCursor(cursor string) (pgtype.Timestamptz, pgtype.UUID, error) {
	raw, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, fmt.Errorf("%w: malformed cursor", errpkg.ErrInvalid)
	}

	ts, idStr, ok := strings.Cut(string(raw), ",")
	if !ok {
		return pgtype.Timestamptz{}, pgtype.UUID{}, fmt.Errorf("%w: malformed cursor", errpkg.ErrInvalid)
	}

	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, fmt.Errorf("%w: malformed cursor timestamp", errpkg.ErrInvalid)
	}

	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, fmt.Errorf("%w: malformed cursor id", errpkg.ErrInvalid)
	}

	return pgtype.Timestamptz{Time: t, Valid: true}, id, nil
}
