package content

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/media"
)

// htmlTagPattern matches HTML tags for stripping when building excerpts.
var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// ListPublishedByAuthor returns a user's published+public content, newest first,
// paginated. Powers the public profile Posts tab (GET /users/{id}/content).
func (s *Service) ListPublishedByAuthor(ctx context.Context, authorID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByAuthorRow, error) {
	rows, err := s.read.ListPublishedContentByAuthor(ctx, gen.ListPublishedContentByAuthorParams{

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
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var authorID pgtype.UUID
	if err := authorID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.ParseInt(o, 10, 32); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, err := h.Service.ListPublishedByAuthor(r.Context(), authorID, limit, offset)
	if err != nil {
		slog.Error("content: ListByAuthor failed", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to list content")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toPostCards(items)})
}

// mediaURL returns an imgproxy/local serving URL for a media row, or an empty
// string when either bucket or path is nil.
func mediaURL(bucket, path *string) string {
	if bucket == nil || path == nil {
		return ""
	}

	return media.MediaURL(*bucket, *path, media.ImageProxyOpts{})
}

// coverImageURL resolves a cover image from storage bucket/path, falling back to
// the provided URL (typically a video thumbnail) when no stored media is present.
func coverImageURL(bucket, path *string, fallbackURL string) string {
	if url := mediaURL(bucket, path); url != "" {
		return url
	}

	if fallbackURL != "" {
		return fallbackURL
	}

	return ""
}

// stringFromInterface coerces sqlc's interface{} aggregation columns (e.g. tags)
// into a string. It returns the empty string for nil.
func stringFromInterface(v interface{}) string {
	if v == nil {
		return ""
	}

	return fmt.Sprintf("%v", v)
}

// splitTags splits a comma-separated tag string into a slice, discarding empty
// entries.
func splitTags(csv string) []string {
	if csv == "" {
		return nil
	}

	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// OrgRef is the lean org reference embedded in an org-attributed PostCard so
// the "on behalf of <Org>" badge renders. Nil for personal posts.
// Ref: japaconnect-engineering/sections/07-organization-profiles.md
type OrgRef struct {
	ID      pgtype.UUID `json:"id"`
	Name    string      `json:"name"`
	Slug    string      `json:"slug"`
	LogoURL string      `json:"logo_url,omitempty"`
}

// orgRefFrom builds an OrgRef badge from the joined org columns, or nil when the
// post is not org-attributed. The logo URL is resolved from storage bucket/path.
func orgRefFrom(orgID pgtype.UUID, name, slug *string, logoBucket, logoPath *string) *OrgRef {
	if !orgID.Valid {
		return nil
	}

	ref := &OrgRef{ID: orgID, LogoURL: mediaURL(logoBucket, logoPath)}
	if name != nil {
		ref.Name = *name
	}

	if slug != nil {
		ref.Slug = *slug
	}

	return ref
}

// PostCard is the lean public shape for the profile Posts tab. It omits the
// internal columns the raw gen row carries — , the raw TipTap `body`
// (body_html is the rendered public form), field_values, content_hash,
// geo_space_id, canonical_url, status/visibility (always published+public
// here), is_locked, reshare_count, publish_at, updated_at.
// Ref: japaconnect-engineering/sections/14-content-type-system.md
type PostCard struct {
	ID                pgtype.UUID        `json:"id"`
	ContentType       string             `json:"content_type"`
	Title             string             `json:"title"`
	Slug              string             `json:"slug"`
	Excerpt           string             `json:"excerpt"`
	BodyHtml          string             `json:"body_html"`
	AuthorID          pgtype.UUID        `json:"author_id"`
	AuthorName        string             `json:"author_name"`
	AuthorUsername    string             `json:"author_username"`
	AuthorAvatarURL   string             `json:"author_avatar_url"`
	OrganizationID    pgtype.UUID        `json:"organization_id"`
	Organization      *OrgRef            `json:"organization,omitempty"`
	SpaceID           pgtype.UUID        `json:"space_id"`
	SpaceName         string             `json:"space_name"`
	SpaceSlug         string             `json:"space_slug"`
	SpaceAvatarURL    string             `json:"space_avatar_url"`
	HeaderImageID     pgtype.UUID        `json:"header_image_id"`
	FeaturedImageID   pgtype.UUID        `json:"featured_image_id"`
	CoverImageURL     string             `json:"cover_image_url"`
	ReadTimeMinutes   int32              `json:"read_time_minutes"`
	ViewCount         int32              `json:"view_count"`
	CommentCount      int32              `json:"comment_count"`
	ReactionCount     int32              `json:"reaction_count"`
	BookmarkCount     int32              `json:"bookmark_count"`
	ShareCount        int32              `json:"share_count"`
	Tags              []string           `json:"tags,omitempty"`
	JourneyStage      *string            `json:"journey_stage,omitempty"`
	VisaRoute         *string            `json:"visa_route,omitempty"`
	SourceCountry     *string            `json:"source_country,omitempty"`
	TargetCountry     *string            `json:"target_country,omitempty"`
	GuideCategory     *string            `json:"guide_category,omitempty"`
	VideoSourceID     pgtype.UUID        `json:"video_source_id"`
	VideoSourceURL    string             `json:"video_source_url"`
	VideoSourceType   string             `json:"video_source_type"`
	VideoDurationSecs int32              `json:"video_duration_secs"`
	VideoThumbnailURL string             `json:"video_thumbnail_url"`
	IsPinned          bool               `json:"is_pinned"`
	PublishedAt       pgtype.Timestamptz `json:"published_at"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
}

// makeExcerpt strips HTML tags from bodyHtml, collapses whitespace, and
// truncates to maxLen characters for use as a post preview.
func makeExcerpt(bodyHtml string, maxLen int) string {
	plain := htmlTagPattern.ReplaceAllString(bodyHtml, "")
	plain = strings.Join(strings.Fields(plain), " ")

	if len(plain) > maxLen {
		// Try to cut at a word boundary.
		cut := strings.LastIndex(plain[:maxLen], " ")
		if cut < maxLen/2 {
			cut = maxLen
		}

		plain = plain[:cut] + "…"
	}

	return plain
}

func toPostCards(items []gen.ListPublishedContentByAuthorRow) []PostCard {
	cards := make([]PostCard, len(items))

	for i := range items {
		c := &items[i]
		cards[i] = PostCard{
			ID:                c.ID,
			ContentType:       c.ContentType,
			Title:             c.Title,
			Slug:              c.Slug,
			Excerpt:           makeExcerpt(c.BodyHtml, 200),
			BodyHtml:          c.BodyHtml,
			AuthorID:          c.AuthorID,
			AuthorName:        c.AuthorName,
			AuthorUsername:    c.AuthorUsername,
			AuthorAvatarURL:   c.AuthorAvatarUrl,
			OrganizationID:    c.OrganizationID,
			Organization:      orgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoStorageBucket, c.OrgLogoStoragePath),
			SpaceID:           c.SpaceID,
			SpaceName:         c.SpaceName,
			SpaceSlug:         c.SpaceSlug,
			SpaceAvatarURL:    mediaURL(c.SpaceAvatarStorageBucket, c.SpaceAvatarStoragePath),
			HeaderImageID:     c.HeaderImageID,
			FeaturedImageID:   c.FeaturedImageID,
			CoverImageURL:     coverImageURL(c.CoverStorageBucket, c.CoverStoragePath, derefString(c.VideoThumbnailUrl)),
			ReadTimeMinutes:   c.ReadTimeMinutes,
			ViewCount:         c.ViewCount,
			CommentCount:      c.CommentCount,
			ReactionCount:     c.ReactionCount,
			BookmarkCount:     c.BookmarkCount,
			ShareCount:        c.ShareCount,
			Tags:              splitTags(stringFromInterface(c.Tags)),
			JourneyStage:      c.JourneyStage,
			VisaRoute:         c.VisaRoute,
			SourceCountry:     c.SourceCountry,
			TargetCountry:     c.TargetCountry,
			GuideCategory:     c.GuideCategory,
			VideoSourceID:     c.VideoSourceID,
			VideoSourceURL:    derefString(c.VideoSourceUrl),
			VideoSourceType:   derefString(c.VideoSourceType),
			VideoDurationSecs: derefInt32(c.VideoDurationSecs),
			VideoThumbnailURL: derefString(c.VideoThumbnailUrl),
			IsPinned:          c.IsPinned,
			PublishedAt:       c.PublishedAt,
			CreatedAt:         c.CreatedAt,
		}
	}

	return cards
}

// ListPublishedByOrganization returns an org's published+public content, newest
// first, paginated. Powers the org profile Posts tab (GET /organizations/{id}/content).
func (s *Service) ListPublishedByOrganization(ctx context.Context, orgID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByOrganizationRow, error) {
	rows, err := s.read.ListPublishedContentByOrganization(ctx, gen.ListPublishedContentByOrganizationParams{

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
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
		return
	}

	limit := parsePageSize(r)

	var offset int32

	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.ParseInt(o, 10, 32); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, err := h.Service.ListPublishedByOrganization(r.Context(), orgID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_error", "Failed to list content")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": toOrgPostCards(items)})
}

func toOrgPostCards(items []gen.ListPublishedContentByOrganizationRow) []PostCard {
	cards := make([]PostCard, len(items))

	for i := range items {
		c := &items[i]
		cards[i] = PostCard{
			ID:                c.ID,
			ContentType:       c.ContentType,
			Title:             c.Title,
			Slug:              c.Slug,
			Excerpt:           makeExcerpt(c.BodyHtml, 200),
			BodyHtml:          c.BodyHtml,
			AuthorID:          c.AuthorID,
			AuthorName:        c.AuthorName,
			AuthorUsername:    c.AuthorUsername,
			AuthorAvatarURL:   c.AuthorAvatarUrl,
			OrganizationID:    c.OrganizationID,
			Organization:      orgRefFrom(c.OrganizationID, c.OrgName, c.OrgSlug, c.OrgLogoStorageBucket, c.OrgLogoStoragePath),
			SpaceID:           c.SpaceID,
			SpaceName:         c.SpaceName,
			SpaceSlug:         c.SpaceSlug,
			SpaceAvatarURL:    mediaURL(c.SpaceAvatarStorageBucket, c.SpaceAvatarStoragePath),
			HeaderImageID:     c.HeaderImageID,
			FeaturedImageID:   c.FeaturedImageID,
			CoverImageURL:     coverImageURL(c.CoverStorageBucket, c.CoverStoragePath, derefString(c.VideoThumbnailUrl)),
			ReadTimeMinutes:   c.ReadTimeMinutes,
			ViewCount:         c.ViewCount,
			CommentCount:      c.CommentCount,
			ReactionCount:     c.ReactionCount,
			BookmarkCount:     c.BookmarkCount,
			ShareCount:        c.ShareCount,
			Tags:              splitTags(stringFromInterface(c.Tags)),
			JourneyStage:      c.JourneyStage,
			VisaRoute:         c.VisaRoute,
			SourceCountry:     c.SourceCountry,
			TargetCountry:     c.TargetCountry,
			GuideCategory:     c.GuideCategory,
			VideoSourceID:     c.VideoSourceID,
			VideoSourceURL:    derefString(c.VideoSourceUrl),
			VideoSourceType:   derefString(c.VideoSourceType),
			VideoDurationSecs: derefInt32(c.VideoDurationSecs),
			VideoThumbnailURL: derefString(c.VideoThumbnailUrl),
			IsPinned:          c.IsPinned,
			PublishedAt:       c.PublishedAt,
			CreatedAt:         c.CreatedAt,
		}
	}

	return cards
}

// derefString safely dereferences a *string.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// derefInt32 safely dereferences a *int32, defaulting to 0.
func derefInt32(n *int32) int32 {
	if n == nil {
		return 0
	}

	return *n
}
