-- 000935_verification_fields_id_repair.up.sql
-- Repair for databases where 000933/000934 ran before the id collision
-- was caught: custom_fields ids d011-d013 were already used by the
-- readiness assessment (migration 000896, group a001), so the original
-- verification fields silently skipped and the 000934 UPDATEs mutated
-- the wrong rows. This migration:
--   1. Restores the readiness fields d011-d013 to their original state.
--   2. Inserts the missing verification fields under fresh ids (d022-d024).
--   3. Re-points evidence_url/registration_id labels+positions on d001
--      group rows if 000934 moved them (idempotent).

-- 1. Restore readiness fields (group a001, positions 23-25, select types).
UPDATE custom_fields
SET label = 'If your visa or profession requires a language test (IELTS, TEF, Goethe, etc.), where are you?',
    position = 23,
    is_required = true
WHERE id = '00000000-0000-0000-0000-00000000d011'
  AND group_id = '00000000-0000-0000-0000-00000000a001';

UPDATE custom_fields
SET label = 'How well do you know your destination (cost of living, neighbourhoods, transport, safety, healthcare)?',
    position = 24,
    is_required = true
WHERE id = '00000000-0000-0000-0000-00000000d012'
  AND group_id = '00000000-0000-0000-0000-00000000a001';

UPDATE custom_fields
SET position = 25
WHERE id = '00000000-0000-0000-0000-00000000d013'
  AND group_id = '00000000-0000-0000-0000-00000000a001';

-- 2. Insert the verification fields under fresh ids — the original
--    d011-d013 ids were taken. Mirrors the corrected 000933 seed.
INSERT INTO custom_fields (id, group_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position) VALUES
  ('00000000-0000-0000-0000-00000000d022', '00000000-0000-0000-0000-00000000d001',
   'Legal / Registered Name', 'legal_name', 'text', 'Name as it appears on official documents', true, true, true, 0),
  ('00000000-0000-0000-0000-00000000d023', '00000000-0000-0000-0000-00000000d001',
   'Additional evidence link', 'evidence_url', 'url', 'https://example.com/proof', false, true, true, 8),
  ('00000000-0000-0000-0000-00000000d024', '00000000-0000-0000-0000-00000000d001',
   'Registration / ID Number', 'registration_id', 'text', 'e.g. RC123456, bar admission no., passport no.', false, true, true, 9)
ON CONFLICT (id) DO NOTHING;
