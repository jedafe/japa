-- 001199_attestation_editorial_seo.down.sql
-- Revert: restore the original six-value document_type CHECK and remove the
-- attestation guide category + its field group. Rows whose document_type
-- sits outside the original six cannot be validated against the old CHECK —
-- the re-add stays NOT VALID so the down migration cannot fail on such rows.

BEGIN;

ALTER TABLE public.attestation_pathways
    DROP CONSTRAINT IF EXISTS attestation_pathways_document_type_check;

ALTER TABLE public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_document_type_check
    CHECK (document_type IN (
        'birth_certificate', 'degree', 'marriage',
        'police_clearance', 'professional_licence', 'other'
    )) NOT VALID;

-- Fields → assignment → group → entity type (dependency order).
DELETE FROM custom_fields
WHERE group_id IN (SELECT id FROM custom_field_groups WHERE slug = 'guide_attestation_fields');

DELETE FROM custom_field_group_assignments
WHERE group_id IN (SELECT id FROM custom_field_groups WHERE slug = 'guide_attestation_fields')
  AND entity_class = 'content_type:guide:attestation';

DELETE FROM custom_field_groups WHERE slug = 'guide_attestation_fields';

DELETE FROM custom_field_entity_types WHERE value = 'guide_category:attestation';

COMMIT;
