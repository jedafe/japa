-- 000443_intelligence_jobs_table.up.sql
-- Intelligence projection table for the jobs domain (§63, ADR-081).
-- Jobs previously had no dedicated table — §63 stored them as contents
-- rows (content_type='job_listing') + custom_field_values EAV. Per the
-- user's decision to use a typed table (mirroring the scholarships
-- precedent, ADR-112), this migration creates intel_jobs carrying the
-- full intelligence provenance columns (confidence, freshness,
-- editorial overrides, verification status).
--
-- The existing /jobs API (internal/jobs/service.go) will be migrated to
-- read from this table in a follow-up; for now the promoter writes here
-- and the radar API unions this table.
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

BEGIN;

CREATE TABLE IF NOT EXISTS intel_jobs (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    intel_record_id         uuid NOT NULL REFERENCES intel_records(id) ON DELETE CASCADE,
    guide_id                uuid REFERENCES contents(id) ON DELETE SET NULL,
    source_url              text NOT NULL,
    source_type             text NOT NULL,
    trust_level             int2 NOT NULL DEFAULT 0,
    title                   text NOT NULL,
    description             text,
    country                 text NOT NULL,          -- destination ISO-3
    country_space_id        uuid REFERENCES spaces(id) ON DELETE SET NULL,
    city                    text,
    employer                text,
    job_type                text,                   -- full_time|part_time|contract|internship|temporary|remote
    experience_level        text,                   -- entry|mid|senior|lead|executive
    salary_min              numeric(18,2),
    salary_max              numeric(18,2),
    currency                text,                   -- ISO 4217
    skills                  text[],
    application_url         text,
    deadline                date,
    remote                  text,                   -- true|false|hybrid
    confidence_overall      real NOT NULL DEFAULT 0,
    verification_status     text NOT NULL DEFAULT 'verified'
        CHECK (verification_status IN ('verified','needs_review','conflict','stale')),
    freshness_score         int4 NOT NULL DEFAULT 100,
    override_source         text NOT NULL DEFAULT 'ingestion'
        CHECK (override_source IN ('ingestion','editorial','mixed')),
    editorial_overrides     jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_verified_at        timestamptz NOT NULL DEFAULT now(),
    created_at              timestamptz NOT NULL DEFAULT now(),
    updated_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, intel_record_id)
);

CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_country
    ON intel_jobs (tenant_id, country);

CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_space
    ON intel_jobs (tenant_id, country_space_id)
    WHERE country_space_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_deadline
    ON intel_jobs (tenant_id, deadline)
    WHERE deadline IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_intel_jobs_tenant_job_type
    ON intel_jobs (tenant_id, job_type)
    WHERE job_type IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_intel_jobs_source_url
    ON intel_jobs (tenant_id, source_url);

-- System settings for the jobs intelligence domain.
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.jobs.enabled', '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'intel.jobs.freshness_decay_policy', '"normal"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
