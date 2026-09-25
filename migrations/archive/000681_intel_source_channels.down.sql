-- 000681_intel_source_channels.down.sql
-- Reverts 000681: drops intel_inbound_events + intel_source_channels,
-- intel_raw_documents.channel_id and the new intel_sources columns, and
-- restores the original CHECK constraint sets.
-- Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §1

BEGIN;

ALTER TABLE intel_raw_documents DROP COLUMN IF EXISTS channel_id;

DROP TABLE IF EXISTS intel_inbound_events;
DROP TABLE IF EXISTS intel_source_channels;

-- Fold categories[1] back into category where category was left blank.
UPDATE intel_sources
SET category = categories[1]
WHERE btrim(category) = '' AND cardinality(categories) > 0;

ALTER TABLE intel_sources
  DROP COLUMN IF EXISTS categories,
  DROP COLUMN IF EXISTS probe_report,
  DROP COLUMN IF EXISTS detected_methods,
  DROP COLUMN IF EXISTS last_probed_at;

-- Re-state rows using the new source_kind values before restoring the
-- original CHECK set.
UPDATE intel_sources SET source_kind = 'publisher'
WHERE source_kind IN ('channel_profile','institution_site','personal_blog',
                      'job_board','gov_portal','community');

ALTER TABLE intel_sources DROP CONSTRAINT IF EXISTS intel_sources_source_kind_check;
ALTER TABLE intel_sources ADD CONSTRAINT intel_sources_source_kind_check
  CHECK (source_kind IN ('publisher','institution','individual','aggregator'));

-- Re-state off_topic rows before restoring the original status CHECK set.
UPDATE intel_records SET status = 'rejected' WHERE status = 'off_topic';

ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_status_check;
ALTER TABLE intel_records ADD CONSTRAINT intel_records_status_check
  CHECK (status IN ('extracted','deduped','auto_published','review_required',
    'approved','rejected','published','stale'));

COMMIT;
