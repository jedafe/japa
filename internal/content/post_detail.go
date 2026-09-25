// Post detail / social wrapper handler (§14, §23, §32).
//
// GET /api/v1/post/{content_type}/{slug} is the social "post page" route
// used by the unified daily card. It returns the same public detail shape as
// GET /content/{slug} but disambiguates by content_type so the same slug can
// exist for multiple content types.
package content

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/mentions"
)

// GetPost handles GET /api/v1/post/{content_type}/{slug}.
func (h *Handlers) GetPost(w http.ResponseWriter, r *http.Request) {

	contentType := chi.URLParam(r, "content_type")
	if contentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_content_type", "Content type is required")
		return
	}

	slug := chi.URLParam(r, "slug")
	if slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "Slug is required")
		return
	}

	// Enriched detail row, disambiguated by both content_type and slug.
	content, err := h.Service.GetContentDetailBySlugAndType(r.Context(), contentType, slug)
	if err != nil {
		// Try slug redirect before returning not found.
		if newSlug, found := h.Service.ResolveSlugRedirect(r.Context(), EntityContent, slug); found {
			content, err = h.Service.GetContentDetailBySlugAndType(r.Context(), contentType, newSlug)
		}

		if err != nil {
			handleServiceError(w, err)
			return
		}
	}

	// Visibility/status gate — same rules as GET /content/{slug}.
	user := auth.FromContext(r.Context())
	if content.Status != StatusPublished {
		if user == nil || !canModifyContent(r, user.UserID, content.AuthorID, "content.edit_any") {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
			return
		}
	} else {
		switch content.Visibility {
		case VisibilityPublic:
			// always allowed
		case VisibilityMembers:
			if user == nil {
				errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
				return
			}
		case VisibilityPrivate:
			if user == nil || !canModifyContent(r, user.UserID, content.AuthorID, "content.edit_any") {
				errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
				return
			}
		default:
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Content not found")
			return
		}
	}

	// Resolve template literals in the content title, body, and meta fields.
	rc := buildRenderContext(r, "")
	if err := h.Service.ResolveContentFields(r.Context(), &content, rc); err != nil {
		slog.Warn("content: failed to resolve content literals",
			"error", err, "content_id", content.ID.String())
	}

	// GDPR / ADR-029: redact mentions of erased users in body_html.
	var erasedIDs []string
	if h.mentionCheckers != nil {
		redactedHTML, ids, redErr := mentions.RedactErasedMentionsInBody(
			r.Context(),
			[]byte(content.Body),
			content.BodyHtml,

			h.mentionCheckers,
		)
		if redErr != nil {
			slog.Warn("content: RedactErasedMentionsInBody failed",
				"content_id", content.ID.String(), "error", redErr)
		} else {
			content.BodyHtml = redactedHTML
			erasedIDs = ids
		}
	}

	// Same public detail DTO as GET /content/{slug}, with reaction_count included.
	dto := map[string]any{
		"id":                content.ID,
		"author_id":         content.AuthorID,
		"space_id":          content.SpaceID,
		"organization_id":   content.OrganizationID,
		"content_type":      content.ContentType,
		"title":             content.Title,
		"slug":              content.Slug,
		"body":              content.Body,
		"body_html":         content.BodyHtml,
		"content_hash":      content.ContentHash,
		"geo_space_id":      content.GeoSpaceID,
		"header_image_id":   content.HeaderImageID,
		"featured_image_id": content.FeaturedImageID,
		"canonical_url":     content.CanonicalUrl,
		"meta_title":        content.MetaTitle,
		"meta_description":  content.MetaDescription,
		"og_image_url":      content.OgImageUrl,
		"seo_indexable":     content.SeoIndexable,
		"status":            content.Status,
		"visibility":        content.Visibility,
		"is_locked":         content.IsLocked,
		"is_pinned":         content.IsPinned,
		"read_time_minutes": content.ReadTimeMinutes,
		"view_count":        content.ViewCount,
		"comment_count":     content.CommentCount,
		"share_count":       content.ShareCount,
		"reshare_count":     content.ReshareCount,
		"reaction_count":    content.ReactionCount,
		"publish_at":        content.PublishAt,
		"published_at":      content.PublishedAt,
		"created_at":        content.CreatedAt,
		"updated_at":        content.UpdatedAt,
		"guide_category":    content.GuideCategory,
		"journey_stage":     content.JourneyStage,
		"visa_route":        content.VisaRoute,
		"source_country":    content.SourceCountry,
		"target_country":    content.TargetCountry,
		"video_source_id":   content.VideoSourceID,
		"author_name":       content.AuthorName,
		"author_username":   content.AuthorUsername,
		"author_avatar_url": content.AuthorAvatarUrl,
		"space_name":        content.SpaceName,
		"space_slug":        content.SpaceSlug,
		"space_avatar_url":  mediaURL(content.SpaceAvatarStorageBucket, content.SpaceAvatarStoragePath),
		"organization":      orgRefFrom(content.OrganizationID, content.OrganizationName, content.OrganizationSlug, content.OrganizationLogoStorageBucket, content.OrganizationLogoStoragePath),
		"excerpt":           MakeExcerpt(content.BodyHtml, 160),
	}

	if len(content.FieldValues) > 0 && json.Valid(content.FieldValues) {
		dto["field_values"] = json.RawMessage(content.FieldValues)
	} else {
		dto["field_values"] = map[string]any{}
	}

	if content.FeaturedStorageBucket != nil && content.FeaturedStoragePath != nil {
		dto["featured_image_url"] = media.MediaURL(*content.FeaturedStorageBucket, *content.FeaturedStoragePath, media.ImageProxyOpts{})
	}

	if content.HeaderStorageBucket != nil && content.HeaderStoragePath != nil {
		dto["header_image_url"] = media.MediaURL(*content.HeaderStorageBucket, *content.HeaderStoragePath, media.ImageProxyOpts{})
	}

	if content.TagsCsv != "" {
		dto["tags"] = strings.Split(content.TagsCsv, ",")
	} else {
		dto["tags"] = []string{}
	}

	if content.SeriesID.Valid {
		dto["series_id"] = content.SeriesID
		if content.SeriesTitle != nil {
			dto["series_title"] = *content.SeriesTitle
		}
		if content.SeriesSlug != nil {
			dto["series_slug"] = *content.SeriesSlug
		}
		if content.SeriesPart != nil {
			dto["series_part"] = *content.SeriesPart
		}
		dto["series_total"] = content.SeriesTotal
		if content.SeriesPrevSlug != "" {
			dto["series_prev_slug"] = content.SeriesPrevSlug
		}
		if content.SeriesNextSlug != "" {
			dto["series_next_slug"] = content.SeriesNextSlug
		}
	}

	if content.ContentType == TypeGuide && content.GuideCategory != nil && h.guideCategoryLister != nil {
		if cats, catErr := h.guideCategoryLister.ListGuideCategories(r.Context()); catErr == nil {
			for _, cat := range cats {
				if cat.Slug == *content.GuideCategory {
					dto["guide_category_label"] = cat.Label
					break
				}
			}
		}
	}

	resp := map[string]any{"data": dto}
	if len(erasedIDs) > 0 {
		resp["deleted_user_ids"] = erasedIDs
	}

	// JSON-LD structured data using the same template interpolation as the
	// canonical single-type page.
	if jsonLd, jErr := h.Service.ResolveJSONLD(r.Context(), content.ContentType, postContentJSONLDFields(content)); jErr == nil && len(jsonLd) > 0 {
		resp["json_ld"] = json.RawMessage(jsonLd)
	} else if jErr != nil {
		slog.Warn("content: JSON-LD template resolution failed", "slug", slug, "content_type", contentType, "error", jErr)
	}

	errpkg.WriteJSON(w, http.StatusOK, resp)
}

// postContentJSONLDFields builds the {{field_key}} map for the social post
// page. It mirrors contentJSONLDFields but accepts the type-disambiguated row.
func postContentJSONLDFields(c gen.GetContentDetailBySlugAndTypeRow) map[string]string {
	desc := ""
	if c.MetaDescription != nil {
		desc = *c.MetaDescription
	}

	canonical := ""
	if c.CanonicalUrl != nil {
		canonical = *c.CanonicalUrl
	}

	coverImageURL := ""
	if c.FeaturedStorageBucket != nil && c.FeaturedStoragePath != nil {
		coverImageURL = media.MediaURL(*c.FeaturedStorageBucket, *c.FeaturedStoragePath, media.ImageProxyOpts{})
	}
	if coverImageURL == "" && c.VideoThumbnailUrl != nil && *c.VideoThumbnailUrl != "" {
		coverImageURL = *c.VideoThumbnailUrl
	}

	sourceURL := ""
	if c.VideoSourceUrl != nil {
		sourceURL = *c.VideoSourceUrl
	}

	duration := ""
	if c.VideoDurationSecs != nil {
		duration = fmt.Sprintf("%d", *c.VideoDurationSecs)
	}

	sourceType := ""
	if c.VideoSourceType != nil {
		sourceType = *c.VideoSourceType
	}

	return map[string]string{
		"title":           c.Title,
		"description":     desc,
		"body":            c.Body,
		"canonical_url":   canonical,
		"published_at":    tsFormat(c.PublishedAt),
		"updated_at":      tsFormat(c.UpdatedAt),
		"created_at":      tsFormat(c.CreatedAt),
		"cover_image_url": coverImageURL,
		"author_name":     c.AuthorName,
		"source_url":      sourceURL,
		"duration":        duration,
		"source_type":     sourceType,
	}
}
