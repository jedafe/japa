// Package content — edge CDN invalidation and cache-tag emission for
// content. Purge calls are fire-and-forget beside the emitWebhook
// emissions — a CDN outage must never block or fail the publish path
// (Rule 8). Everything no-ops when integrations.cdn.provider is unset.
//
// Ref: japaconnect-engineering/adr/ADR-134-edge-cdn-integration.md
//
//	sections/26-seo-performance.md (Edge CDN Integration)
package content

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/cdn"
)

// purgeEdgeContent invalidates the edge cache for a content item: its
// canonical detail page for every slug supplied (pass the old slug too when
// it changed) plus the content/type cache tags. Fire-and-forget — see
// cdn.PurgeAsync.
func (s *Service) purgeEdgeContent(ctx context.Context, id pgtype.UUID, contentType string, slugs ...string) {
	cdn.PurgeContentDetail(ctx, id.String(), contentType, slugs...)
}

// EmitEdgeCacheTags sets the configured provider's cache-tag header
// (CF-Cache-Tag / CDN-Tag) on a public content detail response so the edge
// can invalidate it by tag later. Only published, public content is
// edge-cacheable; no-op when no CDN is configured. Exported so the blog
// detail handler (internal/blog) emits identical tags.
func EmitEdgeCacheTags(w http.ResponseWriter, id pgtype.UUID, contentType, status, visibility string) {
	if status != StatusPublished || visibility != VisibilityPublic {
		return
	}

	cdn.SetCacheTags(w, "content:"+id.String(), "type:"+contentType)
}
