-- 000981_verification_form_multi_step.down.sql
-- Restore the flat single-group field set and drop the step scaffolding.
-- Fields return to group d001 ordered by their step order so the flat
-- Details step keeps the same field sequence the wizard showed.

WITH ordered AS (
  SELECT f.id,
         ROW_NUMBER() OVER (ORDER BY s.position, f.position) - 1 AS new_pos
  FROM custom_fields f
  JOIN custom_field_group_assignments a
    ON a.group_id = f.group_id
   AND a.entity_class = 'form_step'
  JOIN form_steps s ON s.id = a.entity_id
  WHERE f.group_id IN (
    '00000000-0000-0000-0000-00000000d033',
    '00000000-0000-0000-0000-00000000d034',
    '00000000-0000-0000-0000-00000000d035'
  )
)
UPDATE custom_fields f
SET group_id = '00000000-0000-0000-0000-00000000d001', position = o.new_pos
FROM ordered o
WHERE f.id = o.id;

DELETE FROM custom_field_group_assignments
WHERE entity_class = 'form_step'
  AND entity_id IN (
    '00000000-0000-0000-0000-00000000d030',
    '00000000-0000-0000-0000-00000000d031',
    '00000000-0000-0000-0000-00000000d032'
  );

DELETE FROM custom_field_groups
WHERE id IN (
  '00000000-0000-0000-0000-00000000d033',
  '00000000-0000-0000-0000-00000000d034',
  '00000000-0000-0000-0000-00000000d035'
);

DELETE FROM form_steps
WHERE id IN (
  '00000000-0000-0000-0000-00000000d030',
  '00000000-0000-0000-0000-00000000d031',
  '00000000-0000-0000-0000-00000000d032'
);

UPDATE forms SET is_multi_step = false
WHERE id = '00000000-0000-0000-0000-00000000d002';
