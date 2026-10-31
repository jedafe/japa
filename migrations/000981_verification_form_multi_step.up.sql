-- 000981_verification_form_multi_step.up.sql
-- Split the verification-request form's single Details step into three
-- Form Builder steps so the request wizard renders 5 steps
-- (Request → Identity → Presence → Details → Review) instead of 3.
-- Per-step fields live on backing custom field groups assigned with
-- entity_class='form_step' — the same layout CreateFormStep provisions,
-- so admins can keep editing each step's fields in the Form Builder.

UPDATE forms SET is_multi_step = true
WHERE id = '00000000-0000-0000-0000-00000000d002';

INSERT INTO form_steps (id, form_id, title, position) VALUES
  ('00000000-0000-0000-0000-00000000d030', '00000000-0000-0000-0000-00000000d002', 'Identity & documents', 0),
  ('00000000-0000-0000-0000-00000000d031', '00000000-0000-0000-0000-00000000d002', 'Online presence', 1),
  ('00000000-0000-0000-0000-00000000d032', '00000000-0000-0000-0000-00000000d002', 'Additional details', 2)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_groups (id, name, slug, description, is_system) VALUES
  ('00000000-0000-0000-0000-00000000d033', 'Verification: Identity & documents',
   'verification_step_identity',
   'Step 1 of the verification-request form: legal identity and evidence documents.', true),
  ('00000000-0000-0000-0000-00000000d034', 'Verification: Online presence',
   'verification_step_presence',
   'Step 2 of the verification-request form: social profiles used for triangulation.', true),
  ('00000000-0000-0000-0000-00000000d035', 'Verification: Additional details',
   'verification_step_additional',
   'Step 3 of the verification-request form: supplementary links and notes.', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position) VALUES
  ('00000000-0000-0000-0000-00000000d033', 'form_step', '00000000-0000-0000-0000-00000000d030', 0),
  ('00000000-0000-0000-0000-00000000d034', 'form_step', '00000000-0000-0000-0000-00000000d031', 1),
  ('00000000-0000-0000-0000-00000000d035', 'form_step', '00000000-0000-0000-0000-00000000d032', 2)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- Step 1 — Identity & documents: name, registration number, then the
-- document + live-photo uploads the reviewer matches against them.
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d033', position = 0
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'legal_name';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d033', position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'registration_id';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d033', position = 2
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'id_document';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d033', position = 3
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'live_photo';

-- Step 2 — Online presence: the optional social handles, in seeded order.
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d034', position = 0
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'x_handle';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d034', position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'instagram_handle';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d034', position = 2
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'linkedin_url';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d034', position = 3
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'facebook_url';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d034', position = 4
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'youtube_url';

-- Step 3 — Additional details: the seeded stragglers keep slots 0-1;
-- any admin-added fields still on the flat group follow in order.
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d035', position = 0
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'evidence_url';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d035', position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000d001' AND key = 'notes';

WITH extras AS (
  SELECT id, ROW_NUMBER() OVER (ORDER BY position) + 1 AS new_pos
  FROM custom_fields
  WHERE group_id = '00000000-0000-0000-0000-00000000d001'
)
UPDATE custom_fields f
SET group_id = '00000000-0000-0000-0000-00000000d035', position = e.new_pos
FROM extras e
WHERE f.id = e.id;
