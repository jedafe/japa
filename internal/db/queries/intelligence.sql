-- internal/db/queries/intelligence.sql
-- sqlc queries for the intelligence spine tables.
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md

-- name: CreateIntelSource :one
INSERT INTO intel_sources (name, domain, source_url, category, source_type, trust_level, crawl_method, crawl_frequency, crawl_frequency_mins, robots_policy, source_license, attribution_required, usage_restriction, provider_config, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING *;

-- name: GetIntelSource :one
SELECT * FROM intel_sources WHERE id = $1;

-- name: GetIntelSourceByURL :one
SELECT * FROM intel_sources WHERE source_url = $1;

-- name: ListIntelSources :many
SELECT * FROM intel_sources ORDER BY created_at DESC LIMIT $1 OFFSET $2;

-- name: ListIntelSourcesByCategory :many
SELECT * FROM intel_sources WHERE category = $1 AND enabled = true ORDER BY trust_level, name;

-- name: ListDueIntelSources :many
SELECT * FROM intel_sources
WHERE enabled = true
  AND (last_crawled_at IS NULL OR now() - last_crawled_at >= (crawl_frequency_mins || ' minutes')::interval)
ORDER BY last_crawled_at NULLS FIRST
LIMIT $1;

-- name: UpdateIntelSourceCrawlStatus :exec
UPDATE intel_sources
SET last_crawled_at = now(),
    last_success_at = CASE WHEN $2 THEN now() ELSE last_success_at END,
    last_error_at = CASE WHEN $2 THEN last_error_at ELSE now() END,
    last_error_message = $3,
    consecutive_failures = CASE WHEN $2 THEN 0 ELSE consecutive_failures + 1 END,
    updated_at = now()
WHERE id = $1;

-- name: UpdateIntelSourceLastChanged :exec
UPDATE intel_sources SET last_changed_at = now(), updated_at = now() WHERE id = $1;

-- name: UpdateIntelSourceEnabled :exec
UPDATE intel_sources SET enabled = $2, updated_at = now() WHERE id = $1;

-- name: UpdateIntelSourceRobotsPolicy :exec
UPDATE intel_sources SET robots_policy = $2, updated_at = now() WHERE id = $1;

-- name: DeleteIntelSource :exec
DELETE FROM intel_sources WHERE id = $1;

-- name: CreateIntelRawDocument :one
INSERT INTO intel_raw_documents (source_id, url, http_status, http_headers, content_type, body, content_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetLatestIntelRawDocHash :one
SELECT content_hash FROM intel_raw_documents WHERE source_id = $1 ORDER BY retrieved_at DESC LIMIT 1;

-- name: CreateIntelRecord :one
INSERT INTO intel_records (source_id, raw_document_id, entity_type, external_key, extracted, confidence, evidence, confidence_overall, status, geo_space_ids)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'extracted', $9)
RETURNING *;

-- name: GetIntelRecord :one
SELECT * FROM intel_records WHERE id = $1;

-- name: GetIntelRecordByExternalKey :one
SELECT * FROM intel_records WHERE source_id = $1 AND external_key = $2;

-- name: ListIntelRecordsByStatus :many
SELECT * FROM intel_records WHERE entity_type = $1 AND status = $2 ORDER BY extracted_at DESC LIMIT $3 OFFSET $4;

-- name: UpdateIntelRecordStatus :exec
UPDATE intel_records SET status = $2, last_verified_at = now() WHERE id = $1;

-- name: UpdateIntelRecordFreshness :exec
UPDATE intel_records SET freshness_score = $2, status = CASE WHEN $2 <= 40 THEN 'stale' ELSE status END WHERE id = $1;

-- name: UpdateIntelRecordPromoted :exec
UPDATE intel_records SET promoted_record_id = $2, status = 'published' WHERE id = $1;

-- name: ListStaleIntelRecords :many
SELECT * FROM intel_records WHERE freshness_score <= 40 AND status = 'published' LIMIT $1;

-- name: CreateIntelReviewLog :exec
INSERT INTO intel_review_log (intel_record_id, actor_user_id, action, field_changes, note)
VALUES ($1, $2, $3, $4, $5);

-- name: ListIntelReviewLog :many
SELECT * FROM intel_review_log WHERE intel_record_id = $1 ORDER BY created_at DESC;
