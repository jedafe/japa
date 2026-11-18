-- Re-lay out the pathway_steps repeater: replace the single-row flex
-- weights with per-field `full` flags so the studio renders each item as a
-- labeled card — long fields (step, official_url, note) span the full
-- width, short fields pair two-per-row.

UPDATE custom_fields f
SET options = '{"item_fields":[
    {"key":"step","label":"Step","type":"text","full":true},
    {"key":"agency","label":"Agency","type":"text"},
    {"key":"what_to_bring","label":"What to bring","type":"text"},
    {"key":"fee_estimate","label":"Fee estimate","type":"text"},
    {"key":"turnaround_days","label":"Turnaround (days)","type":"number"},
    {"key":"official_url","label":"Official URL","type":"url","full":true},
    {"key":"note","label":"Note","type":"text","full":true}
]}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_steps';
