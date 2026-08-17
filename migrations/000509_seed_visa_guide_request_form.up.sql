-- 000509_seed_visa_guide_request_form.up.sql
-- Seed the "Request a Visa Guide" system form for the Visa Document
-- Checklist tool (Section 70, ADR-070). When no published visa guide exists
-- for a user's origin + destination + visa-type combination, the checklist
-- selector shows a "no guide yet" graceful state with a request-a-guide CTA.
-- This migration creates the system form + field group + fields that the CTA
-- submits to via POST /api/v1/forms/{id}/submissions (Section 06).
--
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md
--      japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
--      japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md

BEGIN;

-- =====================================================================
-- 1. System form: visa-guide-request (slug), is_system=true, free, active.
--    ID ...105 (next after the ...104 suffix used in prior migrations).
-- =====================================================================
INSERT INTO forms (
    id, tenant_id, name, slug, description,
    is_multi_step, submit_label, success_message, status, is_system,
    submission_fee_credits
) VALUES (
    '00000000-0000-0000-0000-000000000105',
    '00000000-0000-0000-0000-000000000001',
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

-- =====================================================================
-- 2. Custom field group for the form's fields (ID ...106).
-- =====================================================================
INSERT INTO custom_field_groups (id, tenant_id, name, slug, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000106',
    '00000000-0000-0000-0000-000000000001',
    'Visa Guide Request Fields',
    'form-visa-guide-request-fields',
    true
)
ON CONFLICT (id) DO NOTHING;

-- =====================================================================
-- 3. Assign the group to the form (entity_class='form', entity_id=form_id).
-- =====================================================================
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES (
    '00000000-0000-0000-0000-000000000106',
    'form',
    '00000000-0000-0000-0000-000000000105',
    0
)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- =====================================================================
-- 4. Fields: origin_country, destination_country, visa_type, notes.
--    origin_country + destination_country + visa_type are required so the
--    editorial team knows exactly which route to prioritise. notes is an
--    optional free-text field for extra context.
--    IDs ...107 (origin), ...108 (destination), ...109 (visa_type), ...110 (notes).
-- =====================================================================
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, position) VALUES
    (
        '00000000-0000-0000-0000-000000000107',
        '00000000-0000-0000-0000-000000000106',
        '00000000-0000-0000-0000-000000000001',
        'Origin Country', 'origin_country', 'text',
        NULL, true, true, 0
    ),
    (
        '00000000-0000-0000-0000-000000000108',
        '00000000-0000-0000-0000-000000000106',
        '00000000-0000-0000-0000-000000000001',
        'Destination Country', 'destination_country', 'text',
        NULL, true, true, 1
    ),
    (
        '00000000-0000-0000-0000-000000000109',
        '00000000-0000-0000-0000-000000000106',
        '00000000-0000-0000-0000-000000000001',
        'Visa Type', 'visa_type', 'select',
        '[{"value":"student","label":"Student"},{"value":"work","label":"Work"},{"value":"tourist","label":"Tourist"},{"value":"business","label":"Business"},{"value":"family","label":"Family Reunion"},{"value":"transit","label":"Transit"},{"value":"other","label":"Other"}]'::jsonb,
        true, true, 2
    ),
    (
        '00000000-0000-0000-0000-000000000110',
        '00000000-0000-0000-0000-000000000106',
        '00000000-0000-0000-0000-000000000001',
        'Additional Notes', 'notes', 'textarea',
        NULL, true, false, 3
    )
ON CONFLICT (id) DO NOTHING;

COMMIT;
