-- 000261_studio_and_missing_rbac.up.sql
-- Creates the AI Content Studio tables (Section 75, ADR-076) and seeds the
-- remaining missing RBAC permission nodes: studio.* (§75), participation.*
-- (§76), and devices.*/devices.admin (used in code but never seeded).
-- Also seeds the studio.* and participation.* system_settings.
--
-- Ref: japaconnect-engineering/sections/75-ai-content-studio.md
--      japaconnect-engineering/sections/76-participation-orchestration.md
--      japaconnect-engineering/sections/77-flutter-mobile-app.md (devices nodes)

BEGIN;

-- =====================================================================
-- 1. AI Content Studio tables (Section 75, ADR-076)
-- =====================================================================

CREATE TABLE IF NOT EXISTS studio_batches (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  name        text NOT NULL,
  description text,
  created_by  uuid NOT NULL REFERENCES users(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS studio_jobs (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id        uuid NOT NULL REFERENCES tenants(id),
  batch_id         uuid REFERENCES studio_batches(id),
  persona_id       uuid NOT NULL REFERENCES ai_personas(id),
  content_type     text NOT NULL,
  guide_category   text,
  topic_prompt     text NOT NULL,
  target_space_id  uuid REFERENCES spaces(id),
  target_tags      text[],
  status           text NOT NULL DEFAULT 'queued'
                     CHECK (status IN ('queued','generating','draft_ready','in_review',
                                       'approved','published','rejected','failed')),
  draft_content_id uuid REFERENCES contents(id),
  review_id        uuid REFERENCES content_reviews(id),
  generated_at     timestamptz,
  reviewed_at      timestamptz,
  published_at     timestamptz,
  error_message    text,
  created_by       uuid NOT NULL REFERENCES users(id),
  created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_studio_jobs_status
  ON studio_jobs (tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_studio_jobs_persona
  ON studio_jobs (tenant_id, persona_id, status);

-- =====================================================================
-- 2. RBAC permission nodes — 9 missing nodes
-- =====================================================================

-- AI Content Studio (§75, ADR-076) — 4 nodes
INSERT INTO permissions (key, description) VALUES
    ('studio.manage', 'Full studio access: create, review, publish, configure'),
    ('studio.create', 'Create studio jobs and batches (without review/publish rights)'),
    ('studio.review', 'Review and approve/reject studio drafts'),
    ('studio.view',   'View studio job/batch status (read-only)')
ON CONFLICT (key) DO NOTHING;

-- Participation Orchestration (§76, ADR-077) — 3 nodes
INSERT INTO permissions (key, description) VALUES
    ('participation.manage', 'Configure activity policies and global participation settings'),
    ('participation.review', 'Review and approve/reject queued persona actions'),
    ('participation.view',   'View persona activity log (read-only)')
ON CONFLICT (key) DO NOTHING;

-- Device management (§77) — 2 nodes used in code but never seeded
INSERT INTO permissions (key, description) VALUES
    ('devices.manage', 'Register and deregister own push device tokens'),
    ('devices.admin',  'Admin: send test push notifications to any device')
ON CONFLICT (key) DO NOTHING;

-- =====================================================================
-- 3. Grant new nodes to super_admin (idempotent)
-- =====================================================================

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN (
    'studio.manage', 'studio.create', 'studio.review', 'studio.view',
    'participation.manage', 'participation.review', 'participation.view',
    'devices.manage', 'devices.admin'
  )
ON CONFLICT DO NOTHING;

-- Grant devices.manage to the default member role (all authenticated users
-- can register their own devices).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'devices.manage'
ON CONFLICT DO NOTHING;

-- =====================================================================
-- 4. system_settings — studio.* (4 keys, §75) and participation.* (3 keys, §76)
-- =====================================================================

INSERT INTO system_settings (tenant_id, key, value)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'studio.enabled',             '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'studio.max_concurrent_jobs', '"3"'),
    ('00000000-0000-0000-0000-000000000001', 'studio.auto_publish_types',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'studio.default_persona_id',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'participation.enabled',              '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'participation.run_interval_minutes', '"60"'),
    ('00000000-0000-0000-0000-000000000001', 'participation.global_daily_cap',     '"500"')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
