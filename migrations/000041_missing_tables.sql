-- 000041_missing_tables.sql
-- Adds missing tables identified in the roadmap audit across multiple sections.
--
-- Tables added:
--   - space_moderators (Section 05): moderator assignments for spaces
--   - content_spaces (Section 14): multi-space post assignment
--   - editor_block_permissions (Section 15): REBUILT — drops old schema from 000011,
--     creates new version with id, tenant_id, and proper multi-tenant UNIQUE constraint
--   - page_builder_pages (Section 16): custom page builder layouts
--   - user_group_promotions (Section 03): auto-promotion rules between groups
--   - custom_field_templates (Section 06): reusable field templates
--   - onboarding_templates (Section 09): configurable onboarding step sequences
--
-- Also adds max_spaces_creatable column to user_groups (Section 05).

-- =====================
-- user_groups: add max_spaces_creatable column (Section 05)
-- 0 = none, -1 = unlimited, N = capped
-- =====================
ALTER TABLE user_groups ADD COLUMN IF NOT EXISTS max_spaces_creatable int NOT NULL DEFAULT 0;

-- =====================
-- space_moderators (Section 05)
-- Tracks which users are moderators of which spaces, and who assigned them.
-- =====================
CREATE TABLE IF NOT EXISTS space_moderators (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    space_id            uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_by         text NOT NULL DEFAULT 'site_admin',  -- 'site_admin' | 'space_owner'
    assigned_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, space_id, user_id)
);

CREATE INDEX idx_space_moderators_space ON space_moderators(space_id);
CREATE INDEX idx_space_moderators_user ON space_moderators(user_id);

-- =====================
-- content_spaces (Section 14)
-- Maps content to one or more spaces. is_primary marks the main space
-- for a post; additional rows enable cross-space visibility.
-- =====================
CREATE TABLE IF NOT EXISTS content_spaces (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    is_primary  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(content_id, space_id)
);

CREATE INDEX idx_content_spaces_space ON content_spaces(space_id);

-- =====================
-- editor_block_permissions (Section 15) — REBUILT
-- The original migration 000011 created this table without id or tenant_id,
-- using a composite PK on (block_type, user_group_id). This rebuild adds
-- proper multi-tenancy (tenant_id), a surrogate PK (id), and a unique
-- constraint on (tenant_id, block_type, user_group_id).
--
-- WARNING: This drops the old table and its seeded rows. Re-seed after migration.
-- =====================
DROP TABLE IF EXISTS editor_block_permissions;

CREATE TABLE IF NOT EXISTS editor_block_permissions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    block_type    text NOT NULL,  -- e.g. 'paragraph', 'heading', 'image', 'video'
    user_group_id uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    enabled       boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, block_type, user_group_id)
);

-- =====================
-- page_builder_pages (Section 16)
-- Custom pages built with the page builder. layout_data holds the
-- page builder JSON structure. Slug is unique per tenant.
-- =====================
CREATE TABLE IF NOT EXISTS page_builder_pages (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title        text NOT NULL,
    slug         text NOT NULL,
    description  text,
    layout_data  jsonb NOT NULL DEFAULT '{}',  -- page builder layout JSON
    status       text NOT NULL DEFAULT 'draft',  -- draft | published | archived
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

-- =====================
-- user_group_promotions (Section 03)
-- Auto-promotion rules: when a user meets trigger conditions in a source
-- group, they are automatically moved to the target group.
-- =====================
CREATE TABLE IF NOT EXISTS user_group_promotions (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_group_id   uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    target_group_id   uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    trigger_type      text NOT NULL,  -- 'post_count' | 'account_age' | 'manual'
    trigger_value     jsonb,  -- e.g. {"post_count": 10} or {"account_age_days": 30}
    is_active         boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, source_group_id, target_group_id)
);

-- =====================
-- custom_field_templates (Section 06)
-- Reusable field templates within a field group. Admins can create
-- templates and stamp them into custom_fields without redefining
-- field_type, options, and validation from scratch.
-- =====================
CREATE TABLE IF NOT EXISTS custom_field_templates (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text,
    field_group_id uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE CASCADE,
    is_system     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);

-- =====================
-- onboarding_templates (Section 09)
-- Configurable onboarding step sequences. Steps are stored as an ordered
-- JSONB array so admins can reorder without schema changes.
-- =====================
CREATE TABLE IF NOT EXISTS onboarding_templates (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text,
    steps       jsonb NOT NULL DEFAULT '[]',  -- ordered list of onboarding steps
    is_active   boolean NOT NULL DEFAULT true,
    is_system   boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, name)
);
