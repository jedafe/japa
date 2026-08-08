-- 000308_create_visa_timeline_tables.up.sql
-- Visa Timeline Tracker (§83, ADR-091).
-- Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md

BEGIN;

-- IMMUTABLE date-only function for the daily unique index.
-- timestamptz::date is STABLE (timezone-dependent) and cannot be used
-- directly in an index expression. This function fixes the timezone to UTC.
CREATE OR REPLACE FUNCTION date_utc(ts timestamptz)
RETURNS date
IMMUTABLE
LANGUAGE sql
AS $$
    SELECT (ts AT TIME ZONE 'UTC')::date
$$;

CREATE TABLE IF NOT EXISTS visa_timelines (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id               uuid REFERENCES users(id) ON DELETE SET NULL, -- nullable for anonymous
    origin_space_id       uuid REFERENCES spaces(id) ON DELETE SET NULL,
    destination_space_id  uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    visa_category         text NOT NULL, -- free-text visa category
    consulate             text,
    application_method    text, -- in_person|mail|online|agent
    outcome               text, -- approved|denied|pending|withdrawn
    is_shared             boolean NOT NULL DEFAULT false,
    anonymised_at         timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    deleted_at            timestamptz -- Rule 11: soft-delete user content
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_visa_timelines_user_cat_date
    ON visa_timelines(user_id, visa_category, date_utc(created_at))
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS visa_timeline_milestones (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    timeline_id    uuid NOT NULL REFERENCES visa_timelines(id) ON DELETE CASCADE,
    milestone_type text NOT NULL, -- application_submitted|biometrics|interview|decision|passport_collected|other
    occurred_at    date NOT NULL,
    notes          text,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_visa_timeline_milestones_timeline_type
    ON visa_timeline_milestones(timeline_id, milestone_type);

-- visa_processing_stats is a materialized aggregate table refreshed nightly
-- by the visa_timeline:refresh_stats Asynq task.
CREATE TABLE IF NOT EXISTS visa_processing_stats (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    origin_space_id       uuid REFERENCES spaces(id) ON DELETE SET NULL,
    destination_space_id  uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    visa_category         text NOT NULL,
    consulate             text,
    period_start          date NOT NULL,
    sample_count          int NOT NULL DEFAULT 0,
    avg_days              numeric,
    median_days           numeric,
    p10_days              numeric,
    p90_days              numeric,
    refreshed_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, origin_space_id, destination_space_id, visa_category, consulate, period_start)
);

COMMIT;
