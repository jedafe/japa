-- 000431_intelligence_rbac_and_settings.up.sql
-- Seeds 8 RBAC permission nodes + 10 system_settings keys for the intelligence system.
-- Idempotent via ON CONFLICT DO NOTHING.
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md

BEGIN;

-- 1. Permission nodes
INSERT INTO permissions (key, description) VALUES
    ('intel.sources.manage',         'Intel Sources Manage'),
    ('intel.sources.crawl_now',      'Intel Sources Crawl Now'),
    ('intel.review.view',            'Intel Review View'),
    ('intel.review.approve',         'Intel Review Approve'),
    ('intel.review.reject',          'Intel Review Reject'),
    ('intel.review.edit',            'Intel Review Edit'),
    ('intel.review.disable_source',  'Intel Review Disable Source'),
    ('intel.radar.view',             'Intel Radar View')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant intel.* admin nodes to super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN (
    'intel.sources.manage', 'intel.sources.crawl_now',
    'intel.review.view', 'intel.review.approve', 'intel.review.reject',
    'intel.review.edit', 'intel.review.disable_source'
  )
ON CONFLICT DO NOTHING;

-- 3. Grant intel.radar.view to member (all authenticated users)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'intel.radar.view'
ON CONFLICT DO NOTHING;

-- 4. system_settings seeds (value column is jsonb)
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'intel.enabled',                        'true'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.schedule_batch_size',            '50'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.extraction_confidence_auto',     '0.85'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.extraction_confidence_review',   '0.50'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.extraction_confidence_reject',   '0.50'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.extraction_max_retries',         '3'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.extraction_model',               '"gpt-4o-mini"'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.freshness_decay_scholarship',    '"slow"'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.crawl_lock_ttl_secs',            '3600'::jsonb),
    ('00000000-0000-0000-0000-000000000001', 'intel.ratelimit_default_per_min',      '10'::jsonb)
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
