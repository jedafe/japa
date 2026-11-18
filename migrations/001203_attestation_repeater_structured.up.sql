-- Restructure the pathway_steps repeater item_fields:
--   * `step` removed — items auto-label "Step N" via options.item_label
--   * what_to_bring  → nested repeater (string list)
--   * fee_estimate   → minmax (Min/Max/Currency triplet → {min,max,currency})
--   * turnaround_days → minmax (Min/Max pair → {min,max}, renders "N–N days")
--   * note           → textarea
-- Renderer support lives in CustomFieldGroup.svelte.

UPDATE custom_fields f
SET options = '{"item_label":"Step","item_fields":[
    {"key":"agency","label":"Agency","type":"text","full":true},
    {"key":"what_to_bring","label":"What to bring","type":"repeater","full":true},
    {"key":"fee_estimate","label":"Fee estimate","type":"minmax","parts":["min","max","currency"]},
    {"key":"turnaround_days","label":"Turnaround (days)","type":"minmax","parts":["min","max"]},
    {"key":"official_url","label":"Official URL","type":"url","full":true},
    {"key":"note","label":"Note","type":"textarea","full":true}
]}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_steps';
