// lifecycle.go — fire-and-forget side effects for blog post writes.
// Blog posts are contents rows, so the blog HTTP handlers must reproduce
// the enqueue fan-out the generic content handlers perform after
// create/update/delete: Meilisearch sync (§12), RAG embeddings (§69),
// moderation scan (§41), feed fan-out (§32), affiliate link scan (§55),
// and link preview fetches (§15). Every enqueue is fire-and-forget per
// Rule 8 — errors are logged, never propagated.
// Ref: japaconnect-engineering/sections/24-blog-extension.md
// Ref: internal/content/handlers.go (the enqueue helpers this mirrors).
package blog

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/worker"
)

// enqueueSearchIndex enqueues a search:index_entity task for a blog post.
// operation is "index" (upsert) or "delete" — the worker maps entity_type
// "content" to the posts index. Fire-and-forget.
func (h *Handlers) enqueueSearchIndex(contentID pgtype.UUID, operation string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id":  contentID.String(),
		"entity_type": "content",

		"operation": operation,
	})
	if err != nil {
		slog.Warn("blog: failed to marshal search index payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeSearchIndexSync, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeSearchIndexSync)),
	); err != nil {
		slog.Warn("blog: failed to enqueue search index task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueContentEmbedding enqueues the content:generate_embedding task so
// the post is (re-)embedded for RAG retrieval with fresh scope metadata
// (§69). Fire-and-forget.
func (h *Handlers) enqueueContentEmbedding(contentID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("blog: failed to marshal embedding task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeAIGenerateContentEmbedding, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeAIGenerateContentEmbedding)),
	); err != nil {
		slog.Warn("blog: failed to enqueue embedding task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueModerationScan enqueues the content:scan_moderation task for a
// blog post (§41). Fire-and-forget.
func (h *Handlers) enqueueModerationScan(contentID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"entity_type": "content",
		"entity_id":   contentID.String(),
	})
	if err != nil {
		slog.Warn("blog: failed to marshal moderation scan payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeModerationScan, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeModerationScan)),
	); err != nil {
		slog.Warn("blog: failed to enqueue moderation scan task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueAffiliateLinkScan enqueues the content:scan_affiliate_links task
// (§55). The worker reads affiliate.scan_on_publish and exits early when
// disabled. Fire-and-forget.
func (h *Handlers) enqueueAffiliateLinkScan(contentID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),
	})
	if err != nil {
		slog.Warn("blog: failed to marshal affiliate scan task payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeContentScanAffiliateLinks, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeContentScanAffiliateLinks)),
	); err != nil {
		slog.Warn("blog: failed to enqueue affiliate scan task", "content_id", contentID.String(), "error", err)
	}
}

// enqueueFeedFanOut enqueues content:fan_out_to_feeds tasks when a blog
// post goes live (§32): once to the author's followers, then per assigned
// space (content_spaces rows, falling back to the primary space_id) with a
// bubble task for parent-space propagation. Mirrors the content handler's
// fan-out; the worker decides push vs pull per feed.fanout_threshold.
// Fire-and-forget per Rule 8.
// Ref: japaconnect-engineering/sections/32-activity-feed-timeline.md.
func (h *Handlers) enqueueFeedFanOut(ctx context.Context, contentID, authorID, primarySpaceID pgtype.UUID) {
	if h.AsynqClient == nil {
		return
	}

	// 1. Always fan out to the author's followers.
	h.enqueueFanOutTask(contentID, authorID, pgtype.UUID{}, "followers")

	// 2. Fan out to each assigned space via content_spaces (Section 14).
	spaceIDs := []pgtype.UUID{}

	if h.content != nil {
		rows, err := h.content.GetContentSpaces(ctx, contentID)
		if err != nil {
			slog.Warn("blog: failed to fetch content_spaces for fan-out, using primary space_id", "content_id", contentID.String(), "error", err)
		} else {
			for _, r := range rows {
				spaceIDs = append(spaceIDs, r.SpaceID)
			}
		}
	}

	if len(spaceIDs) == 0 && primarySpaceID.Valid {
		spaceIDs = append(spaceIDs, primarySpaceID)
	}

	// 3. Enqueue a "space" task and a "bubble" task per assigned space.
	for _, sid := range spaceIDs {
		h.enqueueFanOutTask(contentID, authorID, sid, "space")
		h.enqueueFanOutTask(contentID, authorID, sid, "bubble")
	}
}

// enqueueFanOutTask marshals and enqueues a single fan-out task.
func (h *Handlers) enqueueFanOutTask(contentID, authorID, spaceID pgtype.UUID, fanOutType string) {
	payload, err := json.Marshal(map[string]string{
		"content_id": contentID.String(),

		"author_id":    authorID.String(),
		"space_id":     spaceID.String(),
		"content_type": contentTypeBlogPost,
		"fan_out_type": fanOutType,
	})
	if err != nil {
		slog.Warn("blog: failed to marshal fan-out payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeContentFanOutToFeeds, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeContentFanOutToFeeds)),
	); err != nil {
		slog.Warn("blog: failed to enqueue fan-out task", "content_id", contentID.String(), "fan_out_type", fanOutType, "error", err)
	}
}

// blogHrefURLRegex extracts http(s) URLs from href attributes in HTML.
var blogHrefURLRegex = regexp.MustCompile(`(?i)href="(https?://[^"]+)"`)

// blogDataURLRegex extracts http(s) URLs from data-url attributes — link
// preview block nodes are stored as <div data-type="link-preview"
// data-url="https://..."> (§15).
var blogDataURLRegex = regexp.MustCompile(`(?i)data-url="(https?://[^"]+)"`)

// enqueueLinkPreview enqueues a content:fetch_link_preview task for one
// external URL (§15 — async OG/oEmbed scrape). Fire-and-forget.
func (h *Handlers) enqueueLinkPreview(url, blockID string) {
	if h.AsynqClient == nil {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"url":      url,
		"block_id": blockID,
	})
	if err != nil {
		slog.Warn("blog: failed to marshal link preview payload", "error", err)
		return
	}

	if _, err := h.AsynqClient.Enqueue(
		asynq.NewTask(worker.TypeLinkPreviewFetch, payload),
		asynq.Queue(worker.QueueForTask(worker.TypeLinkPreviewFetch)),
	); err != nil {
		slog.Warn("blog: failed to enqueue link preview task", "url", url, "error", err)
	}
}

// enqueueLinkPreviewsForBody extracts external http(s) URLs from the
// sanitized HTML actually stored and enqueues a link preview fetch for
// each (§15). Deduplicates across href and data-url sources.
func (h *Handlers) enqueueLinkPreviewsForBody(bodyHTML string, contentID pgtype.UUID) {
	if h.AsynqClient == nil || bodyHTML == "" {
		return
	}

	seen := make(map[string]bool)
	blockID := contentID.String()

	allMatches := [][]string{}
	allMatches = append(allMatches, blogHrefURLRegex.FindAllStringSubmatch(bodyHTML, -1)...)
	allMatches = append(allMatches, blogDataURLRegex.FindAllStringSubmatch(bodyHTML, -1)...)

	for _, m := range allMatches {
		u := m[1]
		if seen[u] {
			continue
		}

		seen[u] = true

		h.enqueueLinkPreview(u, blockID)
	}
}

// contentWriteEffects bundles the side effects every blog create/update
// must fire so blog posts stay in parity with posts written through the
// generic content handlers: search index sync, moderation scan, and link
// preview fetches. Fire-and-forget.
func (h *Handlers) contentWriteEffects(bodyHTML string, contentID pgtype.UUID) {
	h.enqueueSearchIndex(contentID, "index")
	h.enqueueModerationScan(contentID)
	h.enqueueLinkPreviewsForBody(bodyHTML, contentID)
}

// publishEffects bundles the extra work a born-published (or
// transition-to-published) blog post needs beyond the shared write
// effects: a content_scores row (§32 — feed queries INNER JOIN it), the
// affiliate link scan (§55), feed fan-out (§32), and RSS regeneration
// (§24 C6). UpdateContentStatus already covers scores/RAG/search/webhooks
// for dedicated status transitions; this helper exists for the create and
// PATCH paths where the blog handler bypasses the content HTTP layer.
// Fire-and-forget per Rule 8.
func (h *Handlers) publishEffects(ctx context.Context, contentID, authorID, spaceID pgtype.UUID) {
	if h.content != nil {
		if err := h.content.EnsureContentScore(ctx, contentID); err != nil {
			slog.Warn("blog: failed to ensure content_scores row on publish", "content_id", contentID.String(), "error", err)
		}
	}

	h.enqueueAffiliateLinkScan(contentID)
	h.enqueueFeedFanOut(ctx, contentID, authorID, spaceID)
	h.enqueueRSSRegenerate()
}
