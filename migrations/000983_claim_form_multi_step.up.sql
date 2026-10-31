-- 000983_claim_form_multi_step.up.sql
-- Split the claim-ownership form's single step into two Form Builder
-- steps so the claim wizard renders 4 steps
-- (Claim → Connection → Evidence → Review) — same pattern as the
-- verification-request form (000981). Per-step fields live on backing
-- custom field groups assigned with entity_class='form_step'.

UPDATE forms SET is_multi_step = true
WHERE id = '00000000-0000-0000-0000-00000000c002';

INSERT INTO form_steps (id, form_id, title, position) VALUES
  ('00000000-0000-0000-0000-00000000d040', '00000000-0000-0000-0000-00000000c002', 'Your connection', 0),
  ('00000000-0000-0000-0000-00000000d041', '00000000-0000-0000-0000-00000000c002', 'Evidence & notes', 1)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_groups (id, name, slug, description, is_system) VALUES
  ('00000000-0000-0000-0000-00000000d042', 'Claim: Your connection',
   'claim_step_connection',
   'Step 1 of the claim-ownership form: role and registration details.', true),
  ('00000000-0000-0000-0000-00000000d043', 'Claim: Evidence & notes',
   'claim_step_evidence',
   'Step 2 of the claim-ownership form: evidence links and supporting notes.', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position) VALUES
  ('00000000-0000-0000-0000-00000000d042', 'form_step', '00000000-0000-0000-0000-00000000d040', 0),
  ('00000000-0000-0000-0000-00000000d043', 'form_step', '00000000-0000-0000-0000-00000000d041', 1)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- Step 1 — Your connection: how the claimant relates to the org.
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d042', position = 0
WHERE group_id = '00000000-0000-0000-0000-00000000c001' AND key = 'role_relationship';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d042', position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000c001' AND key = 'business_registration_number';

-- Step 2 — Evidence & notes: proof links and supporting context.
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d043', position = 0
WHERE group_id = '00000000-0000-0000-0000-00000000c001' AND key = 'evidence_url';
UPDATE custom_fields SET group_id = '00000000-0000-0000-0000-00000000d043', position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000c001' AND key = 'notes';

-- Admin-added stragglers land in the last step, after the seeded pair.
WITH extras AS (
  SELECT id, ROW_NUMBER() OVER (ORDER BY position) + 1 AS new_pos
  FROM custom_fields
  WHERE group_id = '00000000-0000-0000-0000-00000000c001'
)
UPDATE custom_fields f
SET group_id = '00000000-0000-0000-0000-00000000d043', position = e.new_pos
FROM extras e
WHERE f.id = e.id;
