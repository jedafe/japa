-- 000615: guide_* country fields become 'suggest' fields sourced from
-- GET /geo/countries ({"source":"geo_countries"}) — same as the
-- story_fields origin/destination pickers. This replaces the two
-- divergent static 6/10-country option lists and the plain-text
-- origin/destination fields in guide_relocation_fields with a single
-- full, API-driven country list (Rule 16, §06, §79).
UPDATE custom_fields f
SET field_type = 'suggest',
    options = '{"source":"geo_countries"}'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug LIKE 'guide_%'
  AND f.key IN ('country', 'origin_country', 'destination_country');
