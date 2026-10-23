-- 000924_media_missing_at.up.sql
-- §34 media: a row can outlive its storage object (abandoned two-step
-- uploads, object stores that were never synced to the environment,
-- objects removed out-of-band). Every such row resolves to the
-- placeholder SVG, so media listings fill with identical placeholder
-- tiles and drown out usable media.
--
-- missing_at marks rows whose object was confirmed absent. The serve
-- path sets it lazily when a lookup misses, upload/confirm paths clear
-- it when an object lands, and POST /admin/media/reconcile-missing
-- backfills it in bulk. ListMediaFiltered/CountMediaFiltered exclude
-- flagged rows by default (?missing=only|all opt back in).

ALTER TABLE media ADD COLUMN missing_at timestamptz;

-- Rows are filtered with `missing_at IS NULL` on every list; a partial
-- index on the rare flagged rows keeps ?missing=only lookups cheap.
CREATE INDEX media_missing_at_idx ON media (missing_at) WHERE missing_at IS NOT NULL;
