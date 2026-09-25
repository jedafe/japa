-- 000681_intel_source_channels.up.sql
-- Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §1
--
-- NOTE: this database is post-de-tenant (000668 dropped the tenants table and
-- every tenant_id column), so unlike the spec draft these tables carry no
-- tenant_id and the channel queries are not tenant-scoped.

BEGIN;

ALTER TABLE intel_sources
  ADD COLUMN IF NOT EXISTS categories        text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS probe_report      jsonb  NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS detected_methods  text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS last_probed_at    timestamptz;

ALTER TABLE intel_sources DROP CONSTRAINT IF EXISTS intel_sources_source_kind_check;
ALTER TABLE intel_sources ADD CONSTRAINT intel_sources_source_kind_check
  CHECK (source_kind IN ('publisher','institution','individual','aggregator',
    'channel_profile','institution_site','personal_blog',
    'job_board','gov_portal','community'));

ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_status_check;
ALTER TABLE intel_records ADD CONSTRAINT intel_records_status_check
  CHECK (status IN ('extracted','deduped','auto_published','review_required',
    'approved','rejected','published','stale','off_topic'));

CREATE TABLE intel_source_channels (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id              uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
  label                  text NOT NULL,
  entry_url              text NOT NULL,
  crawl_method           text NOT NULL,
  entity_types           text[] NOT NULL DEFAULT '{}',
  pipeline               text NOT NULL DEFAULT 'extract'
                           CHECK (pipeline IN ('extract','curate','hybrid')),
  provider_config        jsonb NOT NULL DEFAULT '{}',
  crawl_frequency_mins   int NOT NULL DEFAULT 1440,
  enabled                boolean NOT NULL DEFAULT true,
  origin                 text NOT NULL DEFAULT 'probe'
                           CHECK (origin IN ('probe','admin','user')),
  relevance_mode         text NOT NULL DEFAULT 'filtered'
                           CHECK (relevance_mode IN ('open','filtered','custom')),
  relevance_topics       text[] NOT NULL DEFAULT '{}',
  relevance_url_patterns text[] NOT NULL DEFAULT '{}',
  last_crawled_at        timestamptz,
  last_success_at        timestamptz,
  last_error_at          timestamptz,
  last_error_message     text,
  last_changed_at        timestamptz,
  consecutive_failures   int NOT NULL DEFAULT 0,
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now(),
  UNIQUE(source_id, entry_url, entity_types)
);
CREATE INDEX idx_intel_channels_due ON intel_source_channels (enabled, crawl_frequency_mins) WHERE enabled;
CREATE INDEX idx_intel_channels_source ON intel_source_channels (source_id);

CREATE TABLE intel_inbound_events (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  channel_id   uuid NOT NULL REFERENCES intel_source_channels(id) ON DELETE CASCADE,
  payload      bytea NOT NULL,
  headers      jsonb,
  content_type text,
  content_hash text NOT NULL,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  status       text NOT NULL DEFAULT 'received'
                 CHECK (status IN ('received','processed','failed','ignored'))
);
CREATE INDEX idx_intel_inbound_channel ON intel_inbound_events (channel_id, received_at DESC);

ALTER TABLE intel_raw_documents
  ADD COLUMN IF NOT EXISTS channel_id uuid REFERENCES intel_source_channels(id) ON DELETE SET NULL;

-- Backfill: categories from category; one channel per existing source.
UPDATE intel_sources SET categories = ARRAY[category] WHERE categories = '{}';

INSERT INTO intel_source_channels
  (source_id, label, entry_url, crawl_method, entity_types,
   pipeline, crawl_frequency_mins, enabled, origin, last_crawled_at,
   last_success_at, last_error_at, last_error_message, last_changed_at,
   consecutive_failures)
SELECT id, 'Default', source_url, crawl_method,
       '{}'::text[],
       'extract', crawl_frequency_mins, enabled, 'admin',
       last_crawled_at, last_success_at, last_error_at,
       last_error_message, last_changed_at, consecutive_failures
FROM intel_sources
WHERE crawl_method <> 'manual';

COMMIT;
