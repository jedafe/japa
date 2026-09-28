-- internal/db/queries/intelligence.sql
-- sqlc queries for the intelligence spine tables.
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
--
-- All queries are platform-scoped (ADR-112 Amendment 2). Every query filters
-- by the id to guarantee data isolation across platforms.

-- name: CreateIntelSource :one
-- categories dual-writes category when the caller does not pass the array
-- (reviewer-flagged gap: propose must populate both columns). probe_report
-- carries the preview-probe snapshot submitted with the proposal (§6).
INSERT INTO intel_sources (name, slug, domain, source_url, description, category, source_type, source_kind, trust_level, crawl_method, crawl_frequency, crawl_frequency_mins, robots_policy, source_license, attribution_required, usage_restriction, provider_config, logo_url, keywords, enabled, status, proposed_by, proposed_at, approved_by, approved_at, persona_id, categories, probe_report) VALUES ($1, sqlc.narg(slug), $2, $3, sqlc.narg(description), $4, $5, COALESCE(sqlc.narg(source_kind), 'publisher'), $6, $7, $8, $9, $10, $11, $12, $13, $14, sqlc.narg(logo_url), sqlc.narg(keywords), $15, COALESCE(sqlc.narg(status), 'active'), sqlc.narg(proposed_by), sqlc.narg(proposed_at), sqlc.narg(approved_by), sqlc.narg(approved_at), sqlc.narg(persona_id), COALESCE(sqlc.narg(categories)::text[], CASE WHEN $4 = '' THEN '{}'::text[] ELSE ARRAY[$4] END), COALESCE(sqlc.narg(probe_report)::jsonb, '{}'::jsonb))

RETURNING *;

-- name: GetIntelSource :one
SELECT * FROM intel_sources WHERE id = $1;

-- name: GetIntelSourceByURL :one
SELECT * FROM intel_sources WHERE source_url = $1;

-- name: GetIntelSourceBySlug :one
SELECT * FROM intel_sources WHERE slug = $1;

-- name: ListIntelSources :many
SELECT * FROM intel_sources WHERE true ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: ListIntelSourcesByCategory :many
SELECT * FROM intel_sources WHERE category = $1 AND enabled = true AND status = 'active' ORDER BY trust_level, name;

-- name: ListDueIntelSources :many
SELECT * FROM intel_sources
WHERE enabled = true
 AND status = 'active'
 -- Manual sources (§106 video channels, entity-linked rows) are never
 -- auto-crawled — without this they were always "due", the fetcher failed
 -- on unknown crawl_method, and the failure circuit-breaker auto-disabled
 -- them (removing them from the public directory).
 AND crawl_method != 'manual'
 AND (last_crawled_at IS NULL OR now() - last_crawled_at >= (crawl_frequency_mins || ' minutes')::interval)
ORDER BY last_crawled_at NULLS FIRST
LIMIT $1;

-- name: ListVideoChannelSourcesDue :many
-- Enabled video_channel sources due for a channel-feed sync: never synced,
-- or last_crawled_at older than the given minute interval. Used by the
-- video:sync_channels poller (§106 channel-sync amendment) — distinct from
-- ListDueIntelSources, which excludes crawl_method='manual' by design.
SELECT * FROM intel_sources
WHERE source_type = 'video_channel'
 AND enabled = true
 AND status = 'active'
 AND (last_crawled_at IS NULL OR now() - last_crawled_at >= ($1::int || ' minutes')::interval)
ORDER BY last_crawled_at NULLS FIRST
LIMIT $2;

-- name: UpdateIntelSourceCrawlStatus :exec
UPDATE intel_sources
SET last_crawled_at = now(),
 last_success_at = CASE WHEN sqlc.arg('success')::boolean THEN now() ELSE last_success_at END,
 last_error_at = CASE WHEN sqlc.arg('success')::boolean THEN last_error_at ELSE now() END,
 last_error_message = sqlc.arg('last_error_message'),
 consecutive_failures = CASE WHEN sqlc.arg('success')::boolean THEN 0 ELSE consecutive_failures + 1 END,
 updated_at = now()
WHERE id = $1;

-- name: UpdateIntelSourceLastChanged :exec
UPDATE intel_sources SET last_changed_at = now(), updated_at = now() WHERE id = $1;

-- name: UpdateIntelSourceEnabled :exec
UPDATE intel_sources SET enabled = $2, updated_at = now() WHERE id = $1;

-- name: UpdateIntelSourceRobotsPolicy :exec
UPDATE intel_sources SET robots_policy = $2, updated_at = now() WHERE id = $1;

-- name: UpdateIntelSourceProviderConfig :exec
UPDATE intel_sources SET provider_config = $2, updated_at = now() WHERE id = $1;

-- name: ClearIntelSourcePersona :exec
UPDATE intel_sources SET persona_id = NULL, updated_at = now() WHERE id = $1;

-- name: DeleteIntelSource :exec
DELETE FROM intel_sources WHERE id = $1;

-- name: UpdateIntelSource :one
UPDATE intel_sources
SET name = $2,
 slug = COALESCE(sqlc.narg(slug), slug),
 domain = $3,
 source_url = $4,
 description = COALESCE(sqlc.narg(description), description),
 category = $5,
 source_type = $6,
 source_kind = COALESCE(sqlc.narg(source_kind), source_kind),
 trust_level = $7,
 crawl_method = $8,
 crawl_frequency = $9,
 crawl_frequency_mins = $10,
 source_license = $11,
 attribution_required = $12,
 usage_restriction = $13,
 logo_url = COALESCE(sqlc.narg(logo_url), logo_url),
 keywords = COALESCE(sqlc.narg(keywords), keywords),
 enabled = $14,
 status = COALESCE(sqlc.narg(status), status),
 proposed_by = COALESCE(sqlc.narg(proposed_by), proposed_by),
 proposed_at = COALESCE(sqlc.narg(proposed_at), proposed_at),
 approved_by = COALESCE(sqlc.narg(approved_by), approved_by),
 approved_at = COALESCE(sqlc.narg(approved_at), approved_at),
 persona_id = COALESCE(sqlc.narg(persona_id), persona_id),
 updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateIntelRawDocument :one
INSERT INTO intel_raw_documents (source_id, url, http_status, http_headers, content_type, body, content_hash, channel_id, parent_document_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)

RETURNING *;

-- name: GetIntelRawDocument :one
SELECT * FROM intel_raw_documents WHERE id = $1;

-- name: GetLatestIntelRawDocHash :one
-- Derived (manifest child) docs are excluded — a source-level baseline
-- must compare against top-level captures, not fan-out pages (Phase 2).
SELECT content_hash FROM intel_raw_documents WHERE source_id = $1
  AND parent_document_id IS NULL
ORDER BY retrieved_at DESC LIMIT 1;

-- name: GetLatestIntelRawDocHashForChannelURL :one
-- Derived-fetch (Phase 2 manifest fan-out) prior hash: the newest raw doc
-- for this channel captured from the same URL — child pages compare
-- against their own previous capture, not the manifest's.
SELECT content_hash FROM intel_raw_documents
WHERE channel_id = $1 AND url = $2 ORDER BY retrieved_at DESC LIMIT 1;

-- name: GetIntelRawDocIDByChannelURLHash :one
-- Inbound re-drive idempotency (Phase 3 hardening): email/websub events
-- materialize under a deterministic url (email:{ch}:{msg}, websub:{ch}:
-- {hash}); a re-run that already persisted the doc (crash between insert
-- and terminal mark, or an admin redrive) reuses it instead of stacking
-- a duplicate row.
SELECT id FROM intel_raw_documents
WHERE channel_id = $1 AND url = $2 AND content_hash = $3
ORDER BY retrieved_at DESC LIMIT 1;

-- name: GetPrevIntelRawDocByChannel :one
-- The previous MANIFEST document for a channel (excluding the given doc) —
-- manifest-diff fan-out (sitemap/wayback Phase 2) compares the new
-- manifest body against this one to find new/changed URLs. Derived child
-- docs share channel_id but are page content, not manifests, so they are
-- excluded via parent_document_id IS NULL; the content_type marker
-- additionally keeps non-manifest fallback-ladder docs (e.g. a static
-- fetch that covered a failed sitemap crawl) out of the diff baseline.
SELECT * FROM intel_raw_documents WHERE channel_id = $1 AND id != $2
  AND parent_document_id IS NULL
  AND content_type = 'application/x-intel-manifest'
ORDER BY retrieved_at DESC LIMIT 1;

-- name: GetPrevIntelRawDocForChannelURL :one
-- The previous MANIFEST document for one derived child URL (excluding the
-- given doc) — nested manifest expansion (sitemapindex member fetched as
-- a derived sitemap doc) diffs against the previous capture of THAT URL,
-- not the channel's top-level manifest.
SELECT * FROM intel_raw_documents WHERE channel_id = $1 AND url = $2 AND id != $3
  AND content_type = 'application/x-intel-manifest'
ORDER BY retrieved_at DESC LIMIT 1;

-- name: CreateIntelRecord :one
INSERT INTO intel_records (source_id, raw_document_id, entity_type, external_key, extracted, confidence, evidence, confidence_overall, status, geo_space_ids) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'extracted', $9)

RETURNING *;

-- name: GetIntelRecord :one
SELECT * FROM intel_records WHERE id = $1;

-- name: GetIntelRecordByExternalKey :one
SELECT * FROM intel_records WHERE source_id = $1 AND external_key = $2;

-- name: ListIntelRecordsByExternalKey :many
SELECT * FROM intel_records WHERE external_key = $1 ORDER BY extracted_at DESC;

-- name: ListIntelRecordsByStatus :many
SELECT * FROM intel_records WHERE entity_type = $1 AND status = $2 ORDER BY extracted_at DESC LIMIT $3 OFFSET $4;

-- name: ListIntelRecordsByStatusAll :many
-- Lists intel records by status across ALL entity types (no entity_type
-- filter). Used by the review-queue handler when no entity_type filter is
-- supplied so job/visa/grant records awaiting review are not invisible
-- (BUG-001 fix — the prior handler hardcoded entity_type='scholarship').
SELECT * FROM intel_records WHERE status = $1 ORDER BY extracted_at DESC LIMIT $2 OFFSET $3;

-- name: UpdateIntelRecordStatus :exec
UPDATE intel_records SET status = $2, last_verified_at = now() WHERE id = $1;

-- name: UpdateIntelRecordFreshness :exec
UPDATE intel_records SET freshness_score = $2, status = CASE WHEN $2 <= 40 THEN 'stale' ELSE status END WHERE id = $1;

-- name: UpdateIntelRecordPromoted :exec
UPDATE intel_records SET promoted_record_id = $2, status = 'published' WHERE id = $1;

-- name: UpdateIntelRecordExtracted :exec
-- Merges reviewer-edited fields into the record's extracted payload. Used by
-- the review-edit endpoint when the record has no domain projection row yet
-- (projections are only created by intel:promote after approval), so edits
-- must land on the intel_record itself for promotion to respect them.
UPDATE intel_records SET extracted = $2, last_verified_at = now() WHERE id = $1;

-- name: ListIntelRecordsFiltered :many
-- Filtered intel records list for the admin review queue / records browser:
-- status is required; entity_type and search are optional ('' disables).
-- search matches external_key, entity_type, and the extracted payload.
-- ORDER BY uses whitelisted CASE keys — only the active (sort_key, sort_dir)
-- pair produces non-NULL values, so each inactive key is a no-op; the final
-- extracted_at DESC keeps the default newest-first ordering.
SELECT * FROM intel_records
WHERE status = @status
  AND (@entity_type::text = '' OR entity_type = @entity_type)
  AND (@search::text = '' OR external_key ILIKE '%' || @search || '%'
       OR entity_type ILIKE '%' || @search || '%'
       OR extracted::text ILIKE '%' || @search || '%')
ORDER BY
  CASE WHEN @sort_key::text = 'entity_type'        AND @sort_dir::text = 'asc'  THEN entity_type        END ASC,
  CASE WHEN @sort_key::text = 'entity_type'        AND @sort_dir::text = 'desc' THEN entity_type        END DESC,
  CASE WHEN @sort_key::text = 'external_key'       AND @sort_dir::text = 'asc'  THEN external_key       END ASC,
  CASE WHEN @sort_key::text = 'external_key'       AND @sort_dir::text = 'desc' THEN external_key       END DESC,
  CASE WHEN @sort_key::text = 'status'             AND @sort_dir::text = 'asc'  THEN status             END ASC,
  CASE WHEN @sort_key::text = 'status'             AND @sort_dir::text = 'desc' THEN status             END DESC,
  CASE WHEN @sort_key::text = 'confidence_overall' AND @sort_dir::text = 'asc'  THEN confidence_overall END ASC,
  CASE WHEN @sort_key::text = 'confidence_overall' AND @sort_dir::text = 'desc' THEN confidence_overall END DESC,
  CASE WHEN @sort_key::text = 'freshness_score'    AND @sort_dir::text = 'asc'  THEN freshness_score    END ASC,
  CASE WHEN @sort_key::text = 'freshness_score'    AND @sort_dir::text = 'desc' THEN freshness_score    END DESC,
  CASE WHEN @sort_key::text = 'extracted_at'       AND @sort_dir::text = 'asc'  THEN extracted_at       END ASC,
  CASE WHEN @sort_key::text = 'extracted_at'       AND @sort_dir::text = 'desc' THEN extracted_at       END DESC,
  extracted_at DESC
LIMIT @row_limit OFFSET @row_offset;

-- name: CountIntelRecordsFiltered :one
SELECT count(*) FROM intel_records
WHERE status = @status
  AND (@entity_type::text = '' OR entity_type = @entity_type)
  AND (@search::text = '' OR external_key ILIKE '%' || @search || '%'
       OR entity_type ILIKE '%' || @search || '%'
       OR extracted::text ILIKE '%' || @search || '%');

-- name: ListStaleIntelRecords :many
SELECT * FROM intel_records WHERE freshness_score <= 40 AND status = 'published' LIMIT $1;

-- name: CountPromotedIntelRecordsBySource :one
-- Returns the number of intel_records for a source that have been promoted
-- (promoted_record_id IS NOT NULL). Used by the source-delete handler to
-- enforce the spec §650 / route-catalog §2225 invariant "promoted records
-- preserved" — deleting a source with promoted records would CASCADE-wipe
-- the promoted scholarships + their contents guides (BUG-004 fix).
SELECT count(*) FROM intel_records
WHERE source_id = $1 AND promoted_record_id IS NOT NULL;

-- name: CreateIntelReviewLog :exec
INSERT INTO intel_review_log (intel_record_id, actor_user_id, action, field_changes, note) VALUES ($1, $2, $3, $4, $5)
;

-- name: ListIntelReviewLog :many
SELECT * FROM intel_review_log WHERE intel_record_id = $1 ORDER BY created_at DESC;

-- name: DeleteIntelReviewLogByActor :exec
DELETE FROM intel_review_log WHERE actor_user_id = $1;

-- name: GetIntelPersonaUserID :one
-- Resolves the backing user_id for an AI persona attached to an intel source.
-- Used by promoters to stamp contents.author_id with the persona's bot user.
SELECT ap.user_id
FROM ai_personas ap
WHERE ap.id = $1;

-- name: ListProposedIntelSources :many
-- Returns sources with status='proposed' for admin review.
SELECT * FROM intel_sources
WHERE status = 'proposed'
ORDER BY proposed_at DESC NULLS LAST
LIMIT $1 OFFSET $2;

-- name: ApproveProposedIntelSource :one
-- Approves a proposed source: status='active', enabled=true, record approver.
UPDATE intel_sources
SET status = 'active',
 enabled = true,
 approved_by = $2,
 approved_at = now(),
 updated_at = now()
WHERE id = $1 AND status = 'proposed'
RETURNING *;

-- name: ListPublicSourcesPopular :many
SELECT * FROM intel_sources
WHERE enabled = true
 AND status = 'active'
 AND ($1::text IS NULL OR $1 = '' OR category = $1)
 AND ($2::text IS NULL OR $2 = '' OR source_kind = $2)
 AND ($3::text IS NULL OR $3 = '' OR name ILIKE '%' || $3 || '%' OR description ILIKE '%' || $3 || '%' OR domain ILIKE '%' || $3 || '%')
ORDER BY follower_count DESC, name ASC
LIMIT $4 OFFSET $5;

-- name: ListPublicSourcesRecent :many
SELECT * FROM intel_sources
WHERE enabled = true
 AND status = 'active'
 AND ($1::text IS NULL OR $1 = '' OR category = $1)
 AND ($2::text IS NULL OR $2 = '' OR source_kind = $2)
 AND ($3::text IS NULL OR $3 = '' OR name ILIKE '%' || $3 || '%' OR description ILIKE '%' || $3 || '%' OR domain ILIKE '%' || $3 || '%')
ORDER BY created_at DESC, name ASC
LIMIT $4 OFFSET $5;

-- name: ListPublicSourcesTrending :many
SELECT * FROM intel_sources
WHERE enabled = true
 AND status = 'active'
 AND ($1::text IS NULL OR $1 = '' OR category = $1)
 AND ($2::text IS NULL OR $2 = '' OR source_kind = $2)
 AND ($3::text IS NULL OR $3 = '' OR name ILIKE '%' || $3 || '%' OR description ILIKE '%' || $3 || '%' OR domain ILIKE '%' || $3 || '%')
 AND created_at > now() - interval '30 days'
ORDER BY follower_count DESC, post_count DESC
LIMIT $4 OFFSET $5;

-- name: CountPublicSources :one
SELECT COUNT(*) FROM intel_sources
WHERE enabled = true
 AND status = 'active'
 AND ($1::text IS NULL OR $1 = '' OR category = $1)
 AND ($2::text IS NULL OR $2 = '' OR source_kind = $2)
 AND ($3::text IS NULL OR $3 = '' OR name ILIKE '%' || $3 || '%' OR description ILIKE '%' || $3 || '%' OR domain ILIKE '%' || $3 || '%');

-- name: IncrementSourcePostCount :exec
UPDATE intel_sources
SET post_count = post_count + 1
WHERE id = $1;

-- name: DecrementSourcePostCount :exec
UPDATE intel_sources
SET post_count = GREATEST(0, post_count - 1)
WHERE id = $1;

-- name: GetIntelSourceBySlugWithLinkedEntity :one
SELECT s.*,
 sp.slug AS space_slug, sp.name AS space_name,
 o.slug AS org_slug, o.name AS org_name,
 u.username AS user_slug, COALESCE(up.display_name, u.username, '') AS user_name
FROM intel_sources s
LEFT JOIN spaces sp ON sp.source_id = s.id
LEFT JOIN organizations o ON o.source_id = s.id
LEFT JOIN users u ON u.source_id = s.id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE s.slug = $1 AND s.enabled = true;

-- name: ListIntelRecordsBySource :many
-- Published records for a source, sorted at the DB level by the requested
-- engagement mode. Scores come from content_scores when present, falling back
-- to the raw contents columns; records without promoted content sort last.
-- Ref: docs/superpowers/specs/2026-09-11-sources-directory-design.md
SELECT ir.* FROM intel_records ir
LEFT JOIN contents c ON c.id = ir.promoted_record_id AND c.deleted_at IS NULL
LEFT JOIN content_scores cs ON cs.content_id = c.id
WHERE ir.source_id = $1
 AND ir.status = 'published'
 AND ($2::text IS NULL OR $2 = '' OR ir.entity_type = $2)
ORDER BY
 CASE
 WHEN $5 = 'popular' THEN COALESCE(cs.view_count, c.view_count, 0)
 WHEN $5 = 'discussed' THEN COALESCE(cs.comment_count, c.comment_count, 0)
 WHEN $5 = 'upvoted' THEN COALESCE(cs.vote_score, 0)
 END DESC NULLS LAST,
 ir.extracted_at DESC
LIMIT $3 OFFSET $4;

-- name: CountIntelRecordsBySource :one
SELECT COUNT(*) FROM intel_records
WHERE source_id = $1
 AND status = 'published'
 AND ($2::text IS NULL OR $2 = '' OR entity_type = $2);

-- name: ListDistinctIntelRecordEntityTypesBySource :many
-- Distinct published entity types in a source's feed — the public detail
-- page scopes its entity-type filter pills to types that actually have
-- items instead of offering every global type.
SELECT DISTINCT entity_type FROM intel_records
WHERE source_id = $1
 AND status = 'published'
ORDER BY entity_type;

-- name: ListSourcesByRecordIDs :many
-- Returns a map-friendly list of sources for a batch of intel record IDs.
-- Used by scholarship, job, and remittance responses to attach attribution.
SELECT
 ir.id AS record_id,
 s.id AS source_id,
 s.name,
 s.slug,
 s.logo_url,
 s.source_url,
 s.trust_level
FROM intel_records ir
JOIN intel_sources s ON s.id = ir.source_id
WHERE ir.id = ANY($1::uuid[]);

-- name: CreateSourceBlock :one
INSERT INTO user_source_blocks (blocker_id, source_id) VALUES ($1, $2)

ON CONFLICT (blocker_id, source_id) DO NOTHING
RETURNING id, created_at;

-- name: DeleteSourceBlock :exec
DELETE FROM user_source_blocks
WHERE blocker_id = $1 AND source_id = $2;

-- name: ListBlockedSourceIDs :many
SELECT source_id FROM user_source_blocks
WHERE blocker_id = $1;

-- name: IsSourceBlocked :one
SELECT EXISTS(
 SELECT 1 FROM user_source_blocks
 WHERE blocker_id = $1 AND source_id = $2
) AS blocked;

-- name: ListSourceCollections :many
SELECT * FROM source_collections

ORDER BY slug;

-- name: ListSourceCollectionItems :many
SELECT
 i.position,
 s.*
FROM source_collection_items i
JOIN intel_sources s ON s.id = i.source_id
WHERE i.collection_id = $1
ORDER BY i.position, i.source_id;

-- name: UpdateIntelSourceMetadata :exec
UPDATE intel_sources
SET description = COALESCE(sqlc.narg(description), description),
 logo_url = COALESCE(sqlc.narg(logo_url), logo_url),
 keywords = COALESCE(sqlc.narg(keywords), keywords),
 updated_at = now()
WHERE id = $1;

-- name: CountIntelRecordsByStatus :one
-- Total count for the status+entity_type filtered review/records lists so
-- the moderation UIs can compute real pagination (meta.count was the page
-- size, which pinned the admin review queue to one page).
SELECT COUNT(*) FROM intel_records WHERE entity_type = $1 AND status = $2;

-- name: CountIntelRecordsByStatusAll :one
SELECT COUNT(*) FROM intel_records WHERE status = $1;

-- name: IntelSourceDomainExists :one
-- Case-insensitive host check for outbound-link source discovery
-- (intel-channels Phase 4): a candidate domain already registered under
-- ANY status (active/proposed/blocked) is not re-proposed.
SELECT EXISTS(
    SELECT 1 FROM intel_sources WHERE lower(domain) = lower($1)
) AS exists;
