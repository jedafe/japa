BEGIN;

ALTER TABLE video_sources
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_video_sources_source_id ON video_sources(source_id) WHERE source_id IS NOT NULL;

ALTER TABLE comparison_data_sources
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_comparison_data_sources_source_id ON comparison_data_sources(source_id) WHERE source_id IS NOT NULL;

-- Backfill: create an intel_sources row for every unlinked video source.
INSERT INTO intel_sources (
    tenant_id, name, slug, domain, source_url, category, source_type,
    source_kind, trust_level, crawl_method, crawl_frequency, crawl_frequency_mins,
    robots_policy, source_license, attribution_required, usage_restriction,
    provider_config, enabled, status, logo_url
)
SELECT
    v.tenant_id,
    v.title,
    'video-' || substr(md5(v.source_url::text), 1, 12),
    COALESCE(regexp_replace(v.source_url, '^https?://([^/]+).*', '\1'), ''),
    v.source_url,
    'video',
    'video_upload',
    'publisher',
    2,
    'manual',
    'manual',
    0,
    '{}',
    NULL,
    false,
    NULL,
    '{}',
    true,
    'active',
    v.thumbnail_url
FROM video_sources v
WHERE v.source_id IS NULL
  AND v.status = 'active'
  AND NOT EXISTS (
      SELECT 1 FROM intel_sources s
      WHERE s.tenant_id = v.tenant_id AND s.source_url = v.source_url
  );

UPDATE video_sources v
SET source_id = s.id
FROM intel_sources s
WHERE v.tenant_id = s.tenant_id
  AND v.source_url = s.source_url
  AND v.source_id IS NULL;

-- Backfill: create an intel_sources row for every unlinked comparison data source.
INSERT INTO intel_sources (
    tenant_id, name, slug, domain, source_url, category, source_type,
    source_kind, trust_level, crawl_method, crawl_frequency, crawl_frequency_mins,
    robots_policy, source_license, attribution_required, usage_restriction,
    provider_config, enabled, status, logo_url
)
SELECT
    c.tenant_id,
    c.source_name,
    'comparison-' || substr(md5(c.source_name::text), 1, 12),
    COALESCE(regexp_replace(c.source_url, '^https?://([^/]+).*', '\1'), ''),
    c.source_url,
    'comparison',
    'comparison_api',
    'publisher',
    c.trust_level,
    'api',
    'daily',
    0,
    '{}',
    NULL,
    false,
    NULL,
    c.config,
    true,
    'active',
    NULL
FROM comparison_data_sources c
WHERE c.source_id IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM intel_sources s
      WHERE s.tenant_id = c.tenant_id
        AND s.source_url IS NOT DISTINCT FROM c.source_url
        AND s.name = c.source_name
  );

UPDATE comparison_data_sources c
SET source_id = s.id
FROM intel_sources s
WHERE c.tenant_id = s.tenant_id
  AND c.source_name = s.name
  AND c.source_url IS NOT DISTINCT FROM s.source_url
  AND c.source_id IS NULL;

COMMIT;
