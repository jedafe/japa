// Package spaces — HTTP handlers for the Geo-First Space System.
//
// Every handler is a method on Handlers so dependencies (Service and
// UnreadCounter) are injected via the constructor, never read from globals.
// Responses follow the JapaConnect API envelope: success bodies are wrapped
// in {"data": ...} and errors in {"error":{"code":"...","message":"..."}}.
// Permission enforcement is handled by middleware wired at the router level;
// these handlers focus on parsing, validation, and Service delegation.
package spaces

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/content/visibility"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/literals"
	"github.com/jedafe/japa/internal/media"
)

// ContentService is the subset of content.Service the space content/pinned
// handlers need.
type ContentService interface {
	ListContentsBySpace(ctx context.Context, spaceID pgtype.UUID, cursor pgtype.Timestamptz, limit int32, filter visibility.VisibilityFilter) ([]gen.ListContentsBySpaceRow, error)
	SearchContentsBySpace(ctx context.Context, spaceID pgtype.UUID, query string, limit, offset int32, filter visibility.VisibilityFilter) ([]gen.SearchContentsBySpaceRow, error)
	PinContent(ctx context.Context, spaceID, contentID, pinnedBy pgtype.UUID, position int32) (gen.SpacePinnedPost, error)
	UnpinContent(ctx context.Context, spaceID, contentID pgtype.UUID) error
	ListPinnedContent(ctx context.Context, spaceID pgtype.UUID) ([]gen.SpacePinnedPost, error)
	ListPinnedContentBySpace(ctx context.Context, spaceID pgtype.UUID, filter visibility.VisibilityFilter) ([]gen.ListPinnedContentBySpaceRow, error)
	// ResolveContentFields resolves {{...}} template literals in the named
	// text fields of content-like structs/slices (§14 literals).
	ResolveContentFields(ctx context.Context, v any, rc literals.RenderContext) error
}

// PermCacheInvalidator flushes a user's permission cache (jc:perm:{user_id})
// after a permission-affecting mutation. Implemented by rbac.RBAC.
// Ref: AGENTS.md Rule 6, sections/03-rbac.md (Permission Cache Invalidation).
type PermCacheInvalidator interface {
	InvalidateUserCache(ctx context.Context, userID pgtype.UUID)
}

// MediaResolver — Handlers holds dependencies for space HTTP handlers. The Service wraps the
// reader/writer database pools; the UnreadCounter tracks per-user unread
// post counts via Valkey (nil is safe — it degrades to no-op).
// MediaResolver resolves a media ID to an imgproxy-signed URL for the space
// logo. The media.Service satisfies this. Nil = logo_url omitted from responses.
type MediaResolver interface {
	GetMedia(ctx context.Context, mediaID pgtype.UUID) (gen.Medium, error)
	GetMediaURL(m *gen.Medium, opts media.ImageProxyOpts) string
}

// RAGEntityIndexer is the subset of the RAG indexer that spaces needs to
// index space content into content_embeddings (§69/ADR-069). Implemented
// by rag.Service; nil disables RAG indexing (graceful degradation).
type RAGEntityIndexer interface {
	IndexEntity(ctx context.Context, entityType string, entityID pgtype.UUID, visibility string, spaceID, authorID pgtype.UUID, body, language string) error
	PurgeEntity(ctx context.Context, entityType string, entityID pgtype.UUID) error
	// ReconcileSpaceScope refreshes scope metadata for a space and its
	// forum threads after a visibility/status change (§69 amendment).
	ReconcileSpaceScope(ctx context.Context, spaceID pgtype.UUID, visibility string) error
	// ReconcileSpaceDelete fail-closes thread embeddings whose parent space
	// is being deleted. Must run BEFORE the delete commits (§69 amendment).
	ReconcileSpaceDelete(ctx context.Context, spaceID pgtype.UUID) error
}

// settingsReader is the subset of config.Settings needed by the spaces
// handlers to resolve hot-reloadable system_settings (e.g.
// i18n.default_language for RAG indexing). Nil falls back to defaults.
type settingsReader interface {
	GetStringOr(ctx context.Context, key, fallback string) string
}

// Handlers is an exported type.
type Handlers struct {
	Service    *Service
	Unread     *UnreadCounter
	content    ContentService
	follows    FollowToggleService
	permCache  PermCacheInvalidator
	media      MediaResolver    // nil = logo_url omitted from responses
	ragIndexer RAGEntityIndexer // nil disables RAG indexing (§69)
	settings   settingsReader   // nil falls back to defaults
}

// FollowToggleService is the subset of the follows.Service used by the space
// handler to toggle follow status. Routing space follow through the generic
// follows service ensures auto-join for open spaces (BL-010 / Section 31)
// and notification dispatch are applied consistently (H3).
type FollowToggleService interface {
	ToggleFollow(ctx context.Context, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error)
}

// SetFollowToggleService injects the generic follows service for space follow.
func (h *Handlers) SetFollowToggleService(s FollowToggleService) {
	h.follows = s
}

// SetPermCacheInvalidator injects the RBAC resolver so handlers can flush
// the TARGET user's permission cache after moderator assign/remove (Rule 6).
// The middleware-level RequirePermissionWithInvalidation only flushes the
// CALLER's cache; the target user's cache must be flushed here.
func (h *Handlers) SetPermCacheInvalidator(p PermCacheInvalidator) {
	h.permCache = p
}

// SetMediaResolver wires the media service used to resolve logo_media_id to a
// signed imgproxy URL (returned as logo_url in space responses). Optional.
func (h *Handlers) SetMediaResolver(m MediaResolver) { h.media = m }

// SetRAGIndexer injects the RAG indexer for embedding space content into
// content_embeddings (§69/ADR-069 BUG-001 fix). nil disables RAG indexing.
func (h *Handlers) SetRAGIndexer(r RAGEntityIndexer) { h.ragIndexer = r }

// SetSettings injects the hot-reloadable settings reader for resolving
// system_settings (e.g. i18n.default_language for RAG indexing). nil falls
// back to compiled-in defaults.
func (h *Handlers) SetSettings(s settingsReader) { h.settings = s }

// indexSpaceRAG fires-and-forgets a RAG indexing call for a space. The body
// is the space name + description + keywords concatenated for embedding.
// Errors are logged, never propagated (Rule 8).
func (h *Handlers) indexSpaceRAG(ctx context.Context, spaceID pgtype.UUID, visibility, name, description string, keywords []string) {
	if h.ragIndexer == nil {
		return
	}

	text := name
	if description != "" {
		if text != "" {
			text += "\n\n"
		}

		text += description
	}

	if len(keywords) > 0 {
		if text != "" {
			text += "\n\n"
		}

		text += strings.Join(keywords, ", ")
	}

	if text == "" {
		return
	}
	// BUG-002 (rag-audit): resolve the indexing language from
	// i18n.default_language (§7 i18n) instead of hardcoding "en". Falls back
	// to "en" when settings are not injected or the key is unset.
	language := "en"
	if h.settings != nil {
		language = h.settings.GetStringOr(ctx, "i18n.default_language", "en")
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx := context.Background()
		if err := h.ragIndexer.IndexEntity(bgCtx, "space", spaceID, visibility, spaceID, pgtype.UUID{}, text, language); err != nil {
			slog.Warn("spaces: RAG index failed for space",
				"space_id", spaceID.String(), "error", err)
		}
	}()
}

// purgeSpaceRAG fires-and-forgets a RAG purge for a deleted space.
func (h *Handlers) purgeSpaceRAG(ctx context.Context, spaceID pgtype.UUID) {
	if h.ragIndexer == nil {
		return
	}

	go func() { // #nosec G118 -- fire-and-forget: request ctx is cancelled on HTTP return (Rule 8)
		bgCtx := context.Background()
		if err := h.ragIndexer.PurgeEntity(bgCtx, "space", spaceID); err != nil {
			slog.Warn("spaces: RAG purge failed for space",
				"space_id", spaceID.String(), "error", err)
		}
	}()
}

// resolveLogoURL resolves a space's logo_media_id to an imgproxy-signed URL
// via the media service. Returns empty when no logo is set, the media
// resolver is not wired, or the media item cannot be loaded. Best-effort.
func (h *Handlers) resolveLogoURL(ctx context.Context, logoMediaID pgtype.UUID) string {
	if !logoMediaID.Valid || h.media == nil {
		return ""
	}

	m, err := h.media.GetMedia(ctx, logoMediaID)
	if err != nil {
		return ""
	}

	return h.media.GetMediaURL(&m, media.ImageProxyOpts{Width: 128, Height: 128, Format: "webp"})
}

// resolveHeaderURL resolves a space's header_image_id to an imgproxy-signed
// URL via the media service. Returns empty when no header is set, the media
// resolver is not wired, or the media item cannot be loaded. Best-effort.
func (h *Handlers) resolveHeaderURL(ctx context.Context, headerMediaID pgtype.UUID) string {
	if !headerMediaID.Valid || h.media == nil {
		return ""
	}

	m, err := h.media.GetMedia(ctx, headerMediaID)
	if err != nil {
		return ""
	}

	return h.media.GetMediaURL(&m, media.ImageProxyOpts{Width: 1280, Height: 320, Format: "webp"})
}

// invalidateTargetPermCache flushes the target user's permission cache after
// a moderator assignment/removal. It is a no-op when no invalidator is wired
// (graceful degradation). Errors are logged but never fail the request — the
// mutation has already succeeded and the cache will expire via TTL.
func (h *Handlers) invalidateTargetPermCache(ctx context.Context, targetUserID pgtype.UUID) {
	if h.permCache == nil {
		return
	}

	h.permCache.InvalidateUserCache(ctx, targetUserID)
}

// NewHandlers creates Handlers with the given Service and UnreadCounter.
func NewHandlers(service *Service, unread *UnreadCounter, contentService ContentService) *Handlers {
	return &Handlers{Service: service, Unread: unread, content: contentService}
}

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid %s: %w", param, err)
	}

	return id, nil
}

// handleServiceError translates a Service-layer error into the appropriate HTTP
// response. It centralizes the not-found, duplicate, and fallback mapping so
// every handler stays consistent.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// spaceFromContext writes a 404 error and returns false when no resolved space
// is present in the request context. Callers should abort when the boolean is
// false.
func spaceFromContext(w http.ResponseWriter, r *http.Request) (*gen.GetSpaceBySlugRow, bool) {
	space := FromContext(r.Context())
	if space == nil {
		errpkg.WriteError(w, http.StatusNotFound, "space_not_found", "Space not found")
		return nil, false
	}

	return space, true
}
