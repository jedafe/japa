-- 000432_intelligence_tenant_scoping.up.sql
-- Adds tenant_id to all five intelligence system tables and updates unique
-- constraints and indexes to include tenant_id.
-- Ref: ADR-112 Amendment 2 (tenant scoping, 2026-08-13)
--
-- All existing rows are backfilled with the default tenant
-- (00000000-0000-0000-0000-000000000001). The column is then set NOT NULL
-- and the FK constraint is added.

BEGIN;

-- Default tenant ID used for backfilling existing rows.
-- 00000000-0000-0000-0000-000000000001

-- ---------------------------------------------------------------------------
-- 1. intel_sources
-- ---------------------------------------------------------------------------
ALTER TABLE intel_sources ADD COLUMN tenant_id uuid;

-- Backfill from default tenant.
UPDATE intel_sources SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

ALTER TABLE intel_sources
  ALTER COLUMN tenant_id SET NOT NULL,
  ADD CONSTRAINT intel_sources_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- Drop the old unique index on source_url and replace with tenant-scoped.
DROP INDEX IF EXISTS idx_intel_sources_url;
CREATE UNIQUE INDEX idx_intel_sources_tenant_url ON intel_sources (tenant_id, source_url);

-- Update the enabled/freq index to be tenant-scoped.
DROP INDEX IF EXISTS idx_intel_sources_enabled_freq;
CREATE INDEX idx_intel_sources_tenant_enabled_freq
  ON intel_sources (tenant_id, enabled, crawl_frequency_mins) WHERE enabled;

-- Update the category index to be tenant-scoped.
DROP INDEX IF EXISTS idx_intel_sources_category;
CREATE INDEX idx_intel_sources_tenant_category
  ON intel_sources (tenant_id, category, trust_level);

-- ---------------------------------------------------------------------------
-- 2. intel_raw_documents
-- ---------------------------------------------------------------------------
ALTER TABLE intel_raw_documents ADD COLUMN tenant_id uuid;

UPDATE intel_raw_documents SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

ALTER TABLE intel_raw_documents
  ALTER COLUMN tenant_id SET NOT NULL,
  ADD CONSTRAINT intel_raw_documents_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- Update indexes to be tenant-scoped.
DROP INDEX IF EXISTS idx_intel_raw_docs_source;
CREATE INDEX idx_intel_raw_docs_tenant_source
  ON intel_raw_documents (tenant_id, source_id, retrieved_at DESC);

DROP INDEX IF EXISTS idx_intel_raw_docs_hash;
CREATE INDEX idx_intel_raw_docs_tenant_hash
  ON intel_raw_documents (tenant_id, source_id, content_hash);

-- ---------------------------------------------------------------------------
-- 3. intel_records
-- ---------------------------------------------------------------------------
ALTER TABLE intel_records ADD COLUMN tenant_id uuid;

UPDATE intel_records SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

ALTER TABLE intel_records
  ALTER COLUMN tenant_id SET NOT NULL,
  ADD CONSTRAINT intel_records_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- Drop the old unique constraint on (source_id, external_key) and replace
-- with a tenant-scoped version.
ALTER TABLE intel_records DROP CONSTRAINT IF EXISTS intel_records_source_id_external_key_key;
ALTER TABLE intel_records
  ADD CONSTRAINT intel_records_tenant_source_external_key
    UNIQUE (tenant_id, source_id, external_key);

-- Update indexes to be tenant-scoped.
DROP INDEX IF EXISTS idx_intel_records_entity_status;
CREATE INDEX idx_intel_records_tenant_entity_status
  ON intel_records (tenant_id, entity_type, status);

DROP INDEX IF EXISTS idx_intel_records_geo;
-- GIN index on geo_space_ids — tenant-scoped via a btree-gin composite or
-- a separate tenant_id btree index. We use a plain GIN on geo_space_ids
-- (tenant filtering is handled by the query's WHERE clause + the
-- tenant_id btree index below).
CREATE INDEX idx_intel_records_tenant_geo
  ON intel_records USING gin (geo_space_ids);
CREATE INDEX idx_intel_records_tenant_id
  ON intel_records (tenant_id);

DROP INDEX IF EXISTS idx_intel_records_promoted;
CREATE INDEX idx_intel_records_tenant_promoted
  ON intel_records (tenant_id, promoted_record_id) WHERE promoted_record_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 4. intel_review_log
-- ---------------------------------------------------------------------------
ALTER TABLE intel_review_log ADD COLUMN tenant_id uuid;

UPDATE intel_review_log SET tenant_id = '00000000-0000-0000-0000-000000000001' WHERE tenant_id IS NULL;

ALTER TABLE intel_review_log
  ALTER COLUMN tenant_id SET NOT NULL,
  ADD CONSTRAINT intel_review_log_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);

DROP INDEX IF EXISTS idx_intel_review_log_record;
CREATE INDEX idx_intel_review_log_tenant_record
  ON intel_review_log (tenant_id, intel_record_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- 5. scholarships
-- ---------------------------------------------------------------------------
ALTER TABLE scholarships ADD COLUMN tenant_id uuid;

-- Backfill scholarships from their guide's tenant (contents.tenant_id).
UPDATE scholarships s
SET tenant_id = COALESCE(
  (SELECT c.tenant_id FROM contents c WHERE c.id = s.guide_id),
  '00000000-0000-0000-0000-000000000001'
)
WHERE s.tenant_id IS NULL;

ALTER TABLE scholarships
  ALTER COLUMN tenant_id SET NOT NULL,
  ADD CONSTRAINT scholarships_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants(id);

-- Drop old unique indexes and replace with tenant-scoped versions.
DROP INDEX IF EXISTS idx_scholarships_intel;
CREATE UNIQUE INDEX idx_scholarships_tenant_intel
  ON scholarships (tenant_id, intel_record_id) WHERE intel_record_id IS NOT NULL;

DROP INDEX IF EXISTS idx_scholarships_guide;
CREATE UNIQUE INDEX idx_scholarships_tenant_guide
  ON scholarships (tenant_id, guide_id);

-- Update filter indexes to be tenant-scoped.
DROP INDEX IF EXISTS idx_scholarships_country_deadline;
CREATE INDEX idx_scholarships_tenant_country_deadline
  ON scholarships (tenant_id, country, deadline);

DROP INDEX IF EXISTS idx_scholarships_eligible;
CREATE INDEX idx_scholarships_tenant_eligible
  ON scholarships USING gin (eligible_countries);

DROP INDEX IF EXISTS idx_scholarships_fields;
CREATE INDEX idx_scholarships_tenant_fields
  ON scholarships USING gin (fields_of_study);

CREATE INDEX idx_scholarships_tenant_id
  ON scholarships (tenant_id);

DROP INDEX IF EXISTS idx_scholarships_status_fresh;
CREATE INDEX idx_scholarships_tenant_status_fresh
  ON scholarships (tenant_id, verification_status, freshness_score);

DROP INDEX IF EXISTS idx_scholarships_space;
CREATE INDEX idx_scholarships_tenant_space
  ON scholarships (tenant_id, country_space_id);

COMMIT;
