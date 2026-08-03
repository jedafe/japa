-- 000246_ai_personas.up.sql
-- AI Persona System (Section 68, ADR-068).
-- Adds the is_bot flag to users (permanent internal truth for persona backing
-- accounts), the is_synthetic flag to platform_events (metrics integrity),
-- the ai_personas config table, and the persona.* RBAC permission nodes.
--
-- Ref: japaconnect-engineering/sections/68-ai-personas.md
--      japaconnect-engineering/adr/ADR-068-ai-persona-system.md

BEGIN;

-- =====================================================================
-- users.is_bot — permanent internal flag for AI persona backing accounts.
-- Cannot log in via normal auth flows. Never flipped after creation.
-- Never exposed publicly (disclosure_mode controls public rendering only).
-- =====================================================================
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_bot boolean NOT NULL DEFAULT false;

-- Index for fast "exclude bots" filtering in metrics queries.
CREATE INDEX IF NOT EXISTS idx_users_is_bot ON users (tenant_id, is_bot) WHERE is_bot = true;

-- =====================================================================
-- platform_events.is_synthetic — tags all persona-originated events.
-- Materialized views and real-engagement dashboards filter
-- WHERE NOT is_synthetic by default (ADR-068 §4).
-- =====================================================================
ALTER TABLE platform_events ADD COLUMN IF NOT EXISTS is_synthetic boolean NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_pe_synthetic
    ON platform_events (tenant_id, event_type, created_at DESC)
    WHERE is_synthetic = true;

-- =====================================================================
-- ai_personas — one row per persona (1:1 with backing user + byline).
-- The only genuinely new structure (ADR-068 §1: reuse-over-new-tables).
-- =====================================================================
CREATE TABLE IF NOT EXISTS ai_personas (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    profile_id       uuid NOT NULL REFERENCES professional_profiles(id) ON DELETE CASCADE,
    disclosure_mode  text NOT NULL DEFAULT 'disclosed'
                       CHECK (disclosure_mode IN ('disclosed','undisclosed')),
    autonomy_mode    text NOT NULL DEFAULT 'queued'
                       CHECK (autonomy_mode IN ('autonomous','queued')),
    voice_prompt     text,
    activity_policy  jsonb NOT NULL DEFAULT '{}',
    status           text NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active','paused','retired')),
    created_by       uuid NOT NULL REFERENCES users(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_personas_tenant ON ai_personas (tenant_id, status);
CREATE INDEX idx_ai_personas_user ON ai_personas (user_id);

-- =====================================================================
-- RBAC permission nodes — persona.* (Section 68, ADR-068)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('persona.manage', 'Full persona management (create, edit, retire)'),
    ('persona.view',   'View persona list and activity (read-only)')
ON CONFLICT (key) DO NOTHING;

-- Grant both persona.* nodes to super_admin (admin capabilities).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('persona.manage', 'persona.view')
ON CONFLICT DO NOTHING;

COMMIT;
