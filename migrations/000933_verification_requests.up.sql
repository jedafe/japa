-- 000933_verification_requests.up.sql
-- User-initiated verification badge requests for organizations and profiles
-- (JEOS §07 Verification Requests). Requests carry Form Builder evidence
-- (form_submissions) and queue for admin review; badge grant stays a
-- separate admin action (two-step). Also ships the ADR-029 user-profile
-- badge columns (users.verified / users.verification_type).
--
-- Ref: sections/07-organization-profiles-claim-ownership.md (Verification Requests)

BEGIN;

-- 1. users: profile verification badge (ADR-029 — spec'd, never shipped)
ALTER TABLE users ADD COLUMN IF NOT EXISTS verified boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_type text;
COMMENT ON COLUMN users.verification_type IS
  'business|embassy|notable|immigration_lawyer|professional_body — set via POST /admin/users/{id}/verify';

-- 2. verification_requests (columns mirror ownership_claims conventions)
CREATE TABLE verification_requests (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requester_user_id  uuid NOT NULL REFERENCES users(id),
  entity_type        text NOT NULL CHECK (entity_type IN ('organization','user')),
  organization_id    uuid REFERENCES organizations(id),
  requested_type     text,
  form_submission_id uuid NOT NULL REFERENCES form_submissions(id),
  status             text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','approved','rejected','withdrawn')),
  reviewed_by        uuid REFERENCES users(id),
  resolution_note    text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  reviewed_at        timestamptz,
  CONSTRAINT vreq_org_entity CHECK (
    (entity_type = 'organization' AND organization_id IS NOT NULL) OR
    (entity_type = 'user' AND organization_id IS NULL)
  )
);
CREATE INDEX idx_vreq_requester ON verification_requests (requester_user_id);
CREATE INDEX idx_vreq_status    ON verification_requests (status) WHERE status = 'pending';
CREATE UNIQUE INDEX idx_vreq_pending_org
  ON verification_requests (organization_id)
  WHERE status = 'pending' AND entity_type = 'organization';
CREATE UNIQUE INDEX idx_vreq_pending_user
  ON verification_requests (requester_user_id)
  WHERE status = 'pending' AND entity_type = 'user';

-- 3. RBAC node + grants (mirror 000144): member_role gets it, super_admin gets all.
INSERT INTO permissions (key, description) VALUES
  ('verification.request', 'Request a verification badge for own profile or administered organization')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'verification.request'
WHERE r.slug = 'member_role'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'verification.request'
WHERE r.slug = 'super_admin_role'
ON CONFLICT DO NOTHING;

-- 4. Seed the verification-request system form.
-- None of these tables carry tenant_id post-refactor.
-- The embed handler gates on status='published' (migration 000919 / CF13).
INSERT INTO custom_field_groups (id, name, slug, description, is_system)
VALUES (
  '00000000-0000-0000-0000-00000000d001',
  'Verification Request Fields', 'verification_request_fields',
  'Fields for the verification badge request form (Section 07). Admins can add/remove fields via the Form Builder.',
  true
) ON CONFLICT (id) DO NOTHING;

INSERT INTO forms (
  id, name, slug, description,
  is_multi_step, submit_label, success_message, status, is_system,
  form_type, submission_fee_credits
) VALUES (
  '00000000-0000-0000-0000-00000000d002',
  'Request Verification Badge', 'verification-request',
  'Submit evidence to request a verification badge for your profile or organization.',
  false, 'Submit Request',
  'Your verification request has been submitted and will be reviewed by our team.',
  'published', true, 'standard', 0
) ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES ('00000000-0000-0000-0000-00000000d001', 'form', '00000000-0000-0000-0000-00000000d002', 0)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

INSERT INTO custom_fields (id, group_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position) VALUES
  ('00000000-0000-0000-0000-00000000d011', '00000000-0000-0000-0000-00000000d001',
   'Legal / Registered Name', 'legal_name', 'text', 'Name as it appears on official documents', true, true, true, 0),
  ('00000000-0000-0000-0000-00000000d012', '00000000-0000-0000-0000-00000000d001',
   'Evidence URL', 'evidence_url', 'url', 'https://example.com/proof', true, true, true, 1),
  ('00000000-0000-0000-0000-00000000d013', '00000000-0000-0000-0000-00000000d001',
   'Registration / ID Number', 'registration_id', 'text', 'e.g. RC123456, bar admission no., passport no.', false, true, true, 2),
  ('00000000-0000-0000-0000-00000000d014', '00000000-0000-0000-0000-00000000d001',
   'Additional Notes', 'notes', 'textarea', 'Anything else reviewers should know…', false, true, true, 3)
ON CONFLICT (id) DO NOTHING;

INSERT INTO system_settings (key, value, is_public) VALUES
  ('verification.form_slug', '"verification-request"'::jsonb, false),
  ('verification.fee_credits', '0', false)
ON CONFLICT DO NOTHING;

COMMIT;
