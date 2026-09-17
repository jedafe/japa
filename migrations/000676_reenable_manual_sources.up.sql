BEGIN;

-- 000676_reenable_manual_sources.up.sql
-- §106 — ListDueIntelSources scheduled crawl_method='manual' sources (they
-- are never auto-crawled), and HandleIntelFetchSource failed on the unknown
-- method. Five consecutive failures tripped the auto-disable circuit-breaker,
-- hiding the rows from the public /sources directory (404 source_not_found).
-- The scheduler query now excludes manual sources and the fetcher skips them
-- gracefully; this re-enables rows disabled specifically by that bug
-- (identified by the error message it recorded).
--
-- Ref: japaconnect-engineering/sections/106-video-content-and-import.md

UPDATE intel_sources
SET enabled = true,
    consecutive_failures = 0,
    last_error_message = NULL,
    updated_at = now()
WHERE crawl_method = 'manual'
  AND enabled = false
  AND last_error_message LIKE '%unknown crawl_method%';

COMMIT;
