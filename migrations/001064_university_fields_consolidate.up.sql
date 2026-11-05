-- 001064_university_fields_consolidate.up.sql
-- De-dupe the university field schema after 001059: move the canonical
-- university_fields defs INTO wiki_university_overview (field rows keep their
-- deterministic UUIDs …090-…105), then retire the university_fields group.
--
-- Why a move, not a copy: custom_field_values, comparison_dimensions.
-- source_field_id, custom_field_permissions, and hardcoded field UUIDs in
-- internal/spaces/wikifacts.go + processoradapter.go all reference the FIELD
-- rows — moving the rows keeps every pointer valid. The wiki overview copy
-- seeded in 001059 has zero cfv rows, so nothing is lost dropping it.
--
-- Effect: exactly ONE field def per key platform-wide for the university
-- dimension set. Wiki edits (SuggestEdit / field ops / fields panel, via the
-- space_type:education assignment) and the intelligence promoter write the
-- same cfv rows the comparison matrix hydrates — no more per-key shadowing.
--
-- Side fixes (same seed bug as mentors in 000940): 'courses' and
-- 'cost_of_living' were also bound to university_fields. They get their own
-- wiki overview groups so their auto-dims come from the right schema.
--
-- wiki_university_admissions.acceptance_rate is renamed to
-- admissions_acceptance_rate — overview owns the canonical acceptance_rate
-- (comparison dimension); the renamed field keeps displaying in the
-- Admissions panel under its own key.
--
-- Ref: §30 Sub-spec 5 + §80 Tool Surface Hydration.

BEGIN;

-- 1. Repoint the remaining types off university_fields BEFORE the group is
--    deleted. (universities was repointed in 001059.)
UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'wiki_course_overview'
  AND t.slug = 'courses';

UPDATE comparison_type_definitions t
SET field_group_id = g.id
FROM custom_field_groups g
WHERE g.slug = 'wiki_city_cost_of_living'
  AND t.slug = 'cost_of_living';

-- 2. The education space type's profile field group follows the same defs.
UPDATE space_types
SET field_group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_university_overview')
WHERE slug = 'education';

-- 3. Drop the duplicated overview defs — every key that also exists in
--    university_fields (12 added in 001059 + the 4 originals aligned by
--    rename). cfv count for these fields is 0; dims and permissions go
--    first.
--
--    Comparisons created after 001059 have dimensions sourced from the
--    dup defs — repoint them at the canonical (deterministic-id) def with
--    the same key so hydration is unchanged.
UPDATE comparison_dimensions d
SET source_field_id = canon.id
FROM custom_fields dup
JOIN custom_field_groups dg ON dg.id = dup.group_id AND dg.slug = 'wiki_university_overview'
JOIN custom_fields canon ON canon.key = dup.key
JOIN custom_field_groups cg ON cg.id = canon.group_id AND cg.slug = 'university_fields'
WHERE d.source_field_id = dup.id;

DELETE FROM custom_field_permissions p
USING custom_fields f, custom_field_groups g
WHERE p.field_id = f.id
  AND f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key IN ('initials', 'official_website_url', 'contact_email',
      'accreditation_body', 'world_ranking', 'country_ranking', 'founded_year',
      'type', 'programs_offered', 'tuition_range', 'student_population',
      'acceptance_rate', 'international_support', 'scholarship_available',
      'application_url', 'colleges');

-- 3b. Repoint comparison_dimensions off the duplicated overview defs onto
--     the canonical university_fields rows (matched by key). The canonical
--     rows move into the overview group in step 4, so the pointers stay
--     valid; without this the FK on source_field_id blocks the delete below.
UPDATE comparison_dimensions d
SET source_field_id = cf.id
FROM custom_fields cf, custom_fields dup, custom_field_groups dg
WHERE d.source_field_id = dup.id
  AND dup.group_id = dg.id
  AND dg.slug = 'wiki_university_overview'
  AND dup.key = cf.key
  AND cf.group_id = '00000000-0000-0000-0000-000000000044';

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_overview'
  AND f.key IN ('initials', 'official_website_url', 'contact_email',
      'accreditation_body', 'world_ranking', 'country_ranking', 'founded_year',
      'type', 'programs_offered', 'tuition_range', 'student_population',
      'acceptance_rate', 'international_support', 'scholarship_available',
      'application_url', 'colleges');

-- 4. Move the 16 canonical defs into the overview group — field ids, cfv
--    rows, permission rows and dimension source_field_ids all carry over.
UPDATE custom_fields
SET group_id = (SELECT id FROM custom_field_groups WHERE slug = 'wiki_university_overview')
WHERE group_id = '00000000-0000-0000-0000-000000000044';

-- 5. Positions: moved defs carry 0-14 + 17; put the two overview-only fields
--    in the gap.
UPDATE custom_fields f
SET position = 15
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_university_overview'
  AND f.key = 'international_students_pct';

UPDATE custom_fields f
SET position = 16
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_university_overview'
  AND f.key = 'notable_for';

-- 6. Retire the emptied group and its dead class assignments.
DELETE FROM custom_field_group_assignments
WHERE group_id = '00000000-0000-0000-0000-000000000044';

DELETE FROM custom_field_groups
WHERE id = '00000000-0000-0000-0000-000000000044';

-- 7. Kill the last same-key collision: overview owns acceptance_rate.
UPDATE custom_fields f
SET key = 'admissions_acceptance_rate'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'wiki_university_admissions'
  AND f.key = 'acceptance_rate';

COMMIT;
