package content

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

// ListPublishedByAuthor returns a user's published+public content, newest first,
// paginated. Powers the public profile Posts tab (GET /users/{id}/content).
func (s *Service) ListPublishedByAuthor(ctx context.Context, tenantID, authorID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByAuthorRow, error) {
	rows, err := s.read.ListPublishedContentByAuthor(ctx, gen.ListPublishedContentByAuthorParams{
		TenantID:   tenantID,
		AuthorID:   authorID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing published content by author: %w", err)
	}

	return rows, nil
}

// ListByAuthor handles GET /users/{id}/content — the public profile Posts tab.
// Author-scoped: fixes the prior wiring to ListContent, which ignored {id} and
// returned all tenant content. Public-safe: only published+public content;
// non-active users are already 404'd by the requireActiveProfileUser gate.
func (h *Handlers) ListByAuthor(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var authorID pgtype.UUID
	if err := authorID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32
	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, err := h.Service.ListPublishedByAuthor(r.Context(), tenantID, authorID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", "Failed to list content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toPostCards(items)})
}

// OrgRef is the lean org reference embedded in an org-attributed PostCard so
// the "on behalf of <Org>" badge renders. Nil for personal posts.
type OrgRef struct {
	ID          pgtype.UUID `json:"id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	LogoMediaID pgtype.UUID `json:"logo_media_id"`
}

// orgRefFrom builds an OrgRef badge from the joined org columns, or nil when the
// post is not org-attributed. Shared by the user and org Posts-tab mappers.
func orgRefFrom(orgID pgtype.UUID, name, slug *string, logoMediaID pgtype.UUID) *OrgRef {
	if !orgID.Valid {
		return nil
	}
	ref := &OrgRef{ID: orgID, LogoMediaID: logoMediaID}
	if name != nil {
		ref.Name = *name
	}
	if slug != nil {
		ref.Slug = *slug
	}
	return ref
}

// PostCard is the lean public shape for the profile Posts tab. It omits the
// internal columns the raw gen row carries — tenant_id, the raw TipTap `body`
// (body_html is the rendered public form), field_values, content_hash,
// geo_space_id, canonical_url, status/visibility (always published+public
// here), is_locked, reshare_count, publish_at, updated_at.
type PostCard struct {
	ID              pgtype.UUID        `json:"id"`
	AuthorID        pgtype.UUID        `json:"author_id"`
	OrganizationID  pgtype.UUID        `json:"organization_id"`
	Organization    *OrgRef            `json:"organization,omitempty"`
	SpaceID         pgtype.UUID        `json:"space_id"`
	ContentType     string             `json:"content_type"`
	Title           string             `json:"title"`
	Slug            string             `json:"slug"`
	BodyHtml        string             `json:"body_html"`
	HeaderImageID   pgtype.UUID        `json:"header_image_id"`
	FeaturedImageID pgtype.UUID        `json:"featured_image_id"`
	ReadTimeMinutes int32              `json:"read_time_minutes"`
	ViewCount       int32              `json:"view_count"`
	CommentCount    int32              `json:"comment_count"`
	ShareCount      int32              `json:"share_count"`
	IsPinned        bool               `json:"is_pinned"`
	PublishedAt     pgtype.Timestamptz `json:"published_at"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
}

func toPostCards(items []gen.ListPublishedContentByAuthorRow) []PostCard {
	cards := make([]PostCard, len(items))
	for i := range items {
		c := &items[i]
		cards[i] = PostCard{
			ID:              c.ID,
			AuthorID:        c.AuthorID,
			OrganizationID:  c.OrganizationID,
			Organization:    orgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoMediaID),
			SpaceID:         c.SpaceID,
			ContentType:     c.ContentType,
			Title:           c.Title,
			Slug:            c.Slug,
			BodyHtml:        c.BodyHtml,
			HeaderImageID:   c.HeaderImageID,
			FeaturedImageID: c.FeaturedImageID,
			ReadTimeMinutes: c.ReadTimeMinutes,
			ViewCount:       c.ViewCount,
			CommentCount:    c.CommentCount,
			ShareCount:      c.ShareCount,
			IsPinned:        c.IsPinned,
			PublishedAt:     c.PublishedAt,
			CreatedAt:       c.CreatedAt,
		}
	}
	return cards
}

// ListPublishedByOrganization returns an org's published+public content, newest
// first, paginated. Powers the org profile Posts tab (GET /organizations/{id}/content).
func (s *Service) ListPublishedByOrganization(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByOrganizationRow, error) {
	rows, err := s.read.ListPublishedContentByOrganization(ctx, gen.ListPublishedContentByOrganizationParams{
		TenantID:       tenantID,
		OrganizationID: orgID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing published content by organization: %w", err)
	}
	return rows, nil
}

// ListByOrganization handles GET /organizations/{id}/content — the org profile
// Posts tab. Non-active orgs are already 404'd by the requireActiveOrgByID gate.
func (h *Handlers) ListByOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
		return
	}

	limit := parsePageSize(r)
	var offset int32
	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, err := h.Service.ListPublishedByOrganization(r.Context(), tenantID, orgID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", "Failed to list content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toOrgPostCards(items)})
}

func toOrgPostCards(items []gen.ListPublishedContentByOrganizationRow) []PostCard {
	cards := make([]PostCard, len(items))
	for i := range items {
		c := &items[i]
		cards[i] = PostCard{
			ID:              c.ID,
			AuthorID:        c.AuthorID,
			OrganizationID:  c.OrganizationID,
			Organization:    orgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoMediaID),
			SpaceID:         c.SpaceID,
			ContentType:     c.ContentType,
			Title:           c.Title,
			Slug:            c.Slug,
			BodyHtml:        c.BodyHtml,
			HeaderImageID:   c.HeaderImageID,
			FeaturedImageID: c.FeaturedImageID,
			ReadTimeMinutes: c.ReadTimeMinutes,
			ViewCount:       c.ViewCount,
			CommentCount:    c.CommentCount,
			ShareCount:      c.ShareCount,
			IsPinned:        c.IsPinned,
			PublishedAt:     c.PublishedAt,
			CreatedAt:       c.CreatedAt,
		}
	}
	return cards
}
