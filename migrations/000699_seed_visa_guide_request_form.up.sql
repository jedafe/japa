-- 000699_seed_visa_guide_request_form.up.sql
-- Renumbered from 000509 — it collided with 000509_comment_views_table.
-- De-tenanted form: forms/custom_field_groups/custom_fields tenant_id columns
-- were dropped by 000668; the seeded rows below carry no tenant column.
-- Tail-position body; databases that applied the 000509 file skip this via the
-- 000000 registry marker.
--
-- Seeds the "Request a Visa Guide" system form — submitted when a user cannot
-- find a visa guide for their origin + destination + visa type combination in
-- the Visa Document Checklist tool (Section 70).

BEGIN;

-- 1. The form itself.
INSERT INTO forms (
    id, name, slug, description,
    is_multi_step, submit_label, success_message, status, is_system,
    submission_fee_credits
) VALUES (
    '00000000-0000-0000-0000-000000000105',
    'Request a Visa Guide',
    'visa-guide-request',
    'Submitted when a user cannot find a visa guide for their origin + destination + visa type combination in the Visa Document Checklist tool (Section 70).',
    false,
    'Submit Request',
    'Thank you — your request has been logged. Our editorial team will prioritize publishing a guide for this route.',
    'active',
    true,
    0
)
ON CONFLICT (id) DO NOTHING;

-- 2. Field group holding the form's fields.
INSERT INTO custom_field_groups (id, name, slug, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000106',
    'Visa Guide Request Fields',
    'form-visa-guide-request-fields',
    true
)
ON CONFLICT (id) DO NOTHING;

-- 3. Attach the group to the form.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES (
    '00000000-0000-0000-0000-000000000106',
    'form',
    '00000000-0000-0000-0000-000000000105',
    0
)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- 4. Fields: origin_country, destination_country, visa_type, notes.
INSERT INTO custom_fields (id, group_id, label, key, field_type, options, is_system, is_required, position) VALUES
    (
        '00000000-0000-0000-0000-000000000107',
        '00000000-0000-0000-0000-000000000106',
        'Origin Country', 'origin_country', 'text',
        NULL, true, true, 0
    ),
    (
        '00000000-0000-0000-0000-000000000108',
        '00000000-0000-0000-0000-000000000106',
        'Destination Country', 'destination_country', 'text',
        NULL, true, true, 1
    ),
    (
        '00000000-0000-0000-0000-000000000109',
        '00000000-0000-0000-0000-000000000106',
        'Visa Type', 'visa_type', 'select',
        '[{"value":"student","label":"Student"},{"value":"work","label":"Work"},{"value":"tourist","label":"Tourist"},{"value":"business","label":"Business"},{"value":"family","label":"Family Reunion"},{"value":"transit","label":"Transit"},{"value":"other","label":"Other"}]'::jsonb,
        true, true, 2
    ),
    (
        '00000000-0000-0000-0000-000000000110',
        '00000000-0000-0000-0000-000000000106',
        'Additional Notes', 'notes', 'textarea',
        NULL, true, false, 3
    )
ON CONFLICT (id) DO NOTHING;

COMMIT;
