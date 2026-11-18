-- Give the pathway_steps repeater item_fields proper input types and flex
-- weights so all seven sub-fields render proportionally on a single row in
-- the studio composer (turnaround_days → number, official_url → url, and
-- weighting so long-text fields get more room than the numeric one).

UPDATE custom_fields f
SET options = '{"item_fields":[
    {"key":"step","label":"Step","type":"text","flex":2},
    {"key":"agency","label":"Agency","type":"text","flex":1.5},
    {"key":"what_to_bring","label":"What to bring","type":"text","flex":1.6},
    {"key":"fee_estimate","label":"Fee estimate","type":"text","flex":0.9},
    {"key":"turnaround_days","label":"Days","type":"number","flex":0.6},
    {"key":"official_url","label":"Official URL","type":"url","flex":1.3},
    {"key":"note","label":"Note","type":"text","flex":1.6}
]}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_steps';
