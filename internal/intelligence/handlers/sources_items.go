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
)

// sourceItem is a single item in a source's feed. It is intentionally a
// flattened, UI-ready shape so the source detail page and the tools can render
// it without domain-specific knowledge.
type sourceItem struct {
	ID           string             `json:"id"`
	ContentID    string             `json:"content_id,omitempty"`
	EntityType   string             `json:"entity_type"`
	Title        string             `json:"title"`
	Excerpt      *string            `json:"excerpt,omitempty"`
	OriginalURL  string             `json:"original_url"`
	Href         string             `json:"href"`
	ThumbnailURL *string            `json:"thumbnail_url,omitempty"`
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

	userID, authed := optionalUser(w, r)

	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Source slug is required")
		return
	}

	src, err := h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), slug)
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

	// If the caller has blocked this source, return an empty feed (v2 Task 2).
	if authed && h.intelSources != nil {
		blocked, err := h.intelSources.IsSourceBlocked(r.Context(), userID, src.ID)
		if err != nil {
			slog.Error("intel: source items block check failed", "slug", slug, "error", err)
		} else if blocked {
			errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"data": sourceItemEnvelope{
					Items:      []sourceItem{},
					Meta:       sourceListMeta{PageSize: limit, PageOffset: int(offset), Total: 0},
					Sort:       sortMode,
					EntityType: entityType,
				},
			})
			return
		}
	}

	pageOffset := (page - 1) * limit

	// v2 source directory: the unfiltered feed merges intel_records with
	// linked video_sources and comparison contents for EVERY sort mode —
	// previously only "latest" took this path, so video/comparison-backed
	// sources returned an empty feed under popular/discussed/upvoted. The
	// merge fetches (offset + limit) items from each source (each sorted at
	// the DB level by the requested mode), then re-sorts and slices in
	// memory. To keep the DB cost bounded, this mixed path is only used when
	// the requested window fits inside max_page_size *
	// mixed_fetch_page_multiplier.
	if entityType == "" && h.canUseMixedFetch(r.Context(), pageOffset+limit) {
		items, total, err := h.listMixedSourceItems(r.Context(), w, r, src.ID, page, limit, pageOffset, sortMode)
		if err != nil {
			return
		}

		h.resolveLiterals(r, &items)

		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": sourceItemEnvelope{
				Items:      items,
				Meta:       sourceListMeta{PageSize: limit, PageOffset: pageOffset, Total: total},
				Sort:       sortMode,
				EntityType: entityType,
			},
		})
		return
	}

	// v2 source directory: if the source is a linked video source, the item
	// feed is served directly from video_sources for the "video" entity type.
	if entityType == "video" && h.videos != nil {
		videos, total, err := h.videos.ListItemsBySourceID(r.Context(), src.ID, sortMode, clampInt32(limit), offset)
		if err != nil {
			slog.Error("intel: source video items list failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
			return
		}

		items := make([]sourceItem, 0, len(videos))
		for _, v := range videos {
			items = append(items, videoSourceToItem(v))
		}

		h.resolveLiterals(r, &items)

		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": sourceItemEnvelope{
				Items:      items,
				Meta:       sourceListMeta{PageSize: limit, PageOffset: (page - 1) * limit, Total: total},
				Sort:       sortMode,
				EntityType: entityType,
			},
		})
		return
	}

	// v2 source directory: if the source is a linked comparison data source,
	// the item feed is served directly from contents for the "comparison"
	// entity type.
	if entityType == "comparison" && h.contents != nil {
		comparisons, total, err := h.contents.ListComparisonContentsBySourceID(r.Context(), src.ID, sortMode, clampInt32(limit), offset)
		if err != nil {
			slog.Error("intel: source comparison items list failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
			return
		}

		items := make([]sourceItem, 0, len(comparisons))
		for _, c := range comparisons {
			items = append(items, comparisonContentToItem(c))
		}

		h.resolveLiterals(r, &items)

		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": sourceItemEnvelope{
				Items:      items,
				Meta:       sourceListMeta{PageSize: limit, PageOffset: (page - 1) * limit, Total: total},
				Sort:       sortMode,
				EntityType: entityType,
			},
		})
		return
	}

	records, err := h.intelStores.ListIntelRecordsBySource(r.Context(), src.ID, entityType, sortMode, clampInt32(limit), offset)
	if err != nil {
		slog.Error("intel: source records list failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
		return
	}

	total, err := h.intelStores.CountIntelRecordsBySource(r.Context(), src.ID, entityType)
	if err != nil {
		slog.Error("intel: source records count failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
		return
	}

	items := make([]sourceItem, 0, len(records))
	for _, rec := range records {
		item, ok, convErr := h.recordToSourceItem(r.Context(), rec)
		if convErr != nil {
			slog.Warn("intel: source item conversion failed", "record_id", rec.ID, "entity_type", rec.EntityType, "error", convErr)
			continue
		}
		if ok {
			items = append(items, item)
		}
	}

	h.resolveLiterals(r, &items)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": sourceItemEnvelope{
			Items:      items,
			Meta:       sourceListMeta{PageSize: limit, PageOffset: (page - 1) * limit, Total: total},
			Sort:       sortMode,
			EntityType: entityType,
		},
	})
}

// recordToSourceItem resolves a published intel_record to a UI-ready item.
// Returns the item, a boolean indicating success, and an error if resolution
// failed in an unexpected way.
func (h *Handlers) recordToSourceItem(ctx context.Context, rec gen.IntelRecord) (sourceItem, bool, error) {
	switch rec.EntityType {
	case "scholarship":
		item, ok, err := h.scholarshipRecordToSourceItem(ctx, rec)
		return item, ok, err
	case "job":
		item, ok, err := h.jobRecordToSourceItem(ctx, rec)
		return item, ok, err
	case "remittance":
		item, ok, err := h.remittanceRecordToSourceItem(ctx, rec)
		return item, ok, err
	case "news", "culture_lifestyle", "guide", "blog", "article":
		item, ok, err := h.contentRecordToSourceItem(ctx, rec)
		return item, ok, err
	case "wiki_page":
		item, ok, err := h.wikiRecordToSourceItem(ctx, rec)
		return item, ok, err
	default:
		return sourceItem{}, false, nil
	}
}

func (h *Handlers) scholarshipRecordToSourceItem(ctx context.Context, rec gen.IntelRecord) (sourceItem, bool, error) {
	sch, err := h.intelStores.GetScholarshipByIntelRecord(ctx, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	href := "/scholarships/" + pgtypeUUIDString(sch.GuideID)

	publishedAt := sch.CreatedAt
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = rec.ExtractedAt
	}

	return sourceItem{
		ID:          pgtypeUUIDString(sch.ID),
		ContentID:   pgtypeUUIDString(sch.GuideID),
		EntityType:  "scholarship",
		Title:       sch.Title,
		Excerpt:     sch.Description,
		OriginalURL: sch.SourceUrl,
		Href:        href,
		PublishedAt: publishedAt,
	}, true, nil
}

func (h *Handlers) jobRecordToSourceItem(ctx context.Context, rec gen.IntelRecord) (sourceItem, bool, error) {
	job, err := h.intelStores.GetIntelJobByIntelRecord(ctx, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	href := "/jobs/" + pgtypeUUIDString(job.GuideID)
	if content, err := h.intelStores.GetContentByID(ctx, job.GuideID); err == nil && content.Slug != "" {
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
		ContentID:   pgtypeUUIDString(job.GuideID),
		EntityType:  "job",
		Title:       job.Title,
		Excerpt:     excerptPtr,
		OriginalURL: job.SourceUrl,
		Href:        href,
		PublishedAt: publishedAt,
	}, true, nil
}

func (h *Handlers) remittanceRecordToSourceItem(ctx context.Context, rec gen.IntelRecord) (sourceItem, bool, error) {
	rate, err := h.intelStores.GetRemittanceRateByIntelRecord(ctx, rec.ID)
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

func (h *Handlers) contentRecordToSourceItem(ctx context.Context, rec gen.IntelRecord) (sourceItem, bool, error) {
	content, err := h.intelStores.GetContentByIntelRecord(ctx, rec.ID)
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
		ContentID:    pgtypeUUIDString(content.ID),
		EntityType:   content.ContentType,
		Title:        content.Title,
		Excerpt:      excerptPtr,
		OriginalURL:  originalURL,
		Href:         "/content/" + content.Slug,
		PublishedAt:  publishedAt,
		ViewCount:    content.ViewCount,
		CommentCount: content.CommentCount,
		ShareCount:   content.ShareCount,
		VoteScore:    content.VoteScore,
	}, true, nil
}

func (h *Handlers) wikiRecordToSourceItem(ctx context.Context, rec gen.IntelRecord) (sourceItem, bool, error) {
	page, err := h.intelStores.GetWikiPageByIntelRecordWithSpace(ctx, rec.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sourceItem{}, false, nil
		}
		return sourceItem{}, false, err
	}

	spaceSlug := page.SpaceSlug
	if spaceSlug == "" {
		spaceSlug = page.Slug
	}
	href := "/wiki/" + spaceSlug + "/" + page.Slug

	return sourceItem{
		ID:          pgtypeUUIDString(page.ID),
		EntityType:  "wiki_page",
		Title:       page.Title,
		Href:        href,
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

func (h *Handlers) canUseMixedFetch(ctx context.Context, needed int) bool {
	maxSize := 100
	if h.settings != nil {
		maxSize = h.settings.GetIntOr(ctx, "intel.radar.max_page_size", 100)
	}
	multiplier := 10
	if h.settings != nil {
		multiplier = h.settings.GetIntOr(ctx, "intel.radar.mixed_fetch_page_multiplier", 10)
	}
	return needed <= maxSize*multiplier
}

func (h *Handlers) listMixedSourceItems(ctx context.Context, w http.ResponseWriter, r *http.Request, sourceID pgtype.UUID, page, limit, pageOffset int, sortMode string) ([]sourceItem, int64, error) {
	slug := chi.URLParam(r, "slug")

	innerLimit := clampInt32(pageOffset + limit)
	records, err := h.intelStores.ListIntelRecordsBySource(ctx, sourceID, "", sortMode, innerLimit, 0)
	if err != nil {
		slog.Error("intel: source records list failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
		return nil, 0, err
	}

	recordTotal, err := h.intelStores.CountIntelRecordsBySource(ctx, sourceID, "")
	if err != nil {
		slog.Error("intel: source records count failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
		return nil, 0, err
	}

	items := make([]sourceItem, 0, len(records))
	for _, rec := range records {
		item, ok, convErr := h.recordToSourceItem(ctx, rec)
		if convErr != nil {
			slog.Warn("intel: source item conversion failed", "record_id", rec.ID, "entity_type", rec.EntityType, "error", convErr)
			continue
		}
		if ok {
			items = append(items, item)
		}
	}

	var videoTotal int64
	if h.videos != nil {
		videos, vTotal, err := h.videos.ListItemsBySourceID(ctx, sourceID, sortMode, innerLimit, 0)
		if err != nil {
			slog.Error("intel: source video items list failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
			return nil, 0, err
		}
		videoTotal = vTotal
		for _, v := range videos {
			items = append(items, videoSourceToItem(v))
		}
	}

	var comparisonTotal int64
	if h.contents != nil {
		comparisons, cTotal, err := h.contents.ListComparisonContentsBySourceID(ctx, sourceID, sortMode, innerLimit, 0)
		if err != nil {
			slog.Error("intel: source comparison items list failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_items_failed", "Failed to load source items")
			return nil, 0, err
		}
		comparisonTotal = cTotal
		for _, c := range comparisons {
			items = append(items, comparisonContentToItem(c))
		}
	}

	sortSourceItems(items, sortMode)

	start := pageOffset
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}

	return items[start:end], int64(recordTotal) + videoTotal + comparisonTotal, nil
}

func videoSourceToItem(v gen.ListVideoSourceItemsBySourceIDRow) sourceItem {
	excerpt := ""
	if v.Description != "" {
		excerpt = v.Description
	}

	// §106: cards open in-app on the video single page — the published post's
	// slug when the video was imported as content, else the video source's
	// own slug. The external URL stays on original_url for the "View" chip.
	href := "/video/" + v.VideoSlug
	if v.ContentSlug != "" {
		href = "/video/" + v.ContentSlug
	}

	return sourceItem{
		ID:           pgtypeUUIDString(v.ID),
		EntityType:   "video",
		Title:        v.Title,
		Excerpt:      &excerpt,
		OriginalURL:  v.SourceUrl,
		Href:         href,
		ThumbnailURL: v.ThumbnailUrl,
		PublishedAt:  v.CreatedAt,
		ViewCount:    v.ViewCount,
		CommentCount: v.CommentCount,
		ShareCount:   v.ShareCount,
		VoteScore:    v.VoteScore,
	}
}

func comparisonContentToItem(c gen.ListComparisonContentsBySourceIDRow) sourceItem {
	excerpt := excerptFromBody(c.Body, 280)

	publishedAt := c.PublishedAt
	if !publishedAt.Valid || publishedAt.Time.IsZero() {
		publishedAt = c.CreatedAt
	}

	return sourceItem{
		ID:           pgtypeUUIDString(c.ID),
		ContentID:    pgtypeUUIDString(c.ID),
		EntityType:   "comparison",
		Title:        c.Title,
		Excerpt:      &excerpt,
		OriginalURL:  "",
		Href:         "/comparisons/" + c.Slug,
		PublishedAt:  publishedAt,
		ViewCount:    c.ViewCount,
		CommentCount: c.CommentCount,
		ShareCount:   c.ShareCount,
		VoteScore:    c.VoteScore,
	}
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
