-- Remove wiki_scholarship_overview from the scholarships comparison
-- pool: its 10 fields are key-for-key identical to the bound
-- guide_scholarship_fields group, so the picker rail showed two
-- interchangeable groups. The group itself stays for scholarship wiki
-- pages (wiki_type_categories); only the pool row goes.

BEGIN;

DELETE FROM comparison_type_field_groups ctfg
USING comparison_type_definitions d, custom_field_groups g
WHERE ctfg.type_id = d.id
  AND ctfg.group_id = g.id
  AND d.slug = 'scholarships'
  AND g.slug = 'wiki_scholarship_overview';

COMMIT;
