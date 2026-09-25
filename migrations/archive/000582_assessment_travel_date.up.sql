-- 000582_assessment_travel_date.up.sql
-- Add an optional travel date field to the default Japa Readiness Assessment form.
-- The date is parsed by the scoring worker and surfaced in the journey context
-- that drives the geo-aware roadmap.
--
-- Ref: docs/superpowers/plans/2026-08-31-geo-aware-readiness-roadmap.md

BEGIN;

DO $$ DECLARE
    tid uuid := '00000000-0000-0000-0000-000000000001';
    gid uuid := '00000000-0000-0000-0000-00000000a001';
BEGIN

INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options, placeholder,
    is_required, is_system, show_in_frontend, position, config
) VALUES (
    '00000000-0000-0000-0000-00000000b006',
    gid,
    tid,
    'When are you planning to travel or move?',
    'travel_date',
    'date',
    '[]'::jsonb,
    'Select a target date',
    false,
    false,
    true,
    12,
    '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
) ON CONFLICT (id) DO NOTHING;

END $$;

COMMIT;
