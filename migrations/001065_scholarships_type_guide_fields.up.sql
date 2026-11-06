-- 001065_scholarships_type_guide_fields.up.sql
-- Align 'scholarships' comparison dimensions with the guide field vocabulary
-- so /studio/create guide edits hydrate comparison cells (§30 Sub-spec 5).
--
-- The type was bound to wiki_scholarship_overview (award_amount, coverage,
-- eligibility, deadline, application_link), but scholarship matrix cells
-- hydrate from the GUIDE's cfv pool — entity_type='content' +
-- contents.field_values — keyed by the guide_scholarship_fields vocabulary
-- (award_amount_usd, eligibility_criteria, application_deadline, ...).
-- Only 'coverage' and 'application_link' keys coincided, and
-- wiki_scholarship_* ops write entity_type='scholarship' cfv — a bucket the
-- resolver never reads. guide_scholarship_fields is both the vocabulary the
-- guide editor writes AND the bucket the resolver reads, so it becomes the
-- dimension source.
--
-- Field-type mapping improves the generated dims: award_amount_usd
-- (number → number), application_deadline (date → text), coverage/
-- level_of_study (select → rating), is_renewable (boolean), etc.
--
-- Existing dimensions sourced from wiki_scholarship_overview fields are
-- repointed at the guide def with the matching key — two exact-key matches,
-- three semantic mappings — so they hydrate from guide-written values.
-- dimension_type is left as stored (hydration coerces by it; a text dim
-- renders the mapped value fine). source_field_key/options are joined from
-- custom_fields at read, so the field row swap alone relinks them.

BEGIN;

-- 1. Subsequent 'scholarships' comparisons generate dimensions from the
--    guide field group.
UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'guide_scholarship_fields'
  AND t.slug = 'scholarships';

-- 2. Repair existing dims: repoint source_field_id wiki-overview → guide.
--    Exact-key matches first.
UPDATE comparison_dimensions d
SET source_field_id = gdef.id
FROM custom_fields odef
JOIN custom_field_groups og ON og.id = odef.group_id AND og.slug = 'wiki_scholarship_overview'
JOIN custom_fields gdef ON gdef.key = odef.key
JOIN custom_field_groups gg ON gg.id = gdef.group_id AND gg.slug = 'guide_scholarship_fields'
WHERE d.source_field_id = odef.id
  AND odef.key IN ('coverage', 'application_link');

--    Semantic mappings (key renamed between vocabularies).
UPDATE comparison_dimensions d
SET source_field_id = gdef.id
FROM custom_fields odef
JOIN custom_field_groups og ON og.id = odef.group_id AND og.slug = 'wiki_scholarship_overview'
JOIN (VALUES
    ('award_amount', 'award_amount_usd'),
    ('eligibility',  'eligibility_criteria'),
    ('deadline',     'application_deadline')
) AS m(old_key, new_key) ON m.old_key = odef.key
JOIN custom_fields gdef ON gdef.key = m.new_key
JOIN custom_field_groups gg ON gg.id = gdef.group_id AND gg.slug = 'guide_scholarship_fields'
WHERE d.source_field_id = odef.id;

COMMIT;
