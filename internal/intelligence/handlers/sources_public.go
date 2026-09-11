package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/follows"
	"github.com/jedafe/japa/internal/tenant"
)

// publicSource is the public source profile shape returned by the /sources
// directory and /sources/{slug} detail endpoints. It omits internal crawl
// configuration (provider_config, robots_policy, error state).
type publicSource struct {
	ID            string         `json:"id"`
	Slug          string         `json:"slug"`
	Name          string         `json:"name"`
	Description   *string        `json:"description,omitempty"`
	Domain        string         `json:"domain"`
	Category      string         `json:"category"`
	SourceType    string         `json:"source_type"`
	SourceKind    string         `json:"source_kind"`
	TrustLevel    int16          `json:"trust_level"`
	FollowerCount int32          `json:"follower_count"`
	PostCount     int32          `json:"post_count"`
	LogoURL       *string        `json:"logo_url,omitempty"`
	IsFollowing   bool           `json:"is_following"`
	LinkedEntity  *linkedEntity  `json:"linked_entity,omitempty"`
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

// HandlePublicSourcesDirectory returns the public source directory.
//
// GET /api/v1/sources
func (h *Handlers) HandlePublicSourcesDirectory(w http.ResponseWriter, r *http.Request) {
	if h.intelStores == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Intelligence system is not configured")
		return
	}

	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = h.defaultTenant
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

	rows, total, err := h.intelStores.ListPublicSources(r.Context(), tenantID, category, sourceKind, search, sort, clampInt32(limit), offset)
	if err != nil {
		slog.Error("intel: public sources directory list failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "sources_list_failed", "Failed to list sources")
		return
	}

	entries := make([]publicSource, 0, len(rows))
	for _, s := range rows {
		entries = append(entries, intelSourceToPublic(s, false, nil))
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

	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = h.defaultTenant
	}

	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Source slug is required")
		return
	}

	row, err := h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), tenantID, slug)
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

	var isFollowing bool
	if authed {
		isFollowing, _ = h.followsSvc.IsFollowing(r.Context(), tenantID, userID, follows.EntityTypeSource, row.ID)
	}

	linked := buildLinkedEntity(row)

	src := publicSource{
		ID:            pgtypeUUIDString(row.ID),
		Slug:          derefString(row.Slug),
		Name:          row.Name,
		Description:   row.Description,
		Domain:        row.Domain,
		Category:      row.Category,
		SourceType:    row.SourceType,
		SourceKind:    row.SourceKind,
		TrustLevel:    row.TrustLevel,
		FollowerCount: row.FollowerCount,
		PostCount:     row.PostCount,
		LogoURL:       row.LogoUrl,
		IsFollowing:   isFollowing,
		LinkedEntity:  linked,
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": src,
	})
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

func intelSourceToPublic(s gen.IntelSource, isFollowing bool, linked *linkedEntity) publicSource {
	return publicSource{
		ID:            pgtypeUUIDString(s.ID),
		Slug:          derefString(s.Slug),
		Name:          s.Name,
		Description:   s.Description,
		Domain:        s.Domain,
		Category:      s.Category,
		SourceType:    s.SourceType,
		SourceKind:    s.SourceKind,
		TrustLevel:    s.TrustLevel,
		FollowerCount: s.FollowerCount,
		PostCount:     s.PostCount,
		LogoURL:       s.LogoUrl,
		IsFollowing:   isFollowing,
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

	tenantID := tenant.IDFromContext(r.Context())
	if !tenantID.Valid {
		tenantID = h.defaultTenant
	}

	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Source slug is required")
		return
	}

	row, err := h.intelStores.GetPublicSourceBySlugWithLinkedEntity(r.Context(), tenantID, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "source_not_found", "Source not found")
			return
		}

		slog.Error("intel: source follow lookup failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_lookup_failed", "Failed to load source")
		return
	}

	isFollowing, err := h.followsSvc.ToggleFollow(r.Context(), tenantID, userID, follows.EntityTypeSource, row.ID)
	if err != nil {
		slog.Error("intel: source follow toggle failed", "slug", slug, "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "source_follow_failed", "Failed to follow source")
		return
	}

	// Re-fetch the source to get the updated follower_count.
	src, err := h.intelSources.Get(r.Context(), tenantID, row.ID)
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
