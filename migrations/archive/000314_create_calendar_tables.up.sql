-- 000314_create_calendar_tables.up.sql
-- Community Calendar (§86, ADR-094).
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

BEGIN;

CREATE TABLE IF NOT EXISTS calendar_entries (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title                    text NOT NULL,
    description              text,
    category                 text NOT NULL DEFAULT 'other', -- webinar|meetup|deadline|holiday|workshop|other
    destination_space_id     uuid REFERENCES spaces(id) ON DELETE SET NULL,
    origin_space_id          uuid REFERENCES spaces(id) ON DELETE SET NULL,
    occurs_at                timestamptz NOT NULL,
    ends_at                  timestamptz,
    is_recurring             boolean NOT NULL DEFAULT false,
    recurrence_pattern       text, -- iCal RRULE or simple pattern
    source_url               text,
    status                   text NOT NULL DEFAULT 'pending', -- pending|approved|archived|flagged
    submitted_by             uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by              uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at              timestamptz,
    staleness_check_due      timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    deleted_at               timestamptz -- Rule 11: soft-delete user content
);

CREATE INDEX IF NOT EXISTS idx_calendar_entries_tenant_occurs_status
    ON calendar_entries(tenant_id, occurs_at, status)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_calendar_entries_tenant_dest_status
    ON calendar_entries(tenant_id, destination_space_id, status)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS calendar_subscriptions (
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    destination_space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    category             text, -- nullable = all categories
    created_at           timestamptz NOT NULL DEFAULT now()
);

-- COALESCE in a PRIMARY KEY constraint is not allowed (PKs reference column
-- names only). Use a unique index on the expression instead.
CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_subscriptions_unique
    ON calendar_subscriptions(user_id, destination_space_id, COALESCE(category, '__all__'));

CREATE TABLE IF NOT EXISTS calendar_saves (
    user_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entry_id  uuid NOT NULL REFERENCES calendar_entries(id) ON DELETE CASCADE,
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    saved_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(user_id, entry_id)
);

COMMIT;
