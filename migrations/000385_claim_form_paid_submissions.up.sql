-- 000385_claim_form_paid_submissions.up.sql
-- Adds paid form submission support (Section 06) and links the claim-ownership
-- flow to the Form Builder (Section 07). Three schema changes:
--   1. forms.submission_fee_credits — 0 = free, >0 = fee in wallet credits
--   2. form_submissions.payment_id — links to payments row for paid submissions
--   3. ownership_claims.form_submission_id — links claim to its form submission
-- Plus a seeded claim-ownership system form with default fields and system_settings.
--
-- Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md

BEGIN;

-- 1. forms: paid submission support
ALTER TABLE forms ADD COLUMN IF NOT EXISTS submission_fee_credits int NOT NULL DEFAULT 0;

-- 2. form_submissions: link to payment record
ALTER TABLE form_submissions ADD COLUMN IF NOT EXISTS payment_id uuid REFERENCES payments(id) ON DELETE SET NULL;

-- 3. ownership_claims: link to form submission
ALTER TABLE ownership_claims ADD COLUMN IF NOT EXISTS form_submission_id uuid REFERENCES form_submissions(id) ON DELETE SET NULL;

-- 4. Seed the claim-ownership system form for the default tenant.
DO $do$ DECLARE tid uuid := '00000000-0000-0000-0000-000000000001'; BEGIN

-- 4a. Custom field group for claim form fields.
INSERT INTO custom_field_groups (id, tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-00000000c001',
    tid,
    'Claim Ownership Fields',
    'claim_ownership_fields',
    'Fields for the organization ownership claim form (Section 07). Admins can add/remove fields via the Form Builder.',
    true
)
ON CONFLICT (id) DO NOTHING;

-- 4b. The claim-ownership form (is_system=true, status='active').
INSERT INTO forms (
    id, tenant_id, name, slug, description,
    is_multi_step, submit_label, success_message, status, is_system,
    submission_fee_credits
)
VALUES (
    '00000000-0000-0000-0000-00000000c002',
    tid,
    'Claim Organization Ownership',
    'claim-ownership',
    'Submit evidence and documentation to claim ownership of an organization on JapaConnect.',
    false,
    'Submit Claim',
    'Your ownership claim has been submitted and will be reviewed by our team.',
    'active',
    true,
    0
)
ON CONFLICT (id) DO NOTHING;

-- 4c. Link the field group to the form (entity_class='form').
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES (
    '00000000-0000-0000-0000-00000000c001',
    'form',
    '00000000-0000-0000-0000-00000000c002',
    0
)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- 4d. Default claim form fields.
-- Evidence URL (url, required)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position)
VALUES (
    '00000000-0000-0000-0000-00000000c011',
    '00000000-0000-0000-0000-00000000c001',
    tid,
    'Evidence URL',
    'evidence_url',
    'url',
    'https://example.com/proof-of-ownership',
    true,
    true,
    true,
    0
)
ON CONFLICT (id) DO NOTHING;

-- Business Registration Number (text, optional)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position)
VALUES (
    '00000000-0000-0000-0000-00000000c012',
    '00000000-0000-0000-0000-00000000c001',
    tid,
    'Business Registration Number',
    'business_registration_number',
    'text',
    'e.g. RC123456 / BN789012',
    false,
    true,
    true,
    1
)
ON CONFLICT (id) DO NOTHING;

-- Your Role / Relationship (select, required)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, position)
VALUES (
    '00000000-0000-0000-0000-00000000c013',
    '00000000-0000-0000-0000-00000000c001',
    tid,
    'Your Role / Relationship',
    'role_relationship',
    'select',
    '[{"value":"owner","label":"Owner / Founder"},{"value":"director","label":"Director / Board Member"},{"value":"authorized_agent","label":"Authorized Agent"},{"value":"other","label":"Other"}]'::jsonb,
    true,
    true,
    true,
    2
)
ON CONFLICT (id) DO NOTHING;

-- Notes (textarea, optional)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position)
VALUES (
    '00000000-0000-0000-0000-00000000c014',
    '00000000-0000-0000-0000-00000000c001',
    tid,
    'Additional Notes',
    'notes',
    'textarea',
    'Explain your relationship to this organization and any additional context…',
    false,
    true,
    true,
    3
)
ON CONFLICT (id) DO NOTHING;

-- 5. Seed system_settings for the claim form slug.
INSERT INTO system_settings (tenant_id, key, value, is_public)
VALUES (tid, 'claim_ownership.form_slug', '"claim-ownership"'::jsonb, false)
ON CONFLICT (tenant_id, key) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value, is_public)
VALUES (tid, 'claim_ownership.fee_credits', '0'::jsonb, false)
ON CONFLICT (tenant_id, key) DO NOTHING;

END $do$;

COMMIT;
