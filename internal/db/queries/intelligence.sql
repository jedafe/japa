-- internal/db/queries/intelligence.sql
-- sqlc queries for the intelligence spine tables.
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md
--
-- All queries are tenant-scoped (ADR-112 Amendment 2). Every query filters
-- by tenant_id to guarantee data isolation across tenants.

-- name: CreateIntelSource :one
INSERT INTO intel_sources (tenant_id, name, slug, domain, source_url, description, category, source_type, source_kind, trust_level, crawl_method, crawl_frequency, crawl_frequency_mins, robots_policy, source_license, attribution_required, usage_restriction, provider_config, logo_url, enabled, status, proposed_by, proposed_at, approved_by, approved_at, persona_id)
VALUES ($1, $2, sqlc.narg(slug), $3, $4, sqlc.narg(description), $5, $6, COALESCE(sqlc.narg(source_kind), 'publisher'), $7, $8, $9, $10, $11, $12, $13, $14, $15, sqlc.narg(logo_url), $16,
        COALESCE(sqlc.narg(status), 'active'),
        sqlc.narg(proposed_by),
        sqlc.narg(proposed_at),
        sqlc.narg(approved_by),
        sqlc.narg(approved_at),
        sqlc.narg(persona_id))
RETURNING *;

-- name: GetIntelSource :one
SELECT * FROM intel_sources WHERE tenant_id = $1 AND id = $2;

-- name: GetIntelSourceByURL :one
SELECT * FROM intel_sources WHERE tenant_id = $1 AND source_url = $2;

-- name: GetIntelSourceBySlug :one
SELECT * FROM intel_sources WHERE tenant_id = $1 AND slug = $2;

-- name: ListIntelSources :many
SELECT * FROM intel_sources WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: ListIntelSourcesByCategory :many
SELECT * FROM intel_sources WHERE tenant_id = $1 AND category = $2 AND enabled = true AND status = 'active' ORDER BY trust_level, name;

-- name: ListDueIntelSources :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND (last_crawled_at IS NULL OR now() - last_crawled_at >= (crawl_frequency_mins || ' minutes')::interval)
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
WHERE tenant_id = $1 AND id = $2;

-- name: UpdateIntelSourceLastChanged :exec
UPDATE intel_sources SET last_changed_at = now(), updated_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: UpdateIntelSourceEnabled :exec
UPDATE intel_sources SET enabled = $3, updated_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: UpdateIntelSourceRobotsPolicy :exec
UPDATE intel_sources SET robots_policy = $3, updated_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: DeleteIntelSource :exec
DELETE FROM intel_sources WHERE tenant_id = $1 AND id = $2;

-- name: UpdateIntelSource :one
UPDATE intel_sources
SET name = $3,
    slug = COALESCE(sqlc.narg(slug), slug),
    domain = $4,
    source_url = $5,
    description = COALESCE(sqlc.narg(description), description),
    category = $6,
    source_type = $7,
    source_kind = COALESCE(sqlc.narg(source_kind), source_kind),
    trust_level = $8,
    crawl_method = $9,
    crawl_frequency = $10,
    crawl_frequency_mins = $11,
    source_license = $12,
    attribution_required = $13,
    usage_restriction = $14,
    logo_url = COALESCE(sqlc.narg(logo_url), logo_url),
    enabled = $15,
    status = COALESCE(sqlc.narg(status), status),
    proposed_by = COALESCE(sqlc.narg(proposed_by), proposed_by),
    proposed_at = COALESCE(sqlc.narg(proposed_at), proposed_at),
    approved_by = COALESCE(sqlc.narg(approved_by), approved_by),
    approved_at = COALESCE(sqlc.narg(approved_at), approved_at),
    persona_id = COALESCE(sqlc.narg(persona_id), persona_id),
    updated_at = now()
WHERE tenant_id = $1 AND id = $2
RETURNING *;

-- name: CreateIntelRawDocument :one
INSERT INTO intel_raw_documents (tenant_id, source_id, url, http_status, http_headers, content_type, body, content_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetIntelRawDocument :one
SELECT * FROM intel_raw_documents WHERE tenant_id = $1 AND id = $2;

-- name: GetLatestIntelRawDocHash :one
SELECT content_hash FROM intel_raw_documents WHERE tenant_id = $1 AND source_id = $2 ORDER BY retrieved_at DESC LIMIT 1;

-- name: CreateIntelRecord :one
INSERT INTO intel_records (tenant_id, source_id, raw_document_id, entity_type, external_key, extracted, confidence, evidence, confidence_overall, status, geo_space_ids)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'extracted', $10)
RETURNING *;

-- name: GetIntelRecord :one
SELECT * FROM intel_records WHERE tenant_id = $1 AND id = $2;

-- name: GetIntelRecordByExternalKey :one
SELECT * FROM intel_records WHERE tenant_id = $1 AND source_id = $2 AND external_key = $3;

-- name: ListIntelRecordsByExternalKey :many
SELECT * FROM intel_records WHERE tenant_id = $1 AND external_key = $2 ORDER BY extracted_at DESC;

-- name: ListIntelRecordsByStatus :many
SELECT * FROM intel_records WHERE tenant_id = $1 AND entity_type = $2 AND status = $3 ORDER BY extracted_at DESC LIMIT $4 OFFSET $5;

-- name: ListIntelRecordsByStatusAll :many
-- Lists intel records by status across ALL entity types (no entity_type
-- filter). Used by the review-queue handler when no entity_type filter is
-- supplied so job/visa/grant records awaiting review are not invisible
-- (BUG-001 fix — the prior handler hardcoded entity_type='scholarship').
SELECT * FROM intel_records WHERE tenant_id = $1 AND status = $2 ORDER BY extracted_at DESC LIMIT $3 OFFSET $4;

-- name: UpdateIntelRecordStatus :exec
UPDATE intel_records SET status = $3, last_verified_at = now() WHERE tenant_id = $1 AND id = $2;

-- name: UpdateIntelRecordFreshness :exec
UPDATE intel_records SET freshness_score = $3, status = CASE WHEN $3 <= 40 THEN 'stale' ELSE status END WHERE tenant_id = $1 AND id = $2;

-- name: UpdateIntelRecordPromoted :exec
UPDATE intel_records SET promoted_record_id = $3, status = 'published' WHERE tenant_id = $1 AND id = $2;

-- name: ListStaleIntelRecords :many
SELECT * FROM intel_records WHERE tenant_id = $1 AND freshness_score <= 40 AND status = 'published' LIMIT $2;

-- name: CountPromotedIntelRecordsBySource :one
-- Returns the number of intel_records for a source that have been promoted
-- (promoted_record_id IS NOT NULL). Used by the source-delete handler to
-- enforce the spec §650 / route-catalog §2225 invariant "promoted records
-- preserved" — deleting a source with promoted records would CASCADE-wipe
-- the promoted scholarships + their contents guides (BUG-004 fix).
SELECT count(*) FROM intel_records
WHERE tenant_id = $1 AND source_id = $2 AND promoted_record_id IS NOT NULL;

-- name: CreateIntelReviewLog :exec
INSERT INTO intel_review_log (tenant_id, intel_record_id, actor_user_id, action, field_changes, note)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListIntelReviewLog :many
SELECT * FROM intel_review_log WHERE tenant_id = $1 AND intel_record_id = $2 ORDER BY created_at DESC;

-- name: DeleteIntelReviewLogByActor :exec
DELETE FROM intel_review_log WHERE tenant_id = $1 AND actor_user_id = $2;

-- name: GetIntelPersonaUserID :one
-- Resolves the backing user_id for an AI persona attached to an intel source.
-- Used by promoters to stamp contents.author_id with the persona's bot user.
SELECT ap.user_id
FROM ai_personas ap
WHERE ap.tenant_id = $1 AND ap.id = $2;

-- name: ListProposedIntelSources :many
-- Returns sources with status='proposed' for admin review.
SELECT * FROM intel_sources
WHERE tenant_id = $1 AND status = 'proposed'
ORDER BY proposed_at DESC NULLS LAST
LIMIT $2 OFFSET $3;

-- name: ApproveProposedIntelSource :one
-- Approves a proposed source: status='active', enabled=true, record approver.
UPDATE intel_sources
SET status = 'active',
    enabled = true,
    approved_by = $3,
    approved_at = now(),
    updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND status = 'proposed'
RETURNING *;

-- name: ListPublicSourcesPopular :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR $2 = '' OR category = $2)
  AND ($3::text IS NULL OR $3 = '' OR source_kind = $3)
  AND ($4::text IS NULL OR $4 = '' OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%')
ORDER BY follower_count DESC, name ASC
LIMIT $5 OFFSET $6;

-- name: ListPublicSourcesRecent :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR $2 = '' OR category = $2)
  AND ($3::text IS NULL OR $3 = '' OR source_kind = $3)
  AND ($4::text IS NULL OR $4 = '' OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%')
ORDER BY created_at DESC, name ASC
LIMIT $5 OFFSET $6;

-- name: ListPublicSourcesTrending :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR $2 = '' OR category = $2)
  AND ($3::text IS NULL OR $3 = '' OR source_kind = $3)
  AND ($4::text IS NULL OR $4 = '' OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%')
  AND created_at > now() - interval '30 days'
ORDER BY follower_count DESC, post_count DESC
LIMIT $5 OFFSET $6;

-- name: CountPublicSources :one
SELECT COUNT(*) FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR $2 = '' OR category = $2)
  AND ($3::text IS NULL OR $3 = '' OR source_kind = $3)
  AND ($4::text IS NULL OR $4 = '' OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%');

-- name: IncrementSourcePostCount :exec
UPDATE intel_sources
SET post_count = post_count + 1
WHERE tenant_id = $1 AND id = $2;

-- name: DecrementSourcePostCount :exec
UPDATE intel_sources
SET post_count = GREATEST(0, post_count - 1)
WHERE tenant_id = $1 AND id = $2;

-- name: GetIntelSourceBySlugWithLinkedEntity :one
SELECT s.*,
       sp.slug AS space_slug, sp.name AS space_name,
       o.slug AS org_slug, o.name AS org_name,
       u.username AS user_slug, COALESCE(up.display_name, u.username) AS user_name
FROM intel_sources s
LEFT JOIN spaces sp ON sp.source_id = s.id
LEFT JOIN organizations o ON o.source_id = s.id
LEFT JOIN users u ON u.source_id = s.id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE s.tenant_id = $1 AND s.slug = $2 AND s.enabled = true;

-- name: ListIntelRecordsBySource :many
SELECT * FROM intel_records
WHERE tenant_id = $1
  AND source_id = $2
  AND status = 'published'
  AND ($3::text IS NULL OR $3 = '' OR entity_type = $3)
ORDER BY extracted_at DESC
LIMIT $4 OFFSET $5;
