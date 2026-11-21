-- 001217_eca_credential_editorial_domains.down.sql
-- Reverts the ECA/credential editorial integration:
--   • drops eca_guides.guide_id
--   • removes the guide_category:eca / guide_category:credential entity
--     types, field groups, assignments, and field definitions
--   • removes only the catalog rows this migration seeded AND that are not
--     referenced by any tool row — referenced providers/professions are
--     left in place so existing guides/pathways stay resolvable.

BEGIN;

DROP INDEX IF EXISTS idx_eca_guides_guide_id;
ALTER TABLE public.eca_guides DROP COLUMN IF EXISTS guide_id;

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id
  AND g.slug IN ('guide_eca_fields', 'guide_credential_fields');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN ('guide_eca_fields', 'guide_credential_fields');

DELETE FROM custom_field_groups
WHERE slug IN ('guide_eca_fields', 'guide_credential_fields');

DELETE FROM custom_field_entity_types
WHERE value IN ('guide_category:eca', 'guide_category:credential');

-- Unseed only unreferenced catalog rows (slug lists mirror the up file).
DELETE FROM eca_providers p
WHERE p.slug IN (
        'wes','iqas','icas','ces','ices','ece','jsa','ierf','iee','spantran',
        'gce','eres','fce','ecctis','vetassess','nzqa','uni-assist','zab',
        'qqi','nuffic','icewg','other'
    )
  AND NOT EXISTS (SELECT 1 FROM eca_guides e WHERE e.provider_id = p.id);

DELETE FROM credential_professions pr
WHERE pr.slug IN (
        'physician','registered-nurse','midwife','pharmacist','dentist',
        'physiotherapist','medical-laboratory-scientist','radiographer',
        'occupational-therapist','speech-language-therapist','optometrist',
        'clinical-psychologist','dietitian','paramedic','veterinarian',
        'civil-engineer','mechanical-engineer','electrical-engineer',
        'chemical-engineer','petroleum-engineer','structural-engineer',
        'geoscientist','lawyer','notary-public','paralegal',
        'chartered-accountant','actuary','early-childhood-educator',
        'primary-teacher','secondary-teacher','special-education-teacher',
        'architect','social-worker','quantity-surveyor',
        'urban-regional-planner','land-surveyor','airline-pilot',
        'electrician','plumber','welder'
    )
  AND NOT EXISTS (SELECT 1 FROM credential_pathways p WHERE p.profession_id = pr.id);

COMMIT;
