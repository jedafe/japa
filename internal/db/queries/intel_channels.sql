-- intel_channels.sql — queries for intel_source_channels and
-- intel_inbound_events (migration 000681).
-- Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §1
--
-- NOTE: post-de-tenant schema (000668) — no tenant_id columns, so these
-- queries are not tenant-scoped.

-- name: CreateIntelSourceChannel :one
INSERT INTO intel_source_channels
  (source_id, label, entry_url, crawl_method, entity_types,
   pipeline, provider_config, crawl_frequency_mins, enabled, origin,
   relevance_mode, relevance_topics, relevance_url_patterns)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING *;

-- name: GetIntelSourceChannel :one
SELECT * FROM intel_source_channels WHERE id = $1;

-- name: ListIntelSourceChannels :many
SELECT * FROM intel_source_channels
WHERE source_id = $1 ORDER BY created_at;

-- name: ListDueIntelChannels :many
SELECT c.* FROM intel_source_channels c
JOIN intel_sources s ON s.id = c.source_id
WHERE c.enabled AND s.enabled AND s.status = 'active'
  AND c.crawl_method NOT IN ('manual','email','webhook','oneshot','xhr_sniff')
  AND (c.last_crawled_at IS NULL
       OR c.last_crawled_at < now() - make_interval(mins => c.crawl_frequency_mins))
ORDER BY c.last_crawled_at ASC NULLS FIRST LIMIT $1;

-- name: UpdateIntelSourceChannel :one
-- NOTE: array columns are NOT here — COALESCE can't distinguish "don't
-- change" from "set empty". Arrays go through UpdateIntelChannelRelevance
-- (direct assignment, empty allowed) and entity_types through
-- UpdateIntelChannelEntityTypes.
UPDATE intel_source_channels SET
  label = COALESCE(sqlc.narg(label), label),
  entry_url = COALESCE(sqlc.narg(entry_url), entry_url),
  crawl_method = COALESCE(sqlc.narg(crawl_method), crawl_method),
  pipeline = COALESCE(sqlc.narg(pipeline), pipeline),
  provider_config = COALESCE(sqlc.narg(provider_config), provider_config),
  crawl_frequency_mins = COALESCE(sqlc.narg(crawl_frequency_mins), crawl_frequency_mins),
  enabled = COALESCE(sqlc.narg(enabled), enabled),
  updated_at = now()
WHERE id = $1 RETURNING *;

-- name: UpdateIntelChannelEntityTypes :one
UPDATE intel_source_channels SET entity_types = $2, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: UpdateIntelChannelRelevance :one
UPDATE intel_source_channels SET
  relevance_mode = $2, relevance_topics = $3, relevance_url_patterns = $4,
  updated_at = now()
WHERE id = $1 RETURNING *;

-- name: GetIntelSourceDefaultChannel :one
-- Legacy intel:fetch_source path resolves the source's oldest enabled
-- channel (the backfilled 'admin' default) to delegate to fetch_channel.
SELECT * FROM intel_source_channels
WHERE source_id = $1 AND enabled
ORDER BY created_at ASC LIMIT 1;

-- name: DeleteIntelSourceChannel :exec
DELETE FROM intel_source_channels WHERE id = $1;

-- name: UpdateIntelChannelCrawlStatus :exec
UPDATE intel_source_channels SET
  last_crawled_at = now(),
  last_success_at = CASE WHEN sqlc.arg(success)::boolean THEN now() ELSE last_success_at END,
  last_error_at   = CASE WHEN sqlc.arg(success) THEN last_error_at ELSE now() END,
  last_error_message = sqlc.narg(last_error_message),
  consecutive_failures = CASE WHEN sqlc.arg(success) THEN 0 ELSE consecutive_failures + 1 END
WHERE id = sqlc.arg(id);

-- name: UpdateIntelChannelEnabled :exec
UPDATE intel_source_channels SET enabled = $2, updated_at = now()
WHERE id = $1;

-- name: CreateIntelInboundEvent :one
INSERT INTO intel_inbound_events (channel_id, payload, headers, content_type, content_hash)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: MarkIntelInboundEvent :exec
UPDATE intel_inbound_events SET status = $2, processed_at = now()
WHERE id = $1;

-- name: UpdateIntelSourceProbe :exec
UPDATE intel_sources SET probe_report = $2, detected_methods = $3, last_probed_at = now()
WHERE id = $1;

-- name: UpdateIntelSourceCategories :one
-- Direct assignment like UpdateIntelChannelEntityTypes — the admin source
-- PATCH detects JSON key presence, so a present-but-empty array is an
-- explicit clear to '{}' rather than a COALESCE no-op.
UPDATE intel_sources SET categories = $2, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: GetLatestIntelRawDocHashByChannel :one
-- Channel-scoped prior hash for detect_changes: two channels of the same
-- source must not cross-contaminate each other's hash comparison.
SELECT content_hash FROM intel_raw_documents WHERE channel_id = $1
ORDER BY retrieved_at DESC LIMIT 1;

-- name: MarkIntelChannelChanged :exec
-- Channel-scoped counterpart of UpdateIntelSourceLastChanged — stamps
-- last_changed_at when detect_changes sees a content change.
UPDATE intel_source_channels SET last_changed_at = now(), updated_at = now()
WHERE id = $1;
