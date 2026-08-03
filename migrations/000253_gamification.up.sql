-- 000253_gamification.up.sql
-- Advanced Gamification (Section 64, ADR-082).
-- Creates badge_definitions, user_badges, challenge_definitions,
-- user_challenge_progress tables, seeds 6 system badges, and seeds
-- gamification.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/64-gamification.md
--      japaconnect-engineering/adr/ADR-082-advanced-gamification.md

BEGIN;

-- =====================================================================
-- badge_definitions — admin-configurable badge definitions with
-- trigger types: lifetime_points threshold, activity_count, or manual.
-- =====================================================================
CREATE TABLE IF NOT EXISTS badge_definitions (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug                  text NOT NULL,
    name                  text NOT NULL,
    description           text,
    icon_media_id         uuid REFERENCES media(id),
    trigger_type          text NOT NULL
                            CHECK (trigger_type IN ('lifetime_points','activity_count','manual')),
    trigger_value         int,
    trigger_activity_key  text,
    is_active             boolean NOT NULL DEFAULT true,
    is_system             boolean NOT NULL DEFAULT false,
    display_order         int NOT NULL DEFAULT 0,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_badge_definitions_tenant
    ON badge_definitions (tenant_id, is_active, display_order);

-- =====================================================================
-- user_badges — earned badge instances (1:1 user × badge).
-- awarded_by is NULL for system-awarded, set for manual admin awards.
-- =====================================================================
CREATE TABLE IF NOT EXISTS user_badges (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_id    uuid NOT NULL REFERENCES badge_definitions(id) ON DELETE CASCADE,
    awarded_at  timestamptz NOT NULL DEFAULT now(),
    awarded_by  uuid REFERENCES users(id),
    UNIQUE(tenant_id, user_id, badge_id)
);

CREATE INDEX IF NOT EXISTS idx_user_badges_user
    ON user_badges (tenant_id, user_id, awarded_at DESC);

-- =====================================================================
-- challenge_definitions — time-bounded action goals with rewards.
-- =====================================================================
CREATE TABLE IF NOT EXISTS challenge_definitions (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug             text NOT NULL,
    name             text NOT NULL,
    description      text,
    image_media_id   uuid REFERENCES media(id),
    activity_key     text NOT NULL,
    target_count     int  NOT NULL,
    reward_points    int  NOT NULL DEFAULT 0,
    reward_badge_id  uuid REFERENCES badge_definitions(id),
    starts_at        timestamptz NOT NULL DEFAULT now(),
    ends_at          timestamptz,
    is_active        boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_challenge_definitions_tenant
    ON challenge_definitions (tenant_id, is_active, starts_at DESC);

-- =====================================================================
-- user_challenge_progress — per-user progress toward a challenge.
-- =====================================================================
CREATE TABLE IF NOT EXISTS user_challenge_progress (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    challenge_id   uuid NOT NULL REFERENCES challenge_definitions(id) ON DELETE CASCADE,
    current_count  int  NOT NULL DEFAULT 0,
    completed      boolean NOT NULL DEFAULT false,
    completed_at   timestamptz,
    reward_claimed boolean NOT NULL DEFAULT false,
    UNIQUE(tenant_id, user_id, challenge_id)
);

CREATE INDEX IF NOT EXISTS idx_challenge_progress_user
    ON user_challenge_progress (tenant_id, user_id, completed);

-- =====================================================================
-- Seed 6 system badges (is_system = true).
-- =====================================================================
INSERT INTO badge_definitions (tenant_id, slug, name, description, trigger_type, trigger_value, trigger_activity_key, is_system, is_active, display_order)
VALUES
    ('00000000-0000-0000-0000-000000000001', 'first_post',       'First Post',         'Published your first piece of content', 'activity_count', 1, 'content.created', true, true, 1),
    ('00000000-0000-0000-0000-000000000001', 'first_comment',    'First Comment',      'Joined the conversation with your first comment', 'activity_count', 1, 'comment.created', true, true, 2),
    ('00000000-0000-0000-0000-000000000001', 'space_explorer',   'Space Explorer',     'Joined 5 spaces', 'activity_count', 5, 'space.joined', true, true, 3),
    ('00000000-0000-0000-0000-000000000001', 'community_pillar', 'Community Pillar',   'Earned 1000 lifetime points', 'lifetime_points', 1000, NULL, true, true, 4),
    ('00000000-0000-0000-0000-000000000001', 'japa_ready',       'Japa Ready',         'Completed the Readiness Assessment', 'activity_count', 1, 'assessment.completed', true, true, 5),
    ('00000000-0000-0000-0000-000000000001', 'super_connector',  'Super Connector',    'Followed 50 people', 'activity_count', 50, 'follow.created', true, true, 6)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- =====================================================================
-- system_settings — 3 gamification.* keys (Section 64, ADR-082)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'gamification.badges_enabled',      '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'gamification.challenges_enabled',  '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'gamification.leaderboard_enabled', '"true"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 4 gamification.* nodes (Section 64, ADR-082)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('gamification.view',              'View badges, challenges, leaderboard (all users)'),
    ('gamification.manage_badges',     'Admin: create/edit badge definitions'),
    ('gamification.manage_challenges', 'Admin: create/edit challenges'),
    ('gamification.award_badge',       'Admin: manually award a badge to a user')
ON CONFLICT (key) DO NOTHING;

-- Grant gamification.manage_* + gamification.award_badge to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('gamification.manage_badges', 'gamification.manage_challenges', 'gamification.award_badge')
ON CONFLICT DO NOTHING;

-- Grant gamification.view to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'gamification.view'
ON CONFLICT DO NOTHING;

COMMIT;
