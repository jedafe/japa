-- 000573_jce_data_sources.up.sql
-- JCE Sub-spec 4: tenant-wide registry of authoritative data feeds for comparisons.
-- Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md

BEGIN;

CREATE TABLE IF NOT EXISTS comparison_data_sources (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type     text,
    entity_id       uuid,
    dimension_slug  text,
    source_name     text NOT NULL,
    source_url      text,
    source_kind     text NOT NULL CHECK (source_kind IN ('official', 'third_party', 'ai_estimate')),
    trust_level     int NOT NULL DEFAULT 3 CHECK (trust_level BETWEEN 1 AND 5),
    freshness_score int NOT NULL DEFAULT 0 CHECK (freshness_score BETWEEN 0 AND 100),
    last_verified_at timestamptz,
    config          jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_tenant
    ON comparison_data_sources(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_entity
    ON comparison_data_sources(tenant_id, entity_type, entity_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_comparison_data_sources_tenant_name
    ON comparison_data_sources(tenant_id, source_name);

COMMIT;
