-- 000306_create_planning_group_tables.up.sql
-- Japa Planning Groups (§82, ADR-090).
-- Ref: japaconnect-engineering/sections/82-japa-planning-groups.md

BEGIN;

-- IMMUTABLE week-start function for the weekly check-in unique index.
-- date_trunc('week', timestamptz) is STABLE (timezone-dependent) so it
-- cannot be used directly in an index expression. This function fixes the
-- timezone to UTC, making the result deterministic and safe for indexing.
CREATE OR REPLACE FUNCTION week_start_utc(ts timestamptz)
RETURNS date
IMMUTABLE
LANGUAGE sql
AS $$
    SELECT ((ts AT TIME ZONE 'UTC')::date - ((EXTRACT(dow FROM (ts AT TIME ZONE 'UTC')::date)::int + 6) % 7))
$$;

CREATE TABLE IF NOT EXISTS planning_groups (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                     text NOT NULL,
    description              text,
    destination_space_id     uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    visa_category            text, -- free-text visa category (no visa_categories table)
    target_departure_window  text, -- e.g. "Q1 2025"
    target_departure_at      date,
    max_members              int NOT NULL DEFAULT 50,
    is_open_join             boolean NOT NULL DEFAULT true,
    owner_id                 uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at               timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    deleted_at               timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_planning_groups_tenant_dest
    ON planning_groups(tenant_id, destination_space_id)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS planning_group_members (
    group_id        uuid NOT NULL REFERENCES planning_groups(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    departure_date  date,
    joined_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(group_id, user_id)
);

CREATE TABLE IF NOT EXISTS planning_group_milestones (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    group_id     uuid NOT NULL REFERENCES planning_groups(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title        text NOT NULL,
    completed    boolean NOT NULL DEFAULT false,
    completed_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_planning_group_milestones_group_user
    ON planning_group_milestones(group_id, user_id);

CREATE TABLE IF NOT EXISTS planning_group_checkins (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    group_id   uuid NOT NULL REFERENCES planning_groups(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    note       text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_planning_group_checkins_weekly
    ON planning_group_checkins(group_id, user_id, week_start_utc(created_at));

COMMIT;
