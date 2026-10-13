-- 000850_user_journeys.up.sql
-- ADR-129: Concurrent Journeys (Trip Instances).
--
-- user_journeys holds concurrent and past journeys beside the primary
-- profile journey (user_profiles.japa_intent/japa_stage). Each row carries
-- its own intent×stage pair under the ADR-122 class rule, a lifecycle
-- status, an optional travel window, a per-journey stage history, and a
-- soft-delete timestamp (Rule 11).
--
-- 1. user_journeys table + indexes.
-- 2. japa_stage.max_journeys system setting — per-member cap on
--    non-deleted journey rows.

BEGIN;

CREATE TABLE user_journeys (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    intent               text        NOT NULL,
    stage                text        NOT NULL DEFAULT 'planning',
    destination_space_id uuid        REFERENCES spaces(id) ON DELETE SET NULL,
    label                text,
    travel_start         date,
    travel_end           date,
    status               text        NOT NULL DEFAULT 'planned',
    is_public            boolean     NOT NULL DEFAULT false,
    history              jsonb       NOT NULL DEFAULT '[]'::jsonb,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    deleted_at           timestamptz,

    CONSTRAINT user_journeys_intent_chk
        CHECK (intent IN ('relocation','study','work','family','visit','business')),
    CONSTRAINT user_journeys_stage_chk
        CHECK (stage IN ('considering','planning','preparing','applied','approved',
                         'relocated','settled','traveled','returned')),
    -- ADR-122 class rule: each journey class owns its terminal pair.
    CONSTRAINT user_journeys_intent_stage_chk
        CHECK (
            (
                intent IN ('relocation','study','work','family')
                AND stage IN ('considering','planning','preparing','applied','approved',
                              'relocated','settled')
            )
            OR (
                intent IN ('visit','business')
                AND stage IN ('considering','planning','preparing','applied','approved',
                              'traveled','returned')
            )
        ),
    CONSTRAINT user_journeys_status_chk
        CHECK (status IN ('planned','active','completed','cancelled')),
    CONSTRAINT user_journeys_window_chk
        CHECK (travel_end IS NULL OR travel_start IS NULL OR travel_end >= travel_start)
);

-- Own-journey listing (excludes soft-deleted rows at the query layer).
CREATE INDEX idx_user_journeys_user
    ON user_journeys (user_id)
    WHERE deleted_at IS NULL;

-- Destination trip directory / co-traveler discovery (future consumers).
CREATE INDEX idx_user_journeys_dest_active
    ON user_journeys (destination_space_id)
    WHERE deleted_at IS NULL AND status IN ('planned','active');

-- Per-member journey cap (Rule 3 — no magic numbers).
INSERT INTO system_settings (key, value, is_public)
SELECT 'japa_stage.max_journeys', '10', false
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'japa_stage.max_journeys');

COMMIT;
