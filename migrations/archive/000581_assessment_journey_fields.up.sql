-- 000581_assessment_journey_fields.up.sql
-- Add non-scored journey context fields to the default Japa Readiness Assessment form.
-- These fields are used by the scoring worker to produce a destination-, stage-,
-- and visa-route-aware roadmap, and by the VisitorContext resolver to update the
-- user's journey context after submission.
--
-- Ref: docs/superpowers/plans/2026-08-31-geo-aware-readiness-roadmap.md

BEGIN;

DO $$ DECLARE
    tid uuid := '00000000-0000-0000-0000-000000000001';
    gid uuid := '00000000-0000-0000-0000-00000000a001';
BEGIN

-- destination_country: the primary relocation destination.
-- Options are loaded from the API (spaces) at runtime; this is the form definition.
INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options, placeholder,
    is_required, is_system, show_in_frontend, position, config
) VALUES (
    '00000000-0000-0000-0000-00000000b001',
    gid,
    tid,
    'Which country are you planning to move to?',
    'destination_country',
    'select',
    '[]'::jsonb,
    'Select your destination country',
    false,
    false,
    true,
    7,
    '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
) ON CONFLICT (id) DO NOTHING;

-- origin_country: where the user is now. Loaded from the API at runtime.
INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options, placeholder,
    is_required, is_system, show_in_frontend, position, config
) VALUES (
    '00000000-0000-0000-0000-00000000b002',
    gid,
    tid,
    'Which country are you currently in?',
    'origin_country',
    'select',
    '[]'::jsonb,
    'Select your current country',
    false,
    false,
    true,
    8,
    '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
) ON CONFLICT (id) DO NOTHING;

-- visa_type: the intended visa route. The option list mirrors the canonical
-- visa_route values used by VisitorContext and the feed journey options.
INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options, placeholder,
    is_required, is_system, show_in_frontend, position, config
) VALUES (
    '00000000-0000-0000-0000-00000000b003',
    gid,
    tid,
    'Which visa route are you pursuing?',
    'visa_type',
    'select',
    '[{"value":"student","label":"Student visa"},{"value":"work","label":"Work visa"},{"value":"business","label":"Business visa"},{"value":"tourist","label":"Tourist/visitor visa"},{"value":"spouse","label":"Spouse/partner visa"},{"value":"family","label":"Family reunion visa"},{"value":"investment","label":"Investment/Investor visa"},{"value":"transit","label":"Transit visa"},{"value":"other","label":"Other"}]'::jsonb,
    'Select your visa route',
    false,
    false,
    true,
    9,
    '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
) ON CONFLICT (id) DO NOTHING;

-- journey_stage: where the user is in the relocation journey.
-- Canonical §87 Japa Stage state machine values.
INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options, placeholder,
    is_required, is_system, show_in_frontend, position, config
) VALUES (
    '00000000-0000-0000-0000-00000000b004',
    gid,
    tid,
    'Where are you in your relocation journey?',
    'journey_stage',
    'select',
    '[{"value":"considering","label":"Considering"},{"value":"planning","label":"Planning"},{"value":"preparing","label":"Preparing"},{"value":"applied","label":"Applied"},{"value":"approved","label":"Approved"},{"value":"relocated","label":"Relocated / settling in"},{"value":"settled","label":"Settled"}]'::jsonb,
    'Select your journey stage',
    false,
    false,
    true,
    10,
    '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
) ON CONFLICT (id) DO NOTHING;

-- purpose: the high-level reason for moving. Mirrors the canonical purpose values.
INSERT INTO custom_fields (
    id, group_id, tenant_id, label, key, field_type, options, placeholder,
    is_required, is_system, show_in_frontend, position, config
) VALUES (
    '00000000-0000-0000-0000-00000000b005',
    gid,
    tid,
    'Why are you relocating?',
    'purpose',
    'select',
    '[{"value":"study","label":"Study"},{"value":"work","label":"Work"},{"value":"business","label":"Business"},{"value":"relocation","label":"Relocation"},{"value":"vacation","label":"Vacation / extended holiday"},{"value":"tourism","label":"Tourism"},{"value":"family","label":"Family reunion"},{"value":"other","label":"Other"}]'::jsonb,
    'Select your purpose',
    false,
    false,
    true,
    11,
    '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
) ON CONFLICT (id) DO NOTHING;

END $$;

COMMIT;
