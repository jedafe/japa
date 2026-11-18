-- Restore the original pathway_steps item_fields options (labels only, no
-- typed inputs or flex weights).

UPDATE custom_fields f
SET options = '{"item_fields":[{"key":"step","label":"Step"},{"key":"agency","label":"Agency"},{"key":"what_to_bring","label":"What to bring"},{"key":"fee_estimate","label":"Fee estimate"},{"key":"turnaround_days","label":"Turnaround (days)"},{"key":"official_url","label":"Official URL","type":"url"},{"key":"note","label":"Note"}]}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_steps';
