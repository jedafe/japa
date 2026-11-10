-- 001107_readiness_postmove_fixes.down.sql
-- Reverse the post-move gates and restore the original labels.

BEGIN;

UPDATE custom_fields
SET config = config - 'visible_if'
WHERE id IN (
  '00000000-0000-0000-0000-00000000b006', -- travel_date
  '00000000-0000-0000-0000-00000000c002', -- relocation_budget
  '00000000-0000-0000-0000-00000000e015'  -- emergency_buffer
);

UPDATE custom_fields SET label = 'Which country are you planning to move to?'
WHERE id = '00000000-0000-0000-0000-00000000b001';

UPDATE custom_fields SET label = 'Which country are you currently in?'
WHERE id = '00000000-0000-0000-0000-00000000b002';

UPDATE custom_fields SET label = 'Which visa route are you pursuing?'
WHERE id = '00000000-0000-0000-0000-00000000b003';

UPDATE custom_fields SET label = 'Do you have a stable income source lined up at your destination?'
WHERE id = '00000000-0000-0000-0000-00000000a012';

UPDATE custom_fields SET label = 'How many months of living expenses have you saved for your move?'
WHERE id = '00000000-0000-0000-0000-00000000a011';

UPDATE custom_fields SET label = 'How prepared do you feel emotionally for the move?'
WHERE id = '00000000-0000-0000-0000-00000000a041';

UPDATE custom_fields SET label = 'Is health insurance or healthcare coverage arranged for your arrival?'
WHERE id = '00000000-0000-0000-0000-00000000c004';

UPDATE custom_fields SET label = 'Have you sorted how you''ll bank and access money on arrival (foreign account, fintech, forex plan)?'
WHERE id = '00000000-0000-0000-0000-00000000e020';

UPDATE custom_fields SET label = 'Beyond your relocation budget, do you have an emergency buffer for your first months?'
WHERE id = '00000000-0000-0000-0000-00000000e015';

UPDATE custom_fields SET label = 'Who is relocating with you? Select all that apply.'
WHERE id = '00000000-0000-0000-0000-00000000b007';

COMMIT;
