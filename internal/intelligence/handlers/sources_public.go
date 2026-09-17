package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
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
var sourceSortOptions = []sourceOption{
	{Value: "popular", Label: "Popular"},
	{Value: "recent", Label: "Recently added"},
	{Value: "trending", Label: "Trending"},
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
// GET /api/v1/sources/collections
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

		public := make([]publicSource, 0, len(sources))
		for _, s := range sources {
			if _, isBlocked := blockedSet[s.ID.String()]; isBlocked {
				continue
			}
			isFollowing := false
			if authed && h.followsSvc != nil {
				isFollowing, _ = h.followsSvc.IsFollowing(r.Context(), userID, follows.EntityTypeSource, s.ID)
			}
			public = append(public, intelSourceToPublic(s, isFollowing, false, nil))
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": collections})
}

// HandlePublicSourcesDirectory returns the public source directory.
//
// GET /api/v1/sources
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

	entries := make([]publicSource, 0, len(rows))
	for _, s := range rows {
		if _, isBlocked := blockedSet[s.ID.String()]; isBlocked {
			continue
		}
		isFollowing := false
		if authed && h.followsSvc != nil {
			isFollowing, _ = h.followsSvc.IsFollowing(r.Context(), userID, follows.EntityTypeSource, s.ID)
		}
		entries = append(entries, intelSourceToPublic(s, isFollowing, false, nil))
	}

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
// GET /api/v1/sources/{slug}
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
	}

	// §106: video channel sources expose the channel account username so the
	// detail page can link the claim/report form — the channel profile itself
	// is not public (it redirects to this source page).
	if src.SourceType == "video_channel" && h.videos != nil {
		if uname, uErr := h.videos.GetChannelUsernameBySourceID(r.Context(), row.ID); uErr == nil {
			src.ChannelUsername = uname
		}
	}

	h.ensureSourceTags(r.Context(), &src)

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": src,
	})
}

// HandleReportSource accepts a user report against a publisher source.
//
// POST /api/v1/sources/{slug}/report
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
		errpkg.WriteError(w, http.StatusInternalServerError, "source_report_failed", "Failed to submit report")
		return
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

	if len(slugList) == 0 {
		src.TagSlugs = tagSlugs
		return
	}

	existing, err := h.tagSvc.ResolveTagSlugs(ctx, slugList)
	if err != nil {
		slog.Debug("intel: source tag resolution failed; leaving keywords unlinked", "source", src.Slug, "error", err)
		src.TagSlugs = tagSlugs
		return
	}

	found := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		found[row.Slug] = struct{}{}
	}

	for i, kw := range keywordList {
		slug := slugList[i]
		if _, ok := found[slug]; ok {
			continue
		}

		_, cerr := h.tagSvc.CreateTag(ctx, tags.CreateTagInput{
			Name:     kw,
			Slug:     slug,
			IsSystem: false,
		})
		if cerr != nil {
			// The tag may have been created by a concurrent request. Re-resolve
			// this single slug; if it now exists, keep the mapping. Otherwise
			// remove the keyword from the linkable set so the chip won't 404.
			retry, rerr := h.tagSvc.ResolveTagSlugs(ctx, []string{slug})
			if rerr == nil && len(retry) > 0 {
				found[slug] = struct{}{}
				continue
			}
			delete(tagSlugs, kw)
			slog.Debug("intel: source tag creation failed; removing keyword link", "source", src.Slug, "slug", slug, "error", cerr)
		}
	}

	src.TagSlugs = tagSlugs
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
			Href: "/users/" + *row.UserSlug,
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
// HandleToggleSourceFollow toggles a follow on a source.
//
// POST /api/v1/sources/{slug}/follow
// DELETE /api/v1/sources/{slug}/follow
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

	isFollowing, err := h.followsSvc.ToggleFollow(r.Context(), userID, follows.EntityTypeSource, row.ID)
	if err != nil {
		slog.Error("intel: source follow toggle failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_failed", "Failed to follow source")
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

type proposeSourceRequest struct {
	Name        string  `json:"name"`
	Domain      string  `json:"domain"`
	SourceURL   string  `json:"source_url"`
	Category    string  `json:"category"`
	SourceType  string  `json:"source_type"`
	Description *string `json:"description"`
}

// HandleGetSourcesMeta returns the public source form metadata: allowed
// categories and source types. The values are the v2 directory catalog and are
// intentionally exposed so the frontend proposal form can render selects without
// hardcoding options (Rule 16).
//
// GET /api/v1/sources/meta
func (h *Handlers) HandleGetSourcesMeta(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"categories": []string{
				"scholarship", "job", "visa", "remittance", "news",
				"cost_of_living", "university", "video", "organization", "people",
			},
			"source_types": []string{"rss", "youtube", "api", "static"},
			"entity_types": sourceItemEntityTypes,
			"sorts":        sourceSortOptions,
			"item_sorts":   sourceItemSortOptions,
		},
	})
}

// HandleProposeSource accepts a public source proposal from an authenticated user.
//
// POST /api/v1/sources/propose
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

	src, err := h.intelSources.Propose(r.Context(), userID, sources.ProposeInput{
		Name:        req.Name,
		Domain:      req.Domain,
		SourceURL:   req.SourceURL,
		Category:    req.Category,
		SourceType:  req.SourceType,
		Description: req.Description,
	})
	if err != nil {
		slog.Error("intel: propose source failed", "error", err)

		switch {
		case errors.Is(err, errpkg.ErrRequired):
			errpkg.WriteError(w, http.StatusBadRequest, "missing_required", "Name and source_url are required")
		case errors.Is(err, errpkg.ErrInvalid):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_source_url", "source_url must be a valid http/https URL")
		case errors.Is(err, errpkg.ErrAlreadyExists), errors.Is(err, errpkg.ErrDuplicate):
			errpkg.WriteError(w, http.StatusConflict, "duplicate_source", "A source with this URL already exists")
		default:
			errpkg.WriteError(w, http.StatusInternalServerError, "propose_source_failed", "Failed to propose source")
		}
		return
	}

	slug := derefString(src.Slug)

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"id":               pgtypeUUIDString(src.ID),
			"slug":             slug,
			"name":             src.Name,
			"status":           src.Status,
			"admin_review_url": "/admin/sources/proposed", // surfaced for UI navigation
		},
	})
}

type blockedSource struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// HandleListBlockedSources returns the sources the current user has blocked.
//
// GET /api/v1/me/blocked-sources
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
			ID:   pgtypeUUIDString(src.ID),
			Slug: derefString(src.Slug),
			Name: src.Name,
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": sources})
}

// HandleToggleSourceBlock toggles a user's block on a source.
// POST blocks, DELETE unblocks. Returns the new block state.
//
// POST /api/v1/sources/{slug}/block
// DELETE /api/v1/sources/{slug}/block
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
