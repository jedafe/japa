package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// sourceItem is a single item in a source's feed. It is intentionally a
// flattened, UI-ready shape so the source detail page and the tools can render
// it without domain-specific knowledge.
type sourceItem struct {
	ID           string             `json:"id"`
	EntityType   string             `json:"entity_type"`
	Title        string             `json:"title"`
	Excerpt      *string            `json:"excerpt,omitempty"`
	OriginalURL  string             `json:"original_url"`
	Href         string             `json:"href"`
	PublishedAt  pgtype.Timestamptz `json:"published_at"`
	ViewCount    int32              `json:"view_count"`
	CommentCount int32              `json:"comment_count"`
	ShareCount   int32              `json:"share_count"`
	VoteScore    int32              `json:"vote_score"`
}

// sourceItemEnvelope is the response payload for the source item feed.
type sourceItemEnvelope struct {
	Items      []sourceItem   `json:"items"`
	Meta       sourceListMeta `json:"meta"`
	Sort       string         `json:"sort"`
	EntityType string         `json:"entity_type"`
}

// HandleListSourceItems returns the published items for a source as a
// domain-agnostic feed. Each item links to the internal JapaConnect page and
// carries the original publisher URL.
//
// GET /api/v1/sources/{slug}/items
func (h *Handlers) HandleListSourceItems(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = h.defaultTenant
	}

	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Source slug is required")
		return
	}

	src, err := h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), tenantID, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "source_not_found", "Source not found")
			return
		}

		slog.Error("intel: source items lookup failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source")
		return
	}

	q := r.URL.Query()
	entityType := q.Get("entity_type")
	sortMode := q.Get("sort")
	if sortMode == "" {
		sortMode = "latest"
	}

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	records, err := h.intelStores.ListIntelRecordsBySource(r.Context(), tenantID, src.ID, entityType, clampInt32(limit), offset)
	if err != nil {
		slog.Error("intel: source records list failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
		return
	}

	items := make([]sourceItem, 0, len(records))
	for _, rec := range records {
		item, ok, convErr := h.recordToSourceItem(r.Context(), tenantID, rec)
		if convErr != nil {
			slog.Warn("intel: source item conversion failed", "record_id", rec.ID, "entity_type", rec.EntityType, "error", convErr)
			continue
		}
		if ok {
			items = append(items, item)
		}
	}

	sortSourceItems(items, sortMode)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": sourceItemEnvelope{
			Items:      items,
			Meta:       sourceListMeta{PageSize: limit, PageOffset: (page - 1) * limit, Total: int64(len(items))},
			Sort:       sortMode,
			EntityType: entityType,
		},
	})
}

// recordToSourceItem resolves a published intel_record to a UI-ready item.
// Returns the item, a boolean indicating success, and an error if resolution
// failed in an unexpected way.
func (h *Handlers) recordToSourceItem(ctx context.Context, tenantID pgtype.UUID, rec gen.IntelRecord) (sourceItem, bool, error) {
	switch rec.EntityType {
	case "scholarship":
		item, ok, err := h.scholarshipRecordToSourceItem(ctx, tenantID, rec)
		return item, ok, err
	case "job":
		item, ok, err := h.jobRecordToSourceItem(ctx, tenantID, rec)
		return item, ok, err
	case "remittance":
		item, ok, err := h.remittanceRecordToSourceItem(ctx, tenantID, rec)
		return item, ok, err
	case "news", "culture_lifestyle", "guide", "blog", "article":
		item, ok, err := h.contentRecordToSourceItem(ctx, tenantID, rec)
		return item, ok, err
	case "wiki_page":
		item, ok, err := h.wikiRecordToSourceItem(ctx, tenantID, rec)
		return item, ok, err
	default:
		return sourceItem{}, false, nil
	}
}

func (h *Handlers) scholarshipRecordToSourceItem(ctx context.Context, tenantID pgtype.UUID, rec gen.IntelRecord) (sourceItem, bool, error) {
	sch, err := h.intelStores.GetScholarshipByIntelRecord(ctx, tenantID, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	href := "/scholarships/" + pgtypeUUIDString(sch.GuideID)
	if content, err := h.intelStores.GetContentByID(ctx, tenantID, sch.GuideID); err == nil && content.Slug != "" {
		href = "/scholarships/" + content.Slug
	}

	publishedAt := sch.CreatedAt
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = rec.ExtractedAt
	}

	return sourceItem{
		ID:          pgtypeUUIDString(sch.ID),
		EntityType:  "scholarship",
		Title:       sch.Title,
		Excerpt:     sch.Description,
		OriginalURL: sch.SourceUrl,
		Href:        href,
		PublishedAt: publishedAt,
	}, true, nil
}

func (h *Handlers) jobRecordToSourceItem(ctx context.Context, tenantID pgtype.UUID, rec gen.IntelRecord) (sourceItem, bool, error) {
	job, err := h.intelStores.GetIntelJobByIntelRecord(ctx, tenantID, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	href := "/jobs/" + pgtypeUUIDString(job.GuideID)
	if content, err := h.intelStores.GetContentByID(ctx, tenantID, job.GuideID); err == nil && content.Slug != "" {
		href = "/jobs/" + content.Slug
	}

	publishedAt := job.CreatedAt
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = rec.ExtractedAt
	}

	excerpt := ""
	if job.Employer != nil {
		excerpt = *job.Employer
	}
	if job.Description != nil {
		if excerpt != "" {
			excerpt += " - "
		}
		excerpt += *job.Description
	}

	var excerptPtr *string
	if excerpt != "" {
		excerptPtr = &excerpt
	}

	return sourceItem{
		ID:          pgtypeUUIDString(job.ID),
		EntityType:  "job",
		Title:       job.Title,
		Excerpt:     excerptPtr,
		OriginalURL: job.SourceUrl,
		Href:        href,
		PublishedAt: publishedAt,
	}, true, nil
}

func (h *Handlers) remittanceRecordToSourceItem(ctx context.Context, tenantID pgtype.UUID, rec gen.IntelRecord) (sourceItem, bool, error) {
	rate, err := h.intelStores.GetRemittanceRateByIntelRecord(ctx, tenantID, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	originalURL := ""
	if rate.SourceUrl != nil {
		originalURL = *rate.SourceUrl
	}

	title := rate.SendCurrency + " -> " + rate.ReceiveCurrency

	publishedAt := rate.FetchedAt
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = rec.ExtractedAt
	}

	return sourceItem{
		ID:          pgtypeUUIDString(rate.ID),
		EntityType:  "remittance",
		Title:       title,
		OriginalURL: originalURL,
		Href:        "/tools/remittance",
		PublishedAt: publishedAt,
	}, true, nil
}

func (h *Handlers) contentRecordToSourceItem(ctx context.Context, tenantID pgtype.UUID, rec gen.IntelRecord) (sourceItem, bool, error) {
	content, err := h.intelStores.GetContentByIntelRecord(ctx, tenantID, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	originalURL := ""
	if content.CanonicalUrl != nil {
		originalURL = *content.CanonicalUrl
	}

	publishedAt := content.PublishedAt
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = content.PublishAt
	}
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = rec.ExtractedAt
	}

	excerpt := excerptFromBody(content.Body, 200)
	if excerpt == "" && content.MetaDescription != nil {
		excerpt = *content.MetaDescription
	}

	var excerptPtr *string
	if excerpt != "" {
		excerptPtr = &excerpt
	}

	return sourceItem{
		ID:           pgtypeUUIDString(content.ID),
		EntityType:   content.ContentType,
		Title:        content.Title,
		Excerpt:      excerptPtr,
		OriginalURL:  originalURL,
		Href:         "/content/" + content.Slug,
		PublishedAt:  publishedAt,
		ViewCount:    content.ViewCount,
		CommentCount: content.CommentCount,
		ShareCount:   content.ShareCount,
	}, true, nil
}

func (h *Handlers) wikiRecordToSourceItem(ctx context.Context, tenantID pgtype.UUID, rec gen.IntelRecord) (sourceItem, bool, error) {
	page, err := h.intelStores.GetWikiPageByIntelRecord(ctx, tenantID, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	return sourceItem{
		ID:          pgtypeUUIDString(page.ID),
		EntityType:  "wiki_page",
		Title:       page.Title,
		Href:        "/wiki/" + page.Slug,
		PublishedAt: rec.ExtractedAt,
	}, true, nil
}

// sortSourceItems sorts the slice in place according to the requested mode.
// Secondary sort is always by published_at desc so ties and zero-engagement
// items do not return in random order.
func sortSourceItems(items []sourceItem, sortMode string) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]

		switch sortMode {
		case "popular":
			if a.ViewCount != b.ViewCount {
				return a.ViewCount > b.ViewCount
			}
		case "discussed":
			if a.CommentCount != b.CommentCount {
				return a.CommentCount > b.CommentCount
			}
		case "upvoted":
			if a.VoteScore != b.VoteScore {
				return a.VoteScore > b.VoteScore
			}
		default: // latest
			if a.PublishedAt.Valid && b.PublishedAt.Valid {
				return a.PublishedAt.Time.After(b.PublishedAt.Time)
			}
			return a.PublishedAt.Valid
		}

		// Secondary: published_at desc.
		if a.PublishedAt.Valid && b.PublishedAt.Valid {
			return a.PublishedAt.Time.After(b.PublishedAt.Time)
		}
		return a.PublishedAt.Valid
	})
}

// excerptFromBody returns a short plain-text excerpt from a markdown body,
// truncated to maxLen runes. It is intentionally simple.
func excerptFromBody(body string, maxLen int) string {
	if body == "" {
		return ""
	}

	text := body
	text = strings.ReplaceAll(text, "#", "")
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "[", "")
	text = strings.ReplaceAll(text, "]", "")
	text = strings.ReplaceAll(text, "(", "")
	text = strings.ReplaceAll(text, ")", "")
	text = strings.ReplaceAll(text, "!", "")

	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	text = strings.TrimSpace(text)

	if len(text) > maxLen {
		return text[:maxLen]
	}
	return text
}
