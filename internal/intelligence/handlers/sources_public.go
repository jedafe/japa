package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/intelligence/extractor/schemas"
	"github.com/jedafe/japa/internal/intelligence/sources"
	"github.com/jedafe/japa/internal/reports"
	"github.com/jedafe/japa/internal/tags"
)

// sourceOption is a single selectable option with a value and a display label.
// Keeping the label on the backend lets the frontend render tabs and selects
// without hardcoding a display map (Rule 16).
type sourceOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// sourceItemEntityTypes is the canonical set of entity types the source feed
// can return. It must stay in sync with recordToSourceItem in sources_items.go.
var sourceItemEntityTypes = []sourceOption{
	{Value: "scholarship", Label: "Scholarships"},
	{Value: "job", Label: "Jobs"},
	{Value: "remittance", Label: "Remittance"},
	{Value: "news", Label: "News"},
	{Value: "culture_lifestyle", Label: "Culture & Lifestyle"},
	{Value: "guide", Label: "Guide"},
	{Value: "blog", Label: "Blog"},
	{Value: "article", Label: "Article"},
	{Value: "wiki_page", Label: "Wiki"},
	{Value: "video", Label: "Video"},
	{Value: "comparison", Label: "Comparison"},
}

// sourceSortOptions are the sorts exposed by the public source directory.
// "rising" is trending restricted to sources created in the last 7 days.
var sourceSortOptions = []sourceOption{
	{Value: "popular", Label: "Popular"},
	{Value: "recent", Label: "Recently added"},
	{Value: "trending", Label: "Trending"},
	{Value: "rising", Label: "Rising"},
}

// sourceKindOptions are the publisher kinds a directory visitor can filter
// by (?source_kind=). They describe WHO publishes the source — distinct from
// source_types (rss/video/api/static), which describe the crawl mechanism
// and are only meaningful on the proposal form. Values must stay in sync with
// the intel_sources.source_kind CHECK constraint — the 10-kind set from the
// intel-channels expansion (migration 000681; sources.SourceKind.IsValid).
// Ref: sources directory v2 design (2026-09-11-sources-directory-design.md),
// intel-channels curation design §1 (2026-09-20-intel-channels-curation-design.md).
var sourceKindOptions = []sourceOption{
	{Value: string(sources.SourceKindPublisher), Label: "Publication"},
	{Value: string(sources.SourceKindInstitution), Label: "Institution"},
	{Value: string(sources.SourceKindIndividual), Label: "Individual"},
	{Value: string(sources.SourceKindAggregator), Label: "Aggregator"},
	{Value: string(sources.SourceKindChannelProfile), Label: "Channel or profile"},
	{Value: string(sources.SourceKindInstitutionSite), Label: "Institution site"},
	{Value: string(sources.SourceKindPersonalBlog), Label: "Personal blog"},
	{Value: string(sources.SourceKindJobBoard), Label: "Job board"},
	{Value: string(sources.SourceKindGovPortal), Label: "Government portal"},
	{Value: string(sources.SourceKindCommunity), Label: "Community"},
}

// submittableSourceKindOptions is the propose-form subset of
// sourceKindOptions: entity-stub kinds (institution, individual) are
// excluded — they are assigned only by CreateSourceFromEntity and
// buildProposeParams rejects them. Mirrors sources.SourceKind.IsSubmittable.
var submittableSourceKindOptions = []sourceOption{
	{Value: string(sources.SourceKindPublisher), Label: "Publication"},
	{Value: string(sources.SourceKindAggregator), Label: "Aggregator"},
	{Value: string(sources.SourceKindChannelProfile), Label: "Channel or profile"},
	{Value: string(sources.SourceKindInstitutionSite), Label: "Institution site"},
	{Value: string(sources.SourceKindPersonalBlog), Label: "Personal blog"},
	{Value: string(sources.SourceKindJobBoard), Label: "Job board"},
	{Value: string(sources.SourceKindGovPortal), Label: "Government portal"},
	{Value: string(sources.SourceKindCommunity), Label: "Community"},
}

// sourceItemSortOptions are the sorts exposed by a source's item feed.
var sourceItemSortOptions = []sourceOption{
	{Value: "latest", Label: "Latest"},
	{Value: "popular", Label: "Popular"},
	{Value: "discussed", Label: "Discussed"},
	{Value: "upvoted", Label: "Upvoted"},
}

// publicSource is the public source profile shape returned by the /sources
// directory and /sources/{slug} detail endpoints. It omits internal crawl
// configuration (provider_config, robots_policy, error state).
type publicSource struct {
	ID            string            `json:"id"`
	Slug          string            `json:"slug"`
	Name          string            `json:"name"`
	Description   *string           `json:"description,omitempty"`
	Domain        string            `json:"domain"`
	SourceURL     string            `json:"source_url,omitempty"`
	Category      string            `json:"category"`
	SourceType    string            `json:"source_type"`
	SourceKind    string            `json:"source_kind"`
	TrustLevel    int16             `json:"trust_level"`
	FollowerCount int32             `json:"follower_count"`
	PostCount     int32             `json:"post_count"`
	LogoURL       *string           `json:"logo_url,omitempty"`
	Keywords      []string          `json:"keywords,omitempty"`
	TagSlugs      map[string]string `json:"tag_slugs,omitempty"`
	IsFollowing   bool              `json:"is_following"`
	IsBlocked     bool              `json:"is_blocked"`
	LinkedEntity  *linkedEntity     `json:"linked_entity,omitempty"`
	// ChannelUsername is the username of the channel account that owns a
	// video_channel source's videos (§106) — lets the detail page link to
	// the claim/report form without exposing the channel profile.
	ChannelUsername string `json:"channel_username,omitempty"`
	// EntityTypes is the set of feed entity types the source currently
	// publishes (detail endpoint only) — the detail page scopes its filter
	// pills to it instead of offering every global type.
	EntityTypes []string `json:"entity_types,omitempty"`
	// Detail-endpoint provenance fields for the About section (v2 spec):
	// license, attribution requirement, crawl cadence, and last crawl time.
	SourceLicense       *string            `json:"source_license,omitempty"`
	AttributionRequired bool               `json:"attribution_required"`
	CrawlFrequency      string             `json:"crawl_frequency,omitempty"`
	LastCrawledAt       pgtype.Timestamptz `json:"last_crawled_at,omitempty"`
}

type linkedEntity struct {
	Type string `json:"type"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Href string `json:"href"`
}

// sourceListMeta is the pagination envelope for source directory responses.
type sourceListMeta struct {
	PageSize   int   `json:"page_size"`
	PageOffset int   `json:"page_offset"`
	Total      int64 `json:"total"`
}

type sourceCollection struct {
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	Sources     []publicSource `json:"sources"`
}

type collectionQuery struct {
	Category     string `json:"category"`
	SourceKind   string `json:"source_kind"`
	Sort         string `json:"sort"`
	Search       string `json:"q"`
	CreatedAfter string `json:"created_after"`
}

// HandleListSourceCollections returns the curated directory sections for the
// current platform. Manual collections read source_collection_items; query-based
// collections execute stored filters against ListPublicSources.
//
// GET /api/v1/sources/collections.
func (h *Handlers) HandleListSourceCollections(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, authed := optionalUser(w, r)

	rows, err := h.intelStores.ListSourceCollections(r.Context())
	if err != nil {
		slog.Error("intel: list source collections failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_collections_failed", "Failed to load source collections")

		return
	}

	// Build blocked set for authenticated users (v2 Task 2).
	blockedSet := make(map[string]struct{})

	if authed && h.intelSources != nil {
		blockedIDs, err := h.intelSources.ListBlockedSourceIDs(r.Context(), userID)
		if err != nil {
			slog.Error("intel: collections blocked ids failed", "error", err)
		} else {
			for _, id := range blockedIDs {
				if id.Valid {
					blockedSet[id.String()] = struct{}{}
				}
			}
		}
	}

	collections := make([]sourceCollection, 0, len(rows))

	for _, c := range rows {
		var sources []gen.IntelSource

		switch c.CollectionType {
		case "manual":
			items, err := h.intelStores.ListSourceCollectionItems(r.Context(), c.ID)
			if err != nil {
				slog.Error("intel: list collection items failed", "collection_id", c.ID, "error", err)
				continue
			}

			sources = items
		case "query":
			var q collectionQuery
			if len(c.QueryJson) > 0 {
				_ = json.Unmarshal(c.QueryJson, &q)
			}

			sort := q.Sort
			if sort == "" {
				sort = derefString(c.Sort)
			}

			if sort == "" {
				sort = "popular"
			}

			queryItems, _, err := h.intelStores.ListPublicSources(r.Context(), q.Category, q.SourceKind, q.Search, sort, int32(c.Limit), 0)
			if err != nil {
				slog.Error("intel: query collection sources failed", "collection_id", c.ID, "error", err)
				continue
			}

			sources = queryItems
		default:
			continue
		}

		// Batch-resolve follow state per collection (§4.1 — no per-row N+1).
		followingSet := make(map[pgtype.UUID]bool)
		if authed && h.followsSvc != nil && len(sources) > 0 {
			ids := make([]pgtype.UUID, 0, len(sources))
			for _, s := range sources {
				ids = append(ids, s.ID)
			}
			if m, err := h.followsSvc.GetBulkFollowingStatus(r.Context(), userID, follows.EntityTypeSource, ids); err == nil {
				followingSet = m
			} else {
				slog.Error("intel: collection following status failed", "collection_id", c.ID, "error", err)
			}
		}

		public := make([]publicSource, 0, len(sources))

		for _, s := range sources {
			if _, isBlocked := blockedSet[s.ID.String()]; isBlocked {
				continue
			}

			public = append(public, intelSourceToPublic(s, followingSet[s.ID], false, nil))
		}

		if len(public) == 0 && c.CollectionType == "manual" {
			// Keep manual collections visible even when empty.
			public = []publicSource{}
		}

		coll := sourceCollection{
			Slug:        c.Slug,
			Name:        c.Name,
			Description: c.Description,
			Sources:     public,
		}
		collections = append(collections, coll)
	}

	h.resolveLiterals(r, &collections)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": collections})
}

// HandlePublicSourcesDirectory returns the public source directory.
//
// GET /api/v1/sources.
func (h *Handlers) HandlePublicSourcesDirectory(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	q := r.URL.Query()
	category := q.Get("category")
	sourceKind := q.Get("source_kind")
	search := q.Get("q")

	sort := q.Get("sort")
	if sort == "" {
		sort = "popular"
	}

	page, limit := h.parsePagination(r)
	offset := clampInt32((page - 1) * limit)

	rows, total, err := h.intelStores.ListPublicSources(r.Context(), category, sourceKind, search, sort, clampInt32(limit), offset)
	if err != nil {
		slog.Error("intel: public sources directory list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "sources_list_failed", "Failed to list sources")

		return
	}

	userID, authed := optionalUser(w, r)

	// Build a set of blocked sources for the authenticated caller (v2 Task 2).
	blockedSet := make(map[string]struct{})

	if authed && h.intelSources != nil {
		blockedIDs, err := h.intelSources.ListBlockedSourceIDs(r.Context(), userID)
		if err != nil {
			slog.Error("intel: public sources blocked ids failed", "error", err)
		} else {
			for _, id := range blockedIDs {
				if id.Valid {
					blockedSet[id.String()] = struct{}{}
				}
			}
		}
	}

	// Batch-resolve follow state — the previous per-row IsFollowing call was
	// an N+1 (one DB round-trip per source card) (§4.1).
	followingSet := make(map[pgtype.UUID]bool)
	if authed && h.followsSvc != nil {
		ids := make([]pgtype.UUID, 0, len(rows))
		for _, s := range rows {
			ids = append(ids, s.ID)
		}
		if m, err := h.followsSvc.GetBulkFollowingStatus(r.Context(), userID, follows.EntityTypeSource, ids); err == nil {
			followingSet = m
		} else {
			slog.Error("intel: public sources following status failed", "error", err)
		}
	}

	entries := make([]publicSource, 0, len(rows))

	for _, s := range rows {
		if _, isBlocked := blockedSet[s.ID.String()]; isBlocked {
			continue
		}

		entries = append(entries, intelSourceToPublic(s, followingSet[s.ID], false, nil))
	}

	h.resolveLiterals(r, &entries)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": entries,
		"meta": sourceListMeta{
			PageSize:   limit,
			PageOffset: (page - 1) * limit,
			Total:      total,
		},
	})
}

// HandleGetSource returns the public source profile, including linked entity
// and (for authenticated users) is_following state.
//
// GET /api/v1/sources/{slug}.
func (h *Handlers) HandleGetSource(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Source slug is required")
		return
	}

	row, err := h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), slug)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		// §106: also accept the source's UUID so callers holding only the ID
		// (e.g. a video channel's synthetic author → source redirect) resolve.
		var id pgtype.UUID
		if idErr := id.Scan(slug); idErr == nil && id.Valid {
			if raw, gErr := h.intelStores.GetIntelSource(r.Context(), id); gErr == nil && raw.Slug != nil && *raw.Slug != "" {
				row, err = h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), *raw.Slug)
			}
		}
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "source_not_found", "Source not found")
			return
		}

		slog.Error("intel: public source detail failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_detail_failed", "Failed to load source")

		return
	}

	userID, authed := optionalUser(w, r)

	var isFollowing, isBlocked bool
	if authed {
		isFollowing, _ = h.followsSvc.IsFollowing(r.Context(), userID, follows.EntityTypeSource, row.ID)
		if h.intelSources != nil {
			isBlocked, _ = h.intelSources.IsSourceBlocked(r.Context(), userID, row.ID)
		}
	}

	linked := buildLinkedEntity(row)

	src := publicSource{
		ID:            pgtypeUUIDString(row.ID),
		Slug:          derefString(row.Slug),
		Name:          row.Name,
		Description:   row.Description,
		Domain:        row.Domain,
		SourceURL:     row.SourceUrl,
		Category:      row.Category,
		SourceType:    row.SourceType,
		SourceKind:    row.SourceKind,
		TrustLevel:    row.TrustLevel,
		FollowerCount: row.FollowerCount,
		PostCount:     row.PostCount,
		LogoURL:       row.LogoUrl,
		Keywords:      row.Keywords,
		IsFollowing:   isFollowing,
		IsBlocked:     isBlocked,
		LinkedEntity:  linked,
		// About-section provenance (detail endpoint only).
		SourceLicense:       row.SourceLicense,
		AttributionRequired: row.AttributionRequired,
		CrawlFrequency:      row.CrawlFrequency,
		LastCrawledAt:       row.LastCrawledAt,
	}

	// §106: video channel sources expose the channel account username so the
	// detail page can link the claim/report form — the channel profile itself
	// is not public (it redirects to this source page).
	if src.SourceType == "video_channel" && h.videos != nil {
		if uname, uErr := h.videos.GetChannelUsernameBySourceID(r.Context(), row.ID); uErr == nil {
			src.ChannelUsername = uname
		}
	}

	src.EntityTypes = h.resolveSourceFeedEntityTypes(r.Context(), row.ID)

	h.ensureSourceTags(r.Context(), &src)

	h.resolveLiterals(r, &src)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": src,
	})
}

// HandleReportSource accepts a user report against a publisher source.
//
// POST /api/v1/sources/{slug}/report.
type sourceReportBody struct {
	Reason       string   `json:"reason"`
	Notes        *string  `json:"notes"`
	EvidenceURLs []string `json:"evidence_urls"`
}

func (h *Handlers) HandleReportSource(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil || h.reportSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "unavailable", "Source reporting is not configured")
		return
	}

	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

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

		slog.Error("intel: source report lookup failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_report_lookup_failed", "Failed to load source")

		return
	}

	var body sourceReportBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	report, err := h.reportSvc.CreateReport(r.Context(), user.UserID, reports.CreateReportInput{
		EntityType:   reports.EntityTypeSource,
		EntityID:     src.ID,
		Reason:       body.Reason,
		Notes:        body.Notes,
		EvidenceURLs: body.EvidenceURLs,
	})
	if err != nil {
		// Mirror reports.submitReport's writeServiceError mapping so client
		// errors don't surface as 500s (a repeat report hit the partial
		// unique index and previously returned 500 source_report_failed).
		switch {
		case errors.Is(err, reports.ErrAlreadyReported):
			errpkg.WriteError(w, http.StatusConflict, "already_reported", err.Error())
		case errors.Is(err, reports.ErrInvalidReason), errors.Is(err, reports.ErrInvalidEntityType):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
		default:
			slog.Error("intel: source report failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_report_failed", "Failed to submit report")
		}

		return
	}

	// Section 51 report:notify_admin — notify every user with reports.review
	// that a new source report was filed, matching the fan-out the generic
	// content/comment/user report handlers run. Fire-and-forget per Rule 8.
	if h.asynqClient != nil {
		payload, _ := json.Marshal(map[string]string{
			"report_id":   report.ID.String(),
			"entity_type": reports.EntityTypeSource,
			"entity_id":   src.ID.String(),
			"reason":      body.Reason,
		})
		if _, qerr := h.asynqClient.EnqueueContext(r.Context(), asynq.NewTask(TaskReportNotifyAdmin, payload)); qerr != nil {
			slog.Warn("intel: failed to enqueue report:notify_admin", "report_id", report.ID.String(), "error", qerr)
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": report,
	})
}

// ensureSourceTags converts one-word source keywords into directory tags,
// creating any that do not already exist. It populates src.TagSlugs with
// keyword→slug mappings so the frontend can link chips to /tags/{slug}
// without 404ing. Multi-word keywords are left out of TagSlugs because the
// directory tag surface is designed for single-token tags.
func (h *Handlers) ensureSourceTags(ctx context.Context, src *publicSource) {
	if h.tagSvc == nil || len(src.Keywords) == 0 {
		return
	}

	tagSlugs := make(map[string]string, 0)

	var slugList []string

	var keywordList []string

	for _, kw := range src.Keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" || strings.ContainsAny(kw, " \t\n\r") {
			continue
		}

		slug := content.Slugify(kw)
		if slug == "" || slug == "untitled" {
			continue
		}

		tagSlugs[kw] = slug
		slugList = append(slugList, slug)
		keywordList = append(keywordList, kw)
	}

	existing, err := h.tagSvc.ResolveTagSlugs(ctx, slugList)
	if err != nil {
		slog.Debug("intel: source tag resolution failed; leaving keywords unlinked", "source", src.Slug, "error", err)
		src.TagSlugs = tagSlugs

		return
	}

	// Read path stays pure — a GET must never create rows. Only keywords whose
	// tag already exists are linked; creation happens on the write path in
	// createSourceTags (called by the propose/approve/update handlers).
	found := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		found[row.Slug] = struct{}{}
	}

	for i, kw := range keywordList {
		if _, ok := found[slugList[i]]; !ok {
			delete(tagSlugs, kw)
		}
	}

	src.TagSlugs = tagSlugs
}

// createSourceTags materializes directory tags for a source's keywords on the
// write path (propose/approve/update), so the public GET only has to link
// existing tags. Concurrent creation is fine — ResolveTagSlugs is re-run for
// any slug the insert raced.
func (h *Handlers) createSourceTags(ctx context.Context, keywords []string) {
	if h.tagSvc == nil {
		return
	}

	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" || strings.ContainsAny(kw, " \t\n\r") {
			continue
		}

		slug := content.Slugify(kw)
		if slug == "" || slug == "untitled" {
			continue
		}

		if existing, err := h.tagSvc.ResolveTagSlugs(ctx, []string{slug}); err == nil && len(existing) > 0 {
			continue
		}

		if _, cerr := h.tagSvc.CreateTag(ctx, tags.CreateTagInput{
			Name:     kw,
			Slug:     slug,
			IsSystem: false,
		}); cerr != nil {
			slog.Debug("intel: source tag creation failed", "slug", slug, "error", cerr)
		}
	}
}

// resolveSourceFeedEntityTypes returns the entity types a source's item feed
// can actually return right now: the distinct published intel_record types,
// plus "video"/"comparison" when those linked feeds have items. Values are
// emitted in the canonical sourceItemEntityTypes order so the detail page's
// filter pills render deterministically and never offer a guaranteed-empty
// type.
func (h *Handlers) resolveSourceFeedEntityTypes(ctx context.Context, sourceID pgtype.UUID) []string {
	have := map[string]bool{}

	recordTypes, err := h.intelStores.ListDistinctIntelRecordEntityTypesBySource(ctx, sourceID)
	if err != nil {
		slog.Error("intel: source entity types lookup failed", "error", err)
	} else {
		for _, et := range recordTypes {
			have[et] = true
		}
	}

	if h.videos != nil {
		if _, total, vErr := h.videos.ListItemsBySourceID(ctx, sourceID, "latest", 1, 0); vErr == nil && total > 0 {
			have["video"] = true
		}
	}

	if h.contents != nil {
		if _, total, cErr := h.contents.ListComparisonContentsBySourceID(ctx, sourceID, "latest", 1, 0); cErr == nil && total > 0 {
			have["comparison"] = true
		}
	}

	out := make([]string, 0, len(have))

	for _, opt := range sourceItemEntityTypes {
		if have[opt.Value] {
			out = append(out, opt.Value)
		}
	}

	return out
}

func buildLinkedEntity(row gen.GetIntelSourceBySlugWithLinkedEntityRow) *linkedEntity {
	switch {
	case row.SpaceSlug != nil && *row.SpaceSlug != "":
		return &linkedEntity{
			Type: "space",
			Slug: *row.SpaceSlug,
			Name: *row.SpaceName,
			Href: "/spaces/" + *row.SpaceSlug,
		}
	case row.OrgSlug != nil && *row.OrgSlug != "":
		return &linkedEntity{
			Type: "organization",
			Slug: *row.OrgSlug,
			Name: *row.OrgName,
			Href: "/organizations/" + *row.OrgSlug,
		}
	case row.UserSlug != nil && *row.UserSlug != "":
		return &linkedEntity{
			Type: "user",
			Slug: *row.UserSlug,
			Name: row.UserName,
			Href: "/u/" + *row.UserSlug,
		}
	}

	return nil
}

func intelSourceToPublic(s gen.IntelSource, isFollowing, isBlocked bool, linked *linkedEntity) publicSource {
	return publicSource{
		ID:            pgtypeUUIDString(s.ID),
		Slug:          derefString(s.Slug),
		Name:          s.Name,
		Description:   s.Description,
		Domain:        s.Domain,
		SourceURL:     s.SourceUrl,
		Category:      s.Category,
		SourceType:    s.SourceType,
		SourceKind:    s.SourceKind,
		TrustLevel:    s.TrustLevel,
		FollowerCount: s.FollowerCount,
		PostCount:     s.PostCount,
		LogoURL:       s.LogoUrl,
		Keywords:      s.Keywords,
		IsFollowing:   isFollowing,
		IsBlocked:     isBlocked,
		LinkedEntity:  linked,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// pgtypeUUIDString returns a string for a pgtype.UUID when valid, otherwise "".
// HandleToggleSourceFollow follows or unfollows a source.
// POST follows (idempotent — no-op when already following); DELETE unfollows
// (idempotent — no-op when not following). Mirrors HandleToggleSourceBlock.
//
// POST /api/v1/sources/{slug}/follow
// DELETE /api/v1/sources/{slug}/follow.
func (h *Handlers) HandleToggleSourceFollow(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil || h.followsSvc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "unavailable", "Source follow is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Source slug is required")
		return
	}

	row, err := h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "source_not_found", "Source not found")
			return
		}

		slog.Error("intel: source follow lookup failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_lookup_failed", "Failed to load source")

		return
	}

	// POST follows; DELETE unfollows. chi only routes those two methods here,
	// so anything else falls back to toggling (defensive parity with
	// HandleToggleSourceBlock).
	var wantFollow bool

	switch r.Method {
	case http.MethodPost:
		wantFollow = true
	case http.MethodDelete:
		wantFollow = false
	default:
		current, cerr := h.followsSvc.IsFollowing(r.Context(), userID, follows.EntityTypeSource, row.ID)
		if cerr != nil {
			slog.Error("intel: source follow state failed", "slug", slug, "error", cerr)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_state_failed", "Failed to check follow state")

			return
		}

		wantFollow = !current
	}

	var isFollowing bool
	if wantFollow {
		isFollowing, err = h.followsSvc.EnsureFollow(r.Context(), userID, follows.EntityTypeSource, row.ID)
	} else {
		isFollowing, err = h.followsSvc.EnsureUnfollow(r.Context(), userID, follows.EntityTypeSource, row.ID)
	}

	if err != nil {
		slog.Error("intel: source follow update failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_failed", "Failed to update follow state")

		return
	}

	// Re-fetch the source to get the updated follower_count.
	src, err := h.intelSources.Get(r.Context(), row.ID)
	if err != nil {
		slog.Error("intel: source follow reload failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_reload_failed", "Failed to load updated source")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"is_following":   isFollowing,
			"follower_count": src.FollowerCount,
		},
	})
}

func pgtypeUUIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}

	return id.String()
}

// proposeSourceRequest is the POST /api/v1/sources/propose body. The
// intel-channels payload adds `categories`, `source_kind` and
// `probe_snapshot`; the legacy `category`/`source_type` fields are still
// accepted for backward compatibility (the service folds category into
// categories and maps 'video'/'youtube' source_type to
// source_kind=channel_profile).
type proposeSourceRequest struct {
	Name          string          `json:"name"`
	Domain        string          `json:"domain"`
	SourceURL     string          `json:"source_url"`
	Category      string          `json:"category"`
	Categories    []string        `json:"categories"`
	SourceType    string          `json:"source_type"`
	SourceKind    string          `json:"source_kind"`
	ProbeSnapshot json.RawMessage `json:"probe_snapshot"`
	Description   *string         `json:"description"`
}

// HandleGetSourcesMeta returns the public source form metadata: allowed
// categories and source types. The values are the v2 directory catalog and are
// intentionally exposed so the frontend proposal form can render selects without
// hardcoding options (Rule 16).
//
// GET /api/v1/sources/meta.
func (h *Handlers) HandleGetSourcesMeta(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			// categories is the curated proposal catalog — extended with the
			// intel-channels categories (blog, event, dataset, announcement,
			// scam_alert) so the propose form can offer multi-category
			// selection (categories[] on the source row; spec §6).
			"categories": []string{
				"scholarship", "job", "visa", "remittance", "news",
				"cost_of_living", "university", "video", "organization", "people",
				"blog", "event", "dataset", "announcement", "scam_alert",
			},
			// submittable_categories is the propose-form subset — domains
			// whose channels reach content without AI extraction
			// (job-family, news/article feeds, video). Mirrors the
			// submittable_source_kinds / source_kinds split;
			// buildProposeParams rejects proposals outside it.
			"submittable_categories": sources.SubmittableCategories(),
			"source_types":           []string{"rss", "video", "api", "static"},
			// source_kinds is the full 10-kind set for the directory filter
			// and admin forms; submittable_source_kinds is the propose-form
			// subset (entity-stub kinds excluded — see IsSubmittable).
			"source_kinds":             sourceKindOptions,
			"submittable_source_kinds": submittableSourceKindOptions,
			// channel_pipelines is the intel_source_channels.pipeline CHECK
			// set (migration 000681) — extract sends pages to the LLM
			// extractor, curate promotes itemized records directly, hybrid
			// runs curate per item + extract per page.
			"channel_pipelines": []string{
				string(sources.PipelineExtract),
				string(sources.PipelineCurate),
				string(sources.PipelineHybrid),
			},
			"entity_types": sourceItemEntityTypes,
			// admin_* fields carry the canonical option sets for the control
			// panel's source form (Rule 16) — the public 'categories' list
			// above stays the curated proposal catalog, while admin_categories
			// is the full extraction-domain taxonomy.
			"admin_categories": schemas.RegisteredCategories(),
			// crawl_methods mirrors the composite fetcher's adapter set
			// (pdf/ocr fetch through the static adapter). 'manual' is a
			// backend sentinel for non-crawlable rows and is not offered.
			"crawl_methods":     []string{"static", "rss", "api", "pdf", "ocr"},
			"crawl_frequencies": []string{"hourly", "daily", "weekly", "monthly"},
			// registered_entity_types is the full extraction-domain registry —
			// a superset of entity_types (which is the curated feed subset).
			// Exposed so the control panel can offer every valid value for a
			// source's provider_config.entity_types override instead of
			// hardcoding the list (Rule 16).
			"registered_entity_types": schemas.RegisteredEntityTypes(),
			"sorts":                   sourceSortOptions,
			"item_sorts":              sourceItemSortOptions,
		},
	})
}

// HandleProposeSource accepts a public source proposal from an authenticated user.
//
// POST /api/v1/sources/propose.
func (h *Handlers) HandleProposeSource(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req proposeSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	// Never trust a client-supplied probe_snapshot — when one was provided
	// the proposer is claiming a probe ran, so re-run the preview probe
	// server-side (same per-user quota class as /sources/probe) and store
	// OUR report. A failed or quota-blocked re-probe keeps the client
	// snapshot — reviewers see it flagged unverified (last_probed_at NULL).
	serverReport, serverMethods := h.reprobeProposal(r.Context(), userID, req)
	snapshot := req.ProbeSnapshot
	if serverReport != nil {
		snapshot = serverReport
	}

	src, err := h.intelSources.Propose(r.Context(), userID, sources.ProposeInput{
		Name:          req.Name,
		Domain:        req.Domain,
		SourceURL:     req.SourceURL,
		Category:      req.Category,
		Categories:    req.Categories,
		SourceType:    req.SourceType,
		SourceKind:    req.SourceKind,
		ProbeSnapshot: snapshot,
		Description:   req.Description,
	})
	if err != nil {
		slog.Error("intel: propose source failed", "error", err)

		switch {
		case errors.Is(err, errpkg.ErrRequired):
			errpkg.WriteError(w, http.StatusBadRequest, "missing_required", "Name, source_url and at least one category are required")
		case errors.Is(err, errpkg.ErrInvalid):
			// ErrInvalid covers bad source_url/domain, an unknown
			// source_kind, a malformed probe_snapshot, and the §106
			// video-platform guard — pick the code/message from the
			// service's field tag so each failure stays identifiable.
			code, msg := "invalid_source_url", "source_url must be a valid http/https URL"

			switch {
			case strings.Contains(err.Error(), "source_kind"):
				code, msg = "invalid_source_kind", "source_kind is not a supported value"
			case strings.Contains(err.Error(), "category"):
				code, msg = "category_not_submittable", "that category is not open for proposals yet"
			case strings.Contains(err.Error(), "probe_snapshot"):
				code, msg = "invalid_probe_snapshot", "probe_snapshot must be a JSON object"
			case strings.Contains(err.Error(), "video"):
				msg = "video sources must be a supported video platform URL"
			}

			errpkg.WriteError(w, http.StatusBadRequest, code, msg)
		case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
			errpkg.WriteError(w, http.StatusConflict, "duplicate_source", "A source with this URL already exists")
		default:
			errpkg.WriteError(w, http.StatusInternalServerError, "propose_source_failed", "Failed to propose source")
		}

		return
	}

	slug := derefString(src.Slug)

	// Stamp the server-verified probe report: last_probed_at marks it as
	// backend-produced so the review UI distinguishes it from a
	// proposer-supplied snapshot.
	if serverReport != nil {
		if serr := h.intelSources.RecordProposeProbe(r.Context(), src.ID, serverReport, serverMethods); serr != nil {
			slog.Warn("intel: propose probe stamp failed", "source_id", src.ID.String(), "error", serr)
		}
	}

	// Materialize directory tags for the proposed keywords here (write path)
	// so GET /sources/{slug} only has to link existing tags.
	h.createSourceTags(r.Context(), src.Keywords)

	h.resolveLiterals(r, &src)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"id":     pgtypeUUIDString(src.ID),
			"slug":   slug,
			"name":   src.Name,
			"status": src.Status,
		},
	})
}

type blockedSource struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

// HandleListBlockedSources returns the sources the current user has blocked.
//
// GET /api/v1/me/blocked-sources.
func (h *Handlers) HandleListBlockedSources(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	ids, err := h.intelSources.ListBlockedSourceIDs(r.Context(), userID)
	if err != nil {
		slog.Error("intel: list blocked sources failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_blocked_sources_failed", "Failed to load blocked sources")

		return
	}

	sources := make([]blockedSource, 0, len(ids))

	for _, id := range ids {
		src, err := h.intelSources.Get(r.Context(), id)
		if err != nil {
			continue
		}

		sources = append(sources, blockedSource{
			ID:     pgtypeUUIDString(src.ID),
			Slug:   derefString(src.Slug),
			Name:   src.Name,
			Domain: src.Domain,
		})
	}

	h.resolveLiterals(r, &sources)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": sources})
}

// HandleToggleSourceBlock toggles a user's block on a source.
// POST blocks, DELETE unblocks. Returns the new block state.
//
// POST /api/v1/sources/{slug}/block
// DELETE /api/v1/sources/{slug}/block.
func (h *Handlers) HandleToggleSourceBlock(w http.ResponseWriter, r *http.Request) {
	if h.intelSources == nil || h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "unavailable", "Source blocking is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

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

		slog.Error("intel: source block lookup failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_block_lookup_failed", "Failed to load source")

		return
	}

	// POST blocks; DELETE unblocks. For other methods, default to the method.
	var wantBlock bool

	switch r.Method {
	case http.MethodPost:
		wantBlock = true
	case http.MethodDelete:
		wantBlock = false
	default:
		// Should not happen with chi routing; treat as toggle.
		blocked, err := h.intelSources.IsSourceBlocked(r.Context(), userID, src.ID)
		if err != nil {
			slog.Error("intel: source block state failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_block_state_failed", "Failed to check block state")

			return
		}

		wantBlock = !blocked
	}

	// Toggle the service to the desired state.
	blocked, err := h.intelSources.IsSourceBlocked(r.Context(), userID, src.ID)
	if err != nil {
		slog.Error("intel: source block state failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_block_state_failed", "Failed to check block state")

		return
	}

	isBlocked := blocked
	if blocked != wantBlock {
		isBlocked, err = h.intelSources.ToggleSourceBlock(r.Context(), userID, src.ID)
		if err != nil {
			slog.Error("intel: source block toggle failed", "slug", slug, "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "source_block_toggle_failed", "Failed to toggle block")

			return
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"is_blocked": isBlocked,
		},
	})
}
