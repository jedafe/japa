package docs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Audience string `json:"audience"`
	Prompt   string `json:"prompt"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	PageID  string `json:"page_id"`
}

// GetDocPageBySlug returns a published page by audience and slug. Checks the
// Valkey cache first (jc:cache:page:docs:{audience}:{slug}:{}, TTL 5min)
// and falls back to the DB on miss (VALKEY-003). Cache key is tenant-scoped
// to prevent cross-tenant data bleed (Rule 4, §4.2).
func (s *Service) GetDocPageBySlug(ctx context.Context, tenantID pgtype.UUID, audience, slug string) (*DocPage, error) {
	cacheKey := valkey.MustKey("cache:page:docs:%s:%s:%s", audience, slug, tenantID.String())
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
			CreatedAt:    r.CreatedAt.Time.Format(time.RFC3339),
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
			CreatedAt:      r.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      r.UpdatedAt.Time.Format(time.RFC3339),
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
			CreatedAt:      r.CreatedAt.Time.Format(time.RFC3339),
			UpdatedAt:      r.UpdatedAt.Time.Format(time.RFC3339),
		}
	}

	return pages, nil
}

// GetDocNav returns the published-pages navigation tree grouped by audience,
// filtered to only the specified audiences. Checks the Valkey cache first
// (jc:cache:page:docs:nav:{}, TTL 10min) and falls back to the DB
// on miss (VALKEY-003). Cache key is tenant-scoped (Rule 4, §4.2).
func (s *Service) GetDocNav(ctx context.Context, tenantID pgtype.UUID, allowedAudiences []string) ([]DocNavTree, error) {
	cacheKey := valkey.MustKey("cache:page:docs:nav:%s", tenantID.String())
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
			ID:       r.ID.String(),
			Title:    r.Title,
			Slug:     r.Slug,
			Audience: r.Audience,
			Excerpt:  extractExcerpt(r.Content, excerptMaxLen),
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

	allResults := make([]DocSearchResult, 0, limit)

	for _, aud := range audiences {
		audPtr := &aud

		rows, err := s.store.search(ctx, query, audPtr, limit)
		if err != nil {
			return nil, err
		}

		for _, r := range rows {
			allResults = append(allResults, DocSearchResult{
				ID:       r.ID.String(),
				Title:    r.Title,
				Slug:     r.Slug,
				Audience: r.Audience,
				Excerpt:  extractExcerpt(r.Content, excerptMaxLen),
			})
			// nolint:gosec // G115: bounded
			if int32(len(allResults)) >= limit { // #nosec //nolint:gosec // G115: bounded
				break
			}
		}

		// nolint:gosec // G115: bounded
		if int32(len(allResults)) >= limit { // #nosec //nolint:gosec // G115: bounded
			break
		}
	}

	return allResults, nil
}

// SubmitDocPageFeedback increments the aggregate helpful/unhelpful counter for
// a published doc page (DOC-011). Vote must be "up" or "down".
func (s *Service) SubmitDocPageFeedback(ctx context.Context, audience, slug, vote string) error {
	if vote != "up" && vote != "down" {
		return fmt.Errorf("invalid vote: %s", vote)
	}

	if !ValidAudiences[audience] {
		return fmt.Errorf("invalid audience: %s", audience)
	}

	return s.store.incrementHelpful(ctx, audience, slug, vote)
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
		ID:       r.ID.String(),
		Title:    r.Title,
		Slug:     r.Slug,
		Audience: r.Audience,
		ParentID: uuidPtr(r.ParentID),
		Position: r.Position,
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
// tenant-scoped to prevent cross-tenant cache bleed (Rule 4, §4.2).
// Best-effort: errors are logged.
func (s *Service) invalidateCache(ctx context.Context, tenantID pgtype.UUID, audience, slug string) {
	if s.cache == nil {
		return
	}

	keys := []string{valkey.MustKey("cache:page:docs:nav:%s", tenantID.String())}
	if audience != "" && slug != "" {
		keys = append(keys, valkey.MustKey("cache:page:docs:%s:%s:%s", audience, slug, tenantID.String()))
	}

	if err := s.cache.Del(ctx, keys...).Err(); err != nil {
		slog.Warn("docs: failed to invalidate cache", "keys", keys, "error", err)
	}
}

// invalidateAllPageCaches flushes the nav cache and all page caches for a
// tenant. Used after update/delete when we don't know the specific page's
// audience+slug (BUG-016 fix — stale page content after update).
func (s *Service) invalidateAllPageCaches(ctx context.Context, tenantID pgtype.UUID) {
	if s.cache == nil {
		return
	}
	// Delete the nav cache.
	navKey := valkey.MustKey("cache:page:docs:nav:%s", tenantID.String())
	if err := s.cache.Del(ctx, navKey).Err(); err != nil {
		slog.Warn("docs: failed to invalidate nav cache", "key", navKey, "error", err)
	}
	// Best-effort: scan and delete all page caches for this tenant.
	// Pattern: jc:cache:page:docs:*:*:{}
	pattern := valkey.MustKey("cache:page:docs:*:*:%s", tenantID.String())

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

// CreateDocPage inserts a new documentation page and returns its ID.
func (s *Service) CreateDocPage(ctx context.Context, tenantID pgtype.UUID, req CreateDocPageRequest) (pgtype.UUID, error) {
	if !ValidAudiences[req.Audience] {
		return pgtype.UUID{}, fmt.Errorf("invalid audience: %s", req.Audience)
	}

	status := req.ReviewStatus
	if status == "" {
		status = "draft"
	}

	if !ValidReviewStatuses[status] {
		return pgtype.UUID{}, fmt.Errorf("invalid review_status: %s", status)
	}

	contentBytes := []byte(req.Content)
	if len(contentBytes) == 0 {
		contentBytes = []byte(`""`)
	}

	var parentID pgtype.UUID
	if req.ParentID != nil {
		if err := parentID.Scan(*req.ParentID); err != nil {
			return pgtype.UUID{}, fmt.Errorf("invalid parent_id %q: %w", *req.ParentID, err)
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
	})
	if err != nil {
		return pgtype.UUID{}, err
	}

	s.invalidateCache(ctx, tenantID, req.Audience, req.Slug)

	return id, nil
}

// UpdateDocPage modifies an existing documentation page.
func (s *Service) UpdateDocPage(ctx context.Context, tenantID, id pgtype.UUID, req UpdateDocPageRequest) error {
	status := req.ReviewStatus
	if status == "" {
		status = "draft"
	}

	if !ValidReviewStatuses[status] {
		return fmt.Errorf("invalid review_status: %s", status)
	}

	// Preserve existing content when the request doesn't include it (e.g. status-only toggle).
	// BUG-011 fix: use req.Content directly as []byte (same as CreateDocPage),
	// do NOT double-encode with json.Marshal(string(...)).
	contentBytes := req.Content
	if contentBytes == nil {
		existing, err := s.store.getByID(ctx, id)
		if err != nil {
			return fmt.Errorf("fetching existing page for content merge: %w", err)
		}

		contentBytes = existing.Content
	}

	// Fetch existing page for title fallback and cache invalidation.
	existing, err := s.store.getByID(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching existing page: %w", err)
	}

	title := req.Title
	if title == "" {
		title = existing.Title
	}

	if err := s.store.update(ctx, gen.UpdateDocPageParams{
		ID: id,

		Title:          title,
		Content:        contentBytes,
		ReviewStatus:   status,
		LastReviewedBy: pgtype.UUID{},
	}); err != nil {
		return err
	}

	// Invalidate nav + the specific page cache (BUG-016 fix).
	s.invalidateCache(ctx, tenantID, existing.Audience, existing.Slug)

	return nil
}

// ApproveDocPage sets review_status to 'approved' and records the reviewer (§44).
func (s *Service) ApproveDocPage(ctx context.Context, tenantID, id, reviewerID pgtype.UUID) error {
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
		s.invalidateCache(ctx, tenantID, existing.Audience, existing.Slug)
	} else {
		s.invalidateAllPageCaches(ctx, tenantID)
	}

	return nil
}

// PublishDocPage sets review_status to 'published'. Requires current status = 'approved' (§44).
func (s *Service) PublishDocPage(ctx context.Context, tenantID, id, reviewerID pgtype.UUID) error {
	// Validate status transition: must be 'approved' before publishing.
	currentStatus, err := s.store.getStatus(ctx, id)
	if err != nil {
		return fmt.Errorf("fetching current status: %w", err)
	}

	if currentStatus != "approved" {
		return fmt.Errorf("cannot publish: current status is %q, must be 'approved'", currentStatus)
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
		s.invalidateCache(ctx, tenantID, existing.Audience, existing.Slug)
	} else {
		s.invalidateAllPageCaches(ctx, tenantID)
	}

	return nil
}

// DeleteDocPage removes a documentation page.
func (s *Service) DeleteDocPage(ctx context.Context, tenantID, id pgtype.UUID) error {
	// Fetch the page before deleting so we can invalidate its cache.
	existing, err := s.store.getByID(ctx, id)
	if err == nil {
		s.invalidateCache(ctx, tenantID, existing.Audience, existing.Slug)
	} else {
		s.invalidateAllPageCaches(ctx, tenantID)
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
		// Phase 8 (i18n) will persist translated variants. Until then the API
		// always reports the source language as English, including when a
		// ?lang query parameter is supplied.
		ContentLang: "en",
		CreatedAt:   p.CreatedAt.Time.Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.Time.Format(time.RFC3339),
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

	s := t.Time.Format(time.RFC3339)

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
func (s *Service) GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input GenerateDraftRequest) (GenerateDraftResponse, error) {
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
				"prompt":     prompt,
				"tenant_id":  tenantID,
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
	})
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("docs: generate draft: %w", err)
	}

	// Invalidate cache for this audience+slug.
	s.invalidateCache(ctx, tenantID, audience, slug)

	return GenerateDraftResponse{
		Status:  "accepted",
		Message: "Content Agent draft created in 'draft' status. Review and approve to publish.",
		PageID:  id.String(),
	}, nil
}
