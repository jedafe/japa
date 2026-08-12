-- 000428_intelligence_spine_tables.up.sql
-- Intelligence System spine: source registry, raw documents, extraction records, review log.
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 3)
--      ADR-112

BEGIN;

CREATE TABLE intel_sources (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name                 text NOT NULL,
  domain               text NOT NULL,
  source_url           text NOT NULL,
  category             text NOT NULL,
  source_type          text NOT NULL,
  trust_level          smallint NOT NULL,
  crawl_method         text NOT NULL,
  crawl_frequency      text NOT NULL,
  crawl_frequency_mins int NOT NULL DEFAULT 1440,
  robots_policy        jsonb NOT NULL DEFAULT '{}',
  source_license       text,
  attribution_required boolean NOT NULL DEFAULT true,
  usage_restriction    text,
  provider_config      jsonb NOT NULL DEFAULT '{}',
  enabled              boolean NOT NULL DEFAULT true,
  last_crawled_at      timestamptz,
  last_success_at      timestamptz,
  last_error_at        timestamptz,
  last_error_message   text,
  last_changed_at      timestamptz,
  consecutive_failures int NOT NULL DEFAULT 0,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_intel_sources_url        ON intel_sources (source_url);
CREATE INDEX idx_intel_sources_enabled_freq      ON intel_sources (enabled, crawl_frequency_mins) WHERE enabled;
CREATE INDEX idx_intel_sources_category          ON intel_sources (category, trust_level);

CREATE TABLE intel_raw_documents (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id     uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
  url           text NOT NULL,
  http_status   int NOT NULL,
  http_headers  jsonb NOT NULL DEFAULT '{}',
  content_type  text,
  body          bytea NOT NULL,
  content_hash  text NOT NULL,
  retrieved_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_intel_raw_docs_source ON intel_raw_documents (source_id, retrieved_at DESC);
CREATE INDEX idx_intel_raw_docs_hash    ON intel_raw_documents (source_id, content_hash);

CREATE TABLE intel_records (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id           uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
  raw_document_id     uuid NOT NULL REFERENCES intel_raw_documents(id) ON DELETE CASCADE,
  entity_type         text NOT NULL,
  external_key        text NOT NULL,
  extracted           jsonb NOT NULL,
  confidence          jsonb NOT NULL,
  evidence            jsonb NOT NULL,
  confidence_overall  real NOT NULL,
  status              text NOT NULL DEFAULT 'extracted'
                        CHECK (status IN ('extracted','deduped','auto_published','review_required','approved','rejected','published','stale')),
  freshness_score     int NOT NULL DEFAULT 100,
  geo_space_ids       uuid[] NOT NULL DEFAULT '{}',
  extracted_at        timestamptz NOT NULL DEFAULT now(),
  last_verified_at    timestamptz NOT NULL DEFAULT now(),
  promoted_record_id  uuid,
  UNIQUE(source_id, external_key)
);

CREATE INDEX idx_intel_records_entity_status ON intel_records (entity_type, status);
CREATE INDEX idx_intel_records_geo           ON intel_records USING gin (geo_space_ids);
CREATE INDEX idx_intel_records_promoted      ON intel_records (promoted_record_id) WHERE promoted_record_id IS NOT NULL;

CREATE TABLE intel_review_log (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  intel_record_id uuid NOT NULL REFERENCES intel_records(id) ON DELETE CASCADE,
  actor_user_id   uuid NOT NULL REFERENCES users(id),
  action          text NOT NULL,
  field_changes   jsonb NOT NULL DEFAULT '{}',
  note            text,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_intel_review_log_record ON intel_review_log (intel_record_id, created_at DESC);

COMMIT;
