-- 000432_intelligence_tenant_scoping.down.sql
-- Reverses the tenant-scoping changes from the up migration.
-- Drops the tenant_id column, FK constraints, and tenant-scoped indexes,
-- then recreates the original non-tenant-scoped indexes and constraints.

BEGIN;

-- 5. scholarships
DROP INDEX IF EXISTS idx_scholarships_tenant_id;
DROP INDEX IF EXISTS idx_scholarships_tenant_space;
DROP INDEX IF EXISTS idx_scholarships_tenant_status_fresh;
DROP INDEX IF EXISTS idx_scholarships_tenant_fields;
DROP INDEX IF EXISTS idx_scholarships_tenant_eligible;
DROP INDEX IF EXISTS idx_scholarships_tenant_country_deadline;
DROP INDEX IF EXISTS idx_scholarships_tenant_guide;
DROP INDEX IF EXISTS idx_scholarships_tenant_intel;
ALTER TABLE scholarships DROP CONSTRAINT IF EXISTS scholarships_tenant_id_fkey;
ALTER TABLE scholarships DROP COLUMN IF EXISTS tenant_id;

-- Restore original indexes.
CREATE UNIQUE INDEX idx_scholarships_intel
  ON scholarships (intel_record_id) WHERE intel_record_id IS NOT NULL;
CREATE UNIQUE INDEX idx_scholarships_guide
  ON scholarships (guide_id);
CREATE INDEX idx_scholarships_country_deadline
  ON scholarships (country, deadline);
CREATE INDEX idx_scholarships_eligible
  ON scholarships USING gin (eligible_countries);
CREATE INDEX idx_scholarships_fields
  ON scholarships USING gin (fields_of_study);
CREATE INDEX idx_scholarships_status_fresh
  ON scholarships (verification_status, freshness_score);
CREATE INDEX idx_scholarships_space
  ON scholarships (country_space_id);

-- 4. intel_review_log
DROP INDEX IF EXISTS idx_intel_review_log_tenant_record;
ALTER TABLE intel_review_log DROP CONSTRAINT IF EXISTS intel_review_log_tenant_id_fkey;
ALTER TABLE intel_review_log DROP COLUMN IF EXISTS tenant_id;
CREATE INDEX idx_intel_review_log_record
  ON intel_review_log (intel_record_id, created_at DESC);

-- 3. intel_records
DROP INDEX IF EXISTS idx_intel_records_tenant_id;
DROP INDEX IF EXISTS idx_intel_records_tenant_promoted;
DROP INDEX IF EXISTS idx_intel_records_tenant_geo;
DROP INDEX IF EXISTS idx_intel_records_tenant_entity_status;
ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_tenant_source_external_key;
ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_tenant_id_fkey;
ALTER TABLE intel_records DROP COLUMN IF EXISTS tenant_id;

-- Restore original unique constraint and indexes.
ALTER TABLE intel_records
  ADD CONSTRAINT intel_records_source_id_external_key_key UNIQUE (source_id, external_key);
CREATE INDEX idx_intel_records_entity_status
  ON intel_records (entity_type, status);
CREATE INDEX idx_intel_records_geo
  ON intel_records USING gin (geo_space_ids);
CREATE INDEX idx_intel_records_promoted
  ON intel_records (promoted_record_id) WHERE promoted_record_id IS NOT NULL;

-- 2. intel_raw_documents
DROP INDEX IF EXISTS idx_intel_raw_docs_tenant_hash;
DROP INDEX IF EXISTS idx_intel_raw_docs_tenant_source;
ALTER TABLE intel_raw_documents DROP CONSTRAINT IF EXISTS intel_raw_documents_tenant_id_fkey;
ALTER TABLE intel_raw_documents DROP COLUMN IF EXISTS tenant_id;
CREATE INDEX idx_intel_raw_docs_source
  ON intel_raw_documents (source_id, retrieved_at DESC);
CREATE INDEX idx_intel_raw_docs_hash
  ON intel_raw_documents (source_id, content_hash);

-- 1. intel_sources
DROP INDEX IF EXISTS idx_intel_sources_tenant_category;
DROP INDEX IF EXISTS idx_intel_sources_tenant_enabled_freq;
DROP INDEX IF EXISTS idx_intel_sources_tenant_url;
ALTER TABLE intel_sources DROP CONSTRAINT IF EXISTS intel_sources_tenant_id_fkey;
ALTER TABLE intel_sources DROP COLUMN IF EXISTS tenant_id;

-- Restore original indexes.
CREATE UNIQUE INDEX idx_intel_sources_url
  ON intel_sources (source_url);
CREATE INDEX idx_intel_sources_enabled_freq
  ON intel_sources (enabled, crawl_frequency_mins) WHERE enabled;
CREATE INDEX idx_intel_sources_category
  ON intel_sources (category, trust_level);

COMMIT;
