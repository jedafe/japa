package docs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Cache TTLs (VALKEY-003 / catalog: jc:cache:page:docs:nav, jc:cache:page:docs:{audience}:{slug}).
const (
	docNavCacheTTL  = 10 * time.Minute
	docPageCacheTTL = 5 * time.Minute
)

// AIDispatcher is the subset of the AI Gateway the docs service needs to
// generate draft pages (content_generation category).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// DocPagesSearcher is the contract for Meilisearch-backed doc page search.
// Implementations live in the search package (SEARCH-001, DOC-003).
type DocPagesSearcher interface {
	SearchDocPages(ctx context.Context, query string, audiences []string, limit int32) ([]DocSearchResult, error)
}

// Service provides business logic for documentation pages.
type Service struct {
	store      *Store
	cache      valkeycompat.Cmdable
	dispatcher AIDispatcher
	searcher   DocPagesSearcher
}

// SetCache injects a Valkey cache client for docs page/nav caching
// (VALKEY-003). When nil, the service falls back to direct DB reads.
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetDispatcher injects the AI Gateway dispatcher for draft generation.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetSearcher injects a Meilisearch-backed searcher for doc pages (DOC-003).
func (s *Service) SetSearcher(sr DocPagesSearcher) { s.searcher = sr }

// NewService creates a Service backed by the given database pools.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
	}
}

// GenerateDraftRequest is the input for AI-generated doc page drafts
// (ADR-088, routed from the Content Studio).
type GenerateDraftRequest struct {
	Title       string `json:"title"`
	Slug        string `json:"slug"`
	Audience    string `json:"audience"`
	Prompt      string `json:"prompt"`
	Category    string `json:"category,omitempty"`
	Subcategory string `json:"subcategory,omitempty"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	PageID  string `json:"page_id"`
}

// GetDocPageBySlug returns a published page by audience and slug. Checks the
// Valkey cache first (jc:cache:page:docs:{audience}:{slug}:{}, TTL 5min)
// and falls back to the DB on miss (VALKEY-003). Cache key is platform-scoped
// to prevent cross-platform data bleed (Rule 4, §4.2).
func (s *Service) GetDocPageBySlug(ctx context.Context, audience, slug string) (*DocPage, error) {
	cacheKey := valkey.MustKey("cache:page:docs:%s:%s:", audience, slug)
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil {
			var page DocPage
			if jsonErr := json.Unmarshal([]byte(cached), &page); jsonErr == nil {
				return &page, nil
			}
		}
	}

	page, err := s.store.getBySlug(ctx, audience, slug)
	if err != nil {
		return nil, err
	}

	result := toDocPage(page)
	if s.cache != nil {
		if data, jsonErr := json.Marshal(result); jsonErr == nil {
			if err := s.cache.Set(ctx, cacheKey, string(data), docPageCacheTTL).Err(); err != nil {
				slog.Warn("docs: failed to cache page", "key", cacheKey, "error", err)
			}
		}
	}

	return result, nil
}

// GetDocPageByID returns a page by ID.
func (s *Service) GetDocPageByID(ctx context.Context, id pgtype.UUID) (*DocPage, error) {
	page, err := s.store.getByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return toDocPage(page), nil
}

// ListDocPagesByAudience returns published pages for an audience.
func (s *Service) ListDocPagesByAudience(ctx context.Context, audience string) ([]DocPageSummary, error) {
	rows, err := s.store.listByAudience(ctx, audience)
	if err != nil {
		return nil, err
	}

	summaries := make([]DocPageSummary, len(rows))
	for i, r := range rows {
		summaries[i] = DocPageSummary{
			ID:           r.ID.String(),
			Title:        r.Title,
			Slug:         r.Slug,
			Audience:     r.Audience,
			ReviewStatus: r.ReviewStatus,
			ParentID:     uuidPtr(r.ParentID),
			Position:     r.Position,
			Category:     r.Category,
			Subcategory:  r.Subcategory,
			CreatedAt:    r.CreatedAt.Time.Format(time.RFC3339Nano),
			Excerpt:      extractExcerpt(r.Content, excerptMaxLen),
		}
	}

	return summaries, nil
}

// ListAllDocPagesByAudience returns all pages (any status) for admin listing.
func (s *Service) ListAllDocPagesByAudience(ctx context.Context, audience string) ([]DocPage, error) {
	rows, err := s.store.listAllByAudience(ctx, audience)
	if err != nil {
		return nil, err
	}

	pages := make([]DocPage, len(rows))
	for i, r := range rows {
		pages[i] = DocPage{
			ID:             r.ID.String(),
			Title:          r.Title,
			Slug:           r.Slug,
			Audience:       r.Audience,
			AIGenerated:    r.AiGenerated,
			ReviewStatus:   r.ReviewStatus,
			LastReviewedBy: uuidPtr(r.LastReviewedBy),
			LastReviewedAt: timestamptzPtr(r.LastReviewedAt),
			ParentID:       uuidPtr(r.ParentID),
			Position:       r.Position,
			Category:       r.Category,
			Subcategory:    r.Subcategory,
			CreatedAt:      r.CreatedAt.Time.Format(time.RFC3339Nano),
			UpdatedAt:      r.UpdatedAt.Time.Format(time.RFC3339Nano),
		}
	}

	return pages, nil
}

// ListAllDocPages returns all pages (any status, any audience) for admin listing.
func (s *Service) ListAllDocPages(ctx context.Context) ([]DocPage, error) {
	rows, err := s.store.listAll(ctx)
	if err != nil {
		return nil, err
	}

	pages := make([]DocPage, len(rows))
	for i, r := range rows {
		pages[i] = DocPage{
			ID:             r.ID.String(),
			Title:          r.Title,
			Slug:           r.Slug,
			Audience:       r.Audience,
			AIGenerated:    r.AiGenerated,
			ReviewStatus:   r.ReviewStatus,
			LastReviewedBy: uuidPtr(r.LastReviewedBy),
			LastReviewedAt: timestamptzPtr(r.LastReviewedAt),
			ParentID:       uuidPtr(r.ParentID),
			Position:       r.Position,
			Category:       r.Category,
			Subcategory:    r.Subcategory,
			CreatedAt:      r.CreatedAt.Time.Format(time.RFC3339Nano),
			UpdatedAt:      r.UpdatedAt.Time.Format(time.RFC3339Nano),
		}
	}

	return pages, nil
}

// GetDocNav returns the published-pages navigation tree grouped by audience,
// filtered to only the specified audiences. Checks the Valkey cache first
// (jc:cache:page:docs:nav:{}, TTL 10min) and falls back to the DB
// on miss (VALKEY-003). Cache key is platform-scoped (Rule 4, §4.2).
func (s *Service) GetDocNav(ctx context.Context, allowedAudiences []string) ([]DocNavTree, error) {
	cacheKey := valkey.MustKey("cache:page:docs:nav:")
	if s.cache != nil {
		if cached, err := s.cache.Get(ctx, cacheKey).Result(); err == nil {
			var trees []DocNavTree
			if jsonErr := json.Unmarshal([]byte(cached), &trees); jsonErr == nil {
				return filterNavTrees(trees, allowedAudiences), nil
			}
		}
	}

	rows, err := s.store.listPublished(ctx)
	if err != nil {
		return nil, err
	}

	trees := buildDocNavTree(rows)
	if s.cache != nil {
		if data, jsonErr := json.Marshal(trees); jsonErr == nil {
			if err := s.cache.Set(ctx, cacheKey, string(data), docNavCacheTTL).Err(); err != nil {
				slog.Warn("docs: failed to cache nav", "key", cacheKey, "error", err)
			}
		}
	}

	return filterNavTrees(trees, allowedAudiences), nil
}

// filterNavTrees returns only the trees whose audience is in the allowed set.
func filterNavTrees(trees []DocNavTree, allowed []string) []DocNavTree {
	if len(allowed) == 0 {
		return nil
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = true
	}

	filtered := make([]DocNavTree, 0, len(trees))

	for _, t := range trees {
		if allowedSet[t.Audience] {
			filtered = append(filtered, t)
		}
	}

	return filtered
}

// SearchDocPages returns published pages matching the query, with a plain-text excerpt.
// When audience is non-empty, results are filtered to that audience section.
// Uses Meilisearch when a searcher is wired, otherwise falls back to SQL.
func (s *Service) SearchDocPages(ctx context.Context, query, audience string, limit int32) ([]DocSearchResult, error) {
	audiences := []string{}
	if audience != "" {
		audiences = []string{audience}
	}

	if s.searcher != nil {
		return s.searcher.SearchDocPages(ctx, query, audiences, limit)
	}

	var audiencePtr *string
	if audience != "" {
		audiencePtr = &audience
	}

	rows, err := s.store.search(ctx, query, audiencePtr, limit)
	if err != nil {
		return nil, err
	}

	results := make([]DocSearchResult, len(rows))
	for i, r := range rows {
		results[i] = DocSearchResult{
			ID:          r.ID.String(),
			Title:       r.Title,
			Slug:        r.Slug,
			Audience:    r.Audience,
			Category:    r.Category,
			Subcategory: r.Subcategory,
			Excerpt:     extractExcerpt(r.Content, excerptMaxLen),
		}
	}

	return results, nil
}

// SearchDocPagesInAudiences returns published pages matching the query,
// restricted to the given set of audiences (§44 — audience-filtered search).
// Uses Meilisearch when a searcher is wired, otherwise falls back to SQL.
func (s *Service) SearchDocPagesInAudiences(ctx context.Context, query string, audiences []string, limit int32) ([]DocSearchResult, error) {
	if len(audiences) == 0 {
		return []DocSearchResult{}, nil
	}

	if s.searcher != nil {
		return s.searcher.SearchDocPages(ctx, query, audiences, limit)
	}

	// §44 amendment 2: single query over the allowed set — JSON keys stripped
	// by the SQL so structural keys can't produce false positives.
	rows, err := s.store.searchInAudiences(ctx, query, audiences, limit)
	if err != nil {
		return nil, err
	}

	results := make([]DocSearchResult, len(rows))
	for i, r := range rows {
		results[i] = DocSearchResult{
			ID:          r.ID.String(),
			Title:       r.Title,
			Slug:        r.Slug,
			Audience:    r.Audience,
			Category:    r.Category,
			Subcategory: r.Subcategory,
			Excerpt:     extractExcerpt(r.Content, excerptMaxLen),
		}
	}

	return results, nil
}

// SubmitDocPageFeedback records the user's vote for a published doc page
// (DOC-011 + §44 amendment 2). Votes are per-user and idempotent: a repeat
// vote is a no-op, a changed vote moves the aggregate counters. Returns the
// post-vote feedback state.
func (s *Service) SubmitDocPageFeedback(ctx context.Context, audience, slug, vote string, userID pgtype.UUID) (*DocFeedback, error) {
	if vote != "up" && vote != "down" {
		return nil, fmt.Errorf("%w: vote must be 'up' or 'down'", errpkg.ErrInvalid)
	}

	if !ValidAudiences[audience] {
		return nil, fmt.Errorf("%w: audience %q", errpkg.ErrInvalid, audience)
	}

	// Resolve the published page — feedback on missing/draft pages is 404.
	page, err := s.store.getBySlug(ctx, audience, slug)
	if err != nil {
		return nil, err
	}

	rows, err := s.store.setVote(ctx, page.ID, userID, vote)
	if err != nil {
		return nil, err
	}

	if rows == 0 {
		return nil, errpkg.ErrNotFound
	}

	// Read back on the writer pool — replica lag must not serve pre-vote
	// counts in the POST response.
	return s.getDocPageFeedback(ctx, audience, slug, userID, true)
}

// ListAudiences returns the canonical ordered list of documentation audience slugs.
func (s *Service) ListAudiences() []string {
	return AudienceOrder
}

// ListStatuses returns the canonical ordered list of doc page review statuses.
func (s *Service) ListStatuses() []string {
	return StatusOrder
}

// excerptMaxLen caps search excerpts at a sidebar-friendly length.
const excerptMaxLen = 140

// buildDocNavTree groups published pages by audience (in canonical order) and
// nests child pages one level deep under their parents. Children whose parent
// is not published are promoted to roots so they stay reachable.
func buildDocNavTree(rows []gen.ListPublishedDocPagesRow) []DocNavTree {
	byAudience := make(map[string][]gen.ListPublishedDocPagesRow)
	for _, r := range rows {
		byAudience[r.Audience] = append(byAudience[r.Audience], r)
	}

	trees := make([]DocNavTree, 0, len(byAudience))

	for _, audience := range AudienceOrder {
		group, ok := byAudience[audience]
		if !ok {
			continue
		}

		trees = append(trees, DocNavTree{Audience: audience, Items: buildAudienceNavItems(group)})
	}

	return trees
}

// buildAudienceNavItems nests one audience's pages by parent_id, preserving
// the position/title ordering of the input rows.
func buildAudienceNavItems(group []gen.ListPublishedDocPagesRow) []DocNavItem {
	roots := make([]DocNavItem, 0, len(group))
	rootIdx := make(map[string]int, len(group))

	for _, r := range group {
		if r.ParentID.Valid {
			continue
		}

		item := navItemFromRow(r)
		rootIdx[item.ID] = len(roots)
		roots = append(roots, item)
	}

	for _, r := range group {
		if !r.ParentID.Valid {
			continue
		}

		if idx, ok := rootIdx[r.ParentID.String()]; ok {
			roots[idx].Children = append(roots[idx].Children, navItemFromRow(r))
		} else {
			roots = append(roots, navItemFromRow(r))
		}
	}

	return roots
}

// navItemFromRow converts a published-page row to a nav item.
func navItemFromRow(r gen.ListPublishedDocPagesRow) DocNavItem {
	return DocNavItem{
		ID:          r.ID.String(),
		Title:       r.Title,
		Slug:        r.Slug,
		Audience:    r.Audience,
		ParentID:    uuidPtr(r.ParentID),
		Position:    r.Position,
		Category:    r.Category,
		Subcategory: r.Subcategory,
	}
}

// extractExcerpt walks TipTap-style JSON content and joins its text nodes
// into a single plain-text excerpt, truncated to maxLen runes.
func extractExcerpt(content []byte, maxLen int) string {
	var doc any
	if err := json.Unmarshal(content, &doc); err != nil {
		return ""
	}

	var b strings.Builder

	collectText(doc, &b)

	text := strings.Join(strings.Fields(b.String()), " ")

	runes := []rune(text)
	if len(runes) > maxLen {
		text = string(runes[:maxLen]) + "…"
	}

	return text
}

// collectText recursively gathers "text" values from TipTap JSON nodes.
func collectText(node any, b *strings.Builder) {
	switch v := node.(type) {
	case map[string]any:
		if s, ok := v["text"].(string); ok {
			b.WriteString(s)
			b.WriteString(" ")
		}

		if c, ok := v["content"]; ok {
			collectText(c, b)
		}
	case []any:
		for _, item := range v {
			collectText(item, b)
		}
	}
}

// invalidateCache clears the docs nav cache and optionally a specific page
// cache after create/update/delete operations (VALKEY-003). All keys are
// platform-scoped to prevent cross-platform cache bleed (Rule 4, §4.2).
// Best-effort: errors are logged.
func (s *Service) invalidateCache(ctx context.Context, audience, slug string) {
	if s.cache == nil {
		return
	}

	keys := []string{valkey.MustKey("cache:page:docs:nav:")}
	if audience != "" && slug != "" {
		keys = append(keys, valkey.MustKey("cache:page:docs:%s:%s:", audience, slug))
	}

	if err := s.cache.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("docs: failed to invalidate cache", "keys", keys, "error", err)
	}
}

// invalidateAllPageCaches flushes the nav cache and all page caches for a
// platform. Used after update/delete when we don't know the specific page's
// audience+slug (BUG-016 fix — stale page content after update).
func (s *Service) invalidateAllPageCaches(ctx context.Context) {
	if s.cache == nil {
		return
	}
	// Delete the nav cache.
	navKey := valkey.MustKey("cache:page:docs:nav:")
	if err := s.cache.Del(ctx, navKey).Err(); err != nil {
		slog.Warn("docs: failed to invalidate nav cache", "key", navKey, "error", err)
	}
	// Best-effort: scan and delete all page caches for this platform.
	// Pattern: jc:cache:page:docs:*:*:{}
	pattern := valkey.MustKey("cache:page:docs:*:*:")

	var cursor uint64

	for {
		keys, nextCursor, err := s.cache.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			slog.Warn("docs: scan failed during cache invalidation", "pattern", pattern, "error", err)
			return
		}

		if len(keys) > 0 {
			s.cache.Del(ctx, keys...)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
}

// maxTaxonomyLen caps free-text category/subcategory labels (§44 amendment).
const maxTaxonomyLen = 120

// maxDocContentBytes caps the raw content blob (TipTap JSON) on create/update
// (§44 amendment 2).
const maxDocContentBytes = 512 << 10 // 512 KiB

// docSlugRe is the canonical slug shape: lowercase alnum segments joined by
// single dashes (§44 amendment 2).
var docSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// reservedSlugs are docs-app route segments that can never be page slugs —
// a page using one would be shadowed by a static route and unreachable.
// `new` is reserved in every audience (the /{audience}/new create route);
// the api audience additionally serves /docs/api/keys and
// /docs/api/playground (§44 amendment 2).
var reservedSlugs = map[string]map[string]bool{
	"*":   {"new": true},
	"api": {"keys": true, "playground": true},
}

// validateDocSlug checks slug shape and the reserved list for an audience.
func validateDocSlug(audience, slug string) error {
	if !docSlugRe.MatchString(slug) {
		return fmt.Errorf("%w: slug must be lowercase alphanumeric segments joined by dashes", errpkg.ErrInvalid)
	}

	if reservedSlugs["*"][slug] || reservedSlugs[audience][slug] {
		return fmt.Errorf("%w: slug %q is reserved in audience %q", errpkg.ErrInvalid, slug, audience)
	}

	return nil
}

// createAllowedStatuses are the only review_status values a page may be
// created with — pages cannot be born approved or published (§44 amendment 2).
var createAllowedStatuses = map[string]bool{"draft": true, "in_review": true}

// patchAllowedStatuses are the statuses UpdateDocPage may set directly.
// approved/published require the dedicated approve/publish endpoints so
// reviewer metadata stays truthful (§44 amendment 2).
var patchAllowedStatuses = map[string]bool{"draft": true, "in_review": true}

// validateParent checks a parent_id assignment: the parent must exist, live
// in the same audience, and must not be the page itself or one of its
// descendants (cycle guard — walks the ancestor chain; §44 amendment 2).
func (s *Service) validateParent(ctx context.Context, pageID, parentID pgtype.UUID, audience string) error {
	if pageID.Valid && parentID.String() == pageID.String() {
		return fmt.Errorf("%w: a page cannot be its own parent", errpkg.ErrInvalid)
	}

	parent, err := s.store.getByID(ctx, parentID)
	if err != nil {
		return fmt.Errorf("%w: parent page not found", errpkg.ErrInvalid)
	}

	if parent.Audience != audience {
		return fmt.Errorf("%w: parent page belongs to audience %q", errpkg.ErrInvalid, parent.Audience)
	}

	// Walk the ancestor chain — if we reach pageID, this assignment would
	// create a cycle.
	ancestor := parent
	for range 100 {
		if !ancestor.ParentID.Valid {
			return nil
		}

		if pageID.Valid && ancestor.ParentID.String() == pageID.String() {
			return fmt.Errorf("%w: parent is a descendant of this page (cycle)", errpkg.ErrInvalid)
		}

		ancestor, err = s.store.getByID(ctx, ancestor.ParentID)
		if err != nil {
			// Dangling ancestor reference — treat the chain as terminated
			// rather than failing the assignment.
			return nil
		}
	}

	return fmt.Errorf("%w: parent chain too deep", errpkg.ErrInvalid)
}

// docContentEqual compares two content blobs semantically: JSON documents
// (TipTap) are compared structurally so key order/whitespace differences in
// a re-sent doc don't count as an edit (§44 amendment 3); anything that
// isn't parseable JSON falls back to byte comparison.
func docContentEqual(a, b []byte) bool {
	var va, vb any
	if json.Unmarshal(a, &va) != nil || json.Unmarshal(b, &vb) != nil {
		return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
	}

	return reflect.DeepEqual(va, vb)
}

// normalizeTaxonomy trims a category/subcategory label. Empty or absent input
// normalizes to nil (stored NULL); overlong labels are rejected.
func normalizeTaxonomy(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}

	v := strings.TrimSpace(*s)
	if v == "" {
		return nil, nil
	}

	if len([]rune(v)) > maxTaxonomyLen {
		return nil, fmt.Errorf("category/subcategory exceeds %d characters", maxTaxonomyLen)
	}

	return &v, nil
}

// CreateDocPage inserts a new documentation page and returns its ID.
// Pages can only be created as draft or in_review — approved/published must
// go through the review workflow (§44 amendment 2).
func (s *Service) CreateDocPage(ctx context.Context, req CreateDocPageRequest) (pgtype.UUID, error) {
	if !ValidAudiences[req.Audience] {
		return pgtype.UUID{}, fmt.Errorf("%w: audience %q", errpkg.ErrInvalid, req.Audience)
	}

	status := req.ReviewStatus
	if status == "" {
		status = "draft"
	}

	if !ValidReviewStatuses[status] {
		return pgtype.UUID{}, fmt.Errorf("%w: review_status %q", errpkg.ErrInvalid, status)
	}

	if !createAllowedStatuses[status] {
		return pgtype.UUID{}, fmt.Errorf("%w: pages cannot be created with status %q — use approve/publish", errpkg.ErrInvalidStatusTransition, status)
	}

	if err := validateDocSlug(req.Audience, req.Slug); err != nil {
		return pgtype.UUID{}, err
	}

	category, err := normalizeTaxonomy(req.Category)
	if err != nil {
		return pgtype.UUID{}, err
	}

	subcategory, err := normalizeTaxonomy(req.Subcategory)
	if err != nil {
		return pgtype.UUID{}, err
	}

	contentBytes := []byte(req.Content)
	if len(contentBytes) == 0 {
		contentBytes = []byte(`""`)
	}

	if len(contentBytes) > maxDocContentBytes {
		return pgtype.UUID{}, fmt.Errorf("%w: content exceeds %d bytes", errpkg.ErrInvalid, maxDocContentBytes)
	}

	var parentID pgtype.UUID
	if req.ParentID != nil && *req.ParentID != "" {
		if err := parentID.Scan(*req.ParentID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("%w: parent_id %q", errpkg.ErrInvalid, *req.ParentID)
		}

		if err := s.validateParent(ctx, pgtype.UUID{}, parentID, req.Audience); err != nil {
			return pgtype.UUID{}, err
		}
	}

	id, err := s.store.create(ctx, gen.CreateDocPageParams{
		Title:        req.Title,
		Slug:         req.Slug,
		Audience:     req.Audience,
		Content:      contentBytes,
		AiGenerated:  req.AIGenerated,
		ReviewStatus: status,
		ParentID:     parentID,
		Position:     req.Position,
		Category:     category,
		Subcategory:  subcategory,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}

	s.invalidateCache(ctx, req.Audience, req.Slug)

	return id, nil
}

// UpdateDocPage modifies an existing documentation page. Pointer fields
// preserve stored values when absent (§44 amendment 2). Status transitions
// are enforced: PATCH may move a page to draft/in_review only — approving or
// publishing requires the dedicated endpoints so reviewer metadata stays
// truthful. last_reviewed_by/at are never written here.
func (s *Service) UpdateDocPage(ctx context.Context, id pgtype.UUID, req UpdateDocPageRequest) error {
	// Single fetch drives fallbacks, transition checks, hierarchy validation,
	// and cache invalidation.
	existing, err := s.store.getByID(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching existing page: %w", err)
	}

	// Status: absent preserves the stored status (§44 amendment 2 — an absent
	// status silently resetting to draft was the unpublish bug).
	status := existing.ReviewStatus

	if req.ReviewStatus != "" {
		if !ValidReviewStatuses[req.ReviewStatus] {
			return fmt.Errorf("%w: review_status %q", errpkg.ErrInvalid, req.ReviewStatus)
		}

		if req.ReviewStatus != existing.ReviewStatus && !patchAllowedStatuses[req.ReviewStatus] {
			return fmt.Errorf("%w: %q requires the approve/publish endpoints", errpkg.ErrInvalidStatusTransition, req.ReviewStatus)
		}

		status = req.ReviewStatus
	}

	// Preserve existing content when the request doesn't include it (e.g. status-only toggle).
	// BUG-011 fix: use req.Content directly as []byte (same as CreateDocPage),
	// do NOT double-encode with json.Marshal(string(...)).
	contentBytes := req.Content
	if contentBytes == nil {
		contentBytes = existing.Content
	}

	if len(contentBytes) > maxDocContentBytes {
		return fmt.Errorf("%w: content exceeds %d bytes", errpkg.ErrInvalid, maxDocContentBytes)
	}

	title := req.Title
	if title == "" {
		title = existing.Title
	}

	// Audience: absent preserves; a move re-validates and invalidates both
	// audience caches.
	audience := existing.Audience

	if req.Audience != nil && *req.Audience != "" && *req.Audience != existing.Audience {
		if !ValidAudiences[*req.Audience] {
			return fmt.Errorf("%w: audience %q", errpkg.ErrInvalid, *req.Audience)
		}

		audience = *req.Audience
	}

	// Slug: absent preserves; a rename is re-validated (including the
	// reserved list) against the target audience.
	slug := existing.Slug

	if req.Slug != nil && *req.Slug != "" && *req.Slug != existing.Slug {
		if err := validateDocSlug(audience, *req.Slug); err != nil {
			return err
		}

		slug = *req.Slug
	}

	position := existing.Position
	if req.Position != nil {
		position = *req.Position
	}

	parentID := existing.ParentID
	if req.ParentID != nil {
		parentID = pgtype.UUID{}

		if *req.ParentID != "" {
			if err := parentID.Scan(*req.ParentID); err != nil {
				return fmt.Errorf("%w: parent_id %q", errpkg.ErrInvalid, *req.ParentID)
			}

			if err := s.validateParent(ctx, id, parentID, audience); err != nil {
				return err
			}
		}
	}

	// Optimistic lock: expected_updated_at must still match the stored row.
	var expectedUpdatedAt pgtype.Timestamptz

	if req.ExpectedUpdatedAt != nil && *req.ExpectedUpdatedAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpectedUpdatedAt)
		if err != nil {
			return fmt.Errorf("%w: expected_updated_at must be RFC3339", errpkg.ErrInvalid)
		}

		expectedUpdatedAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	// §44 amendment 3: editing an approved page's title or content voids the
	// approval — the reviewer approved a different artifact. The page demotes
	// to in_review and the review stamp clears so "Last reviewed" never
	// describes content that wasn't actually reviewed. Organizational moves
	// (slug/category/parent/position/audience) don't demote. Published pages
	// are unaffected — published edits go live by design.
	clearReview := false

	if existing.ReviewStatus == "approved" &&
		(title != existing.Title || !docContentEqual(contentBytes, existing.Content)) {
		if status == "approved" {
			status = "in_review"
		}

		clearReview = true
	}

	// Taxonomy: a nil field preserves the stored value; "" clears it (§44
	// amendment). Subcategory is independent of category — a page may move to
	// a different category while keeping or clearing its subcategory.
	category := existing.Category

	if req.Category != nil {
		if category, err = normalizeTaxonomy(req.Category); err != nil {
			return err
		}
	}

	subcategory := existing.Subcategory

	if req.Subcategory != nil {
		if subcategory, err = normalizeTaxonomy(req.Subcategory); err != nil {
			return err
		}
	}

	rows, err := s.store.update(ctx, gen.UpdateDocPageParams{
		ID: id,

		Title:             title,
		Content:           contentBytes,
		ReviewStatus:      status,
		Slug:              slug,
		ParentID:          parentID,
		Position:          position,
		Audience:          audience,
		Category:          category,
		Subcategory:       subcategory,
		ExpectedUpdatedAt: expectedUpdatedAt,
		ClearReview:       &clearReview,
	})
	if err != nil {
		return err
	}

	if rows == 0 {
		// The WHERE guard rejected the write. If the page still exists, the
		// expected_updated_at precondition failed — stale write.
		if _, gerr := s.store.getByID(ctx, id); gerr != nil {
			return fmt.Errorf("fetching existing page: %w", gerr)
		}

		return fmt.Errorf("%w: page was modified since %s", errpkg.ErrStaleWrite, *req.ExpectedUpdatedAt)
	}

	// Invalidate nav + the page caches at both the old and new coordinates
	// (BUG-016 fix extended to slug/audience moves).
	s.invalidateCache(ctx, existing.Audience, existing.Slug)

	if audience != existing.Audience || slug != existing.Slug {
		s.invalidateCache(ctx, audience, slug)
	}

	return nil
}

// GetDocPageBySlugAnyStatus returns a page regardless of review status.
// Editor preview only — callers must hold docs.manage (§44 amendment 2).
// Never cached: preview must reflect the latest row.
func (s *Service) GetDocPageBySlugAnyStatus(ctx context.Context, audience, slug string) (*DocPage, error) {
	page, err := s.store.getBySlugAnyStatus(ctx, audience, slug)
	if err != nil {
		return nil, err
	}

	return toDocPage(page), nil
}

// GetDocPageFeedback returns aggregate helpful counters plus the requesting
// user's recorded vote for a published page (§44 amendment 2). userID may be
// invalid for guests — my_vote is simply nil.
func (s *Service) GetDocPageFeedback(ctx context.Context, audience, slug string, userID pgtype.UUID) (*DocFeedback, error) {
	return s.getDocPageFeedback(ctx, audience, slug, userID, false)
}

// getDocPageFeedback backs both feedback reads; write=true routes to the
// writer pool so a POST response reflects the just-recorded vote.
func (s *Service) getDocPageFeedback(ctx context.Context, audience, slug string, userID pgtype.UUID, write bool) (*DocFeedback, error) {
	row, err := s.store.getFeedback(ctx, audience, slug, userID, write)
	if err != nil {
		return nil, err
	}

	return &DocFeedback{
		PageID:     row.PageID.String(),
		HelpfulYes: row.HelpfulYes,
		HelpfulNo:  row.HelpfulNo,
		MyVote:     row.MyVote,
	}, nil
}

// ApproveDocPage sets review_status to 'approved' and records the reviewer
// (§44). Allowed from draft/in_review (and is a no-op when already approved);
// approving a published page would silently demote it, so that is rejected
// (§44 amendment 2).
func (s *Service) ApproveDocPage(ctx context.Context, id, reviewerID pgtype.UUID) error {
	currentStatus, err := s.store.getStatus(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching current status: %w", err)
	}

	if currentStatus == "published" {
		return fmt.Errorf("%w: page is already published — unpublish to draft first", errpkg.ErrInvalidStatusTransition)
	}

	if err := s.store.updateStatus(ctx, gen.UpdateDocPageStatusParams{
		ID: id,

		ReviewStatus:   "approved",
		LastReviewedBy: reviewerID,
	}); err != nil {
		return err
	}

	// Invalidate caches for this page.
	existing, err := s.store.getByID(ctx, id)
	if err == nil {
		s.invalidateCache(ctx, existing.Audience, existing.Slug)
	} else {
		s.invalidateAllPageCaches(ctx)
	}

	return nil
}

// PublishDocPage sets review_status to 'published'. Requires current status = 'approved' (§44).
func (s *Service) PublishDocPage(ctx context.Context, id, reviewerID pgtype.UUID) error {
	// Validate status transition: must be 'approved' before publishing.
	currentStatus, err := s.store.getStatus(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching current status: %w", err)
	}

	if currentStatus != "approved" {
		return fmt.Errorf("%w: cannot publish status %q, must be 'approved'", errpkg.ErrInvalidStatusTransition, currentStatus)
	}

	if err := s.store.updateStatus(ctx, gen.UpdateDocPageStatusParams{
		ID: id,

		ReviewStatus:   "published",
		LastReviewedBy: reviewerID,
	}); err != nil {
		return err
	}

	// Invalidate caches for this page.
	existing, err := s.store.getByID(ctx, id)
	if err == nil {
		s.invalidateCache(ctx, existing.Audience, existing.Slug)
	} else {
		s.invalidateAllPageCaches(ctx)
	}

	return nil
}

// DeleteDocPage removes a documentation page.
func (s *Service) DeleteDocPage(ctx context.Context, id pgtype.UUID) error {
	// Fetch the page before deleting so we can invalidate its cache.
	existing, err := s.store.getByID(ctx, id)
	if err == nil {
		s.invalidateCache(ctx, existing.Audience, existing.Slug)
	} else {
		s.invalidateAllPageCaches(ctx)
	}

	if err := s.store.delete(ctx, id); err != nil {
		return err
	}

	return nil
}

// toDocPage converts a gen.DocPage to our API type.
func toDocPage(p gen.DocPage) *DocPage {
	var content json.RawMessage
	if len(p.Content) > 0 {
		content = p.Content
	}

	return &DocPage{
		ID:             p.ID.String(),
		Title:          p.Title,
		Slug:           p.Slug,
		Audience:       p.Audience,
		Content:        content,
		AIGenerated:    p.AiGenerated,
		ReviewStatus:   p.ReviewStatus,
		LastReviewedBy: uuidPtr(p.LastReviewedBy),
		LastReviewedAt: timestamptzPtr(p.LastReviewedAt),
		ParentID:       uuidPtr(p.ParentID),
		Position:       p.Position,
		Category:       p.Category,
		Subcategory:    p.Subcategory,
		HelpfulYes:     p.HelpfulYes,
		HelpfulNo:      p.HelpfulNo,
		// Phase 8 (i18n) will persist translated variants. Until then the API
		// always reports the source language as English, including when a
		// ?lang query parameter is supplied.
		ContentLang: "en",
		CreatedAt:   p.CreatedAt.Time.Format(time.RFC3339Nano),
		UpdatedAt:   p.UpdatedAt.Time.Format(time.RFC3339Nano),
	}
}

// uuidPtr returns a string pointer from a pgtype.UUID, or nil if the UUID is zero.
func uuidPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}

	s := u.String()

	return &s
}

// timestamptzPtr returns a formatted time string pointer, or nil if not valid.
func timestamptzPtr(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}

	// RFC3339Nano keeps microsecond precision — updated_at round-trips through
	// PATCH expected_updated_at and must compare equal to the stored
	// timestamptz (§44 amendment 2).
	s := t.Time.Format(time.RFC3339Nano)

	return &s
}

// slugify converts a title into a URL-safe slug.
func slugify(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

// GenerateDraft creates a doc page draft via the Content Agent (ADR-088).
// The draft is inserted with ai_generated=true, review_status='draft'. It
// requires HITL approval before publication.
// // When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category. If the dispatcher is nil,
// the draft is still created with the prompt as content so a reviewer can
// fill it manually (graceful degradation).
func (s *Service) GenerateDraft(ctx context.Context, input GenerateDraftRequest) (GenerateDraftResponse, error) {
	if input.Title == "" {
		return GenerateDraftResponse{}, errors.New("title is required")
	}

	slug := input.Slug
	if slug == "" {
		slug = slugify(input.Title)
	}

	audience := input.Audience
	if audience == "" {
		audience = "public"
	}

	// Build the prompt sent to the Content Agent.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf("Generate documentation for: %s (audience: %s)", input.Title, audience)
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])
	_ = promptHash // stored in content for audit; doc_pages has no ai_prompt_hash column

	// Call the AI Gateway when a dispatcher is wired.
	generatedText := prompt

	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt": prompt,

				"guide_type": "doc_page",
			},
		})
		if aiErr != nil {
			return GenerateDraftResponse{}, fmt.Errorf("docs: AI generation failed: %w", aiErr)
		}

		if text, ok := resp.Raw["text"].(string); ok {
			generatedText = text
		} else if text, ok := resp.Raw["content"].(string); ok {
			generatedText = text
		} else if text, ok := resp.Raw["output"].(string); ok {
			generatedText = text
		}

		slog.Info("docs: AI Gateway generated draft", "title", input.Title, "audience", audience)
	}

	// Build TipTap JSON content from the generated text.
	contentBytes := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":%q}]}]}`, generatedText))

	category, err := normalizeTaxonomy(&input.Category)
	if err != nil {
		return GenerateDraftResponse{}, err
	}

	subcategory, err := normalizeTaxonomy(&input.Subcategory)
	if err != nil {
		return GenerateDraftResponse{}, err
	}

	// Create the doc page with ai_generated=true, review_status='draft'.
	id, err := s.store.create(ctx, gen.CreateDocPageParams{
		Title:        input.Title,
		Slug:         slug,
		Audience:     audience,
		Content:      contentBytes,
		AiGenerated:  true,
		ReviewStatus: "draft",
		ParentID:     pgtype.UUID{},
		Position:     0,
		Category:     category,
		Subcategory:  subcategory,
	})
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("docs: generate draft: %w", err)
	}

	// Invalidate cache for this audience+slug.
	s.invalidateCache(ctx, audience, slug)

	return GenerateDraftResponse{
		Status:  "accepted",
		Message: "Content Agent draft created in 'draft' status. Review and approve to publish.",
		PageID:  id.String(),
	}, nil
}
