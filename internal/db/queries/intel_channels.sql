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

-- name: ClaimIntelInboundEvent :execrows
-- Atomic mutex for the processing task: transitions received → processing
-- and reports whether the claim won. Only one concurrent worker can flip
-- the row, so duplicate task enqueues (or an overlapping retry while the
-- first run is mid-flight) cannot double-materialize a delivery.
UPDATE intel_inbound_events SET status = 'processing', claimed_at = now()
WHERE id = $1 AND status = 'received';

-- name: MarkIntelInboundEvent :exec
-- Terminal transition from 'received' (inbound handler paths that never
-- claimed) or 'processing' (the claiming worker). Terminal states
-- (processed/failed/ignored) are immutable so a late duplicate task run
-- can never flip a finished delivery back or forward.
UPDATE intel_inbound_events SET status = $2, processed_at = now()
WHERE id = $1 AND status IN ('received','processing');

-- name: ReleaseIntelInboundEvent :exec
-- Returns a claimed event to 'received' when the processing run fails on
-- a transient error and the task is being retried — the next attempt must
-- be able to win the claim CAS again (the task's early-out skips any
-- non-'received' status). Clears claimed_at so the stale-claim window in
-- RedriveIntelInboundEvent starts fresh on the next claim. Only
-- 'processing' rows transition — a concurrently-marked terminal event is
-- left alone.
UPDATE intel_inbound_events SET status = 'received', claimed_at = NULL
WHERE id = $1 AND status = 'processing';

-- name: RedriveIntelInboundEvent :execrows
-- Admin re-drive: reset a failed/rejected/stuck-processing event back to
-- 'received' so the follow-up task re-processes it. A 'processing' event
-- is only redrivable once its claim is older than $2 seconds — anything
-- fresher is a live worker mid-flight and must not be hijacked (that
-- would double-materialize the delivery). A 'received' event is only
-- redrivable past the same window: a fresh one is queued-but-not-yet-run,
-- while a stale one means the enqueued task was lost (queue flush) — the
-- claim CAS makes a late-running duplicate a no-op anyway. 0 rows =
-- terminal-processed, actively claimed, or still waiting.
UPDATE intel_inbound_events SET status = 'received', processed_at = NULL, claimed_at = NULL
WHERE id = $1 AND (
  status IN ('failed','ignored')
  OR (status = 'processing'
      AND (claimed_at IS NULL OR claimed_at < now() - make_interval(secs => $2)))
  OR (status = 'received' AND received_at < now() - make_interval(secs => $2))
);

-- name: GetIntelInboundEvent :one
SELECT * FROM intel_inbound_events WHERE id = $1;

-- name: PurgeIntelInboundEvents :execrows
-- Retention sweep (intel:purge_inbound_events): deletes every event whose
-- received_at is older than $1 days — raw payloads carry sender PII, so
-- retention is bounded by intel.inbound.retention_days. Any status is
-- covered: terminal rows are history, and a received/processing row that
-- old is a lost delivery whose redrive value has expired. $1 <= 0 deletes
-- nothing (the cataloged "keep forever" sentinel can't be bypassed by a
-- stray call). Returns the deleted row count.
DELETE FROM intel_inbound_events
WHERE sqlc.arg(retention_days)::int > 0
  AND received_at < now() - make_interval(days => sqlc.arg(retention_days)::int);

-- name: NoteIntelChannelError :exec
-- Surface a non-crawl channel error (e.g. a hub denying a WebSub
-- subscription) on the channel row WITHOUT touching consecutive_failures,
-- last_crawled_at, or last_success_at — it is a flag, not a crawl result.
UPDATE intel_source_channels SET last_error_message = $2, last_error_at = now()
WHERE id = $1;

-- name: GetIntelEmailChannelByAddress :one
-- Inbound-email routing (Phase 3): resolve the delivery's recipient to the
-- enabled email channel carrying it in provider_config.inbound_address.
SELECT * FROM intel_source_channels
WHERE crawl_method = 'email' AND enabled
  AND lower(provider_config->>'inbound_address') = lower($1)
ORDER BY created_at ASC
LIMIT 1;

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
-- source must not cross-contaminate each other's hash comparison. Derived
-- (manifest child) docs are excluded — a sitemap/wayback manifest must
-- compare against the previous manifest, not a child's page hash, or every
-- crawl after fan-out would read as "changed" (Phase 2).
SELECT content_hash FROM intel_raw_documents WHERE channel_id = $1
  AND parent_document_id IS NULL
ORDER BY retrieved_at DESC LIMIT 1;

-- name: GetLatestIntelManifestDocHashByChannel :one
-- Manifest-channel prior hash (Phase 2): the newest MANIFEST doc only —
-- the content_type marker keeps a fallback-ladder document (e.g. a static
-- fetch that covered a failed sitemap crawl) from becoming the manifest's
-- diff baseline and reading "changed" on every subsequent crawl.
SELECT content_hash FROM intel_raw_documents WHERE channel_id = $1
  AND parent_document_id IS NULL
  AND content_type = 'application/x-intel-manifest'
ORDER BY retrieved_at DESC LIMIT 1;

-- name: MarkIntelChannelChanged :exec
-- Channel-scoped counterpart of UpdateIntelSourceLastChanged — stamps
-- last_changed_at when detect_changes sees a content change.
UPDATE intel_source_channels SET last_changed_at = now(), updated_at = now()
WHERE id = $1;
