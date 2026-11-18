BEGIN;

UPDATE custom_fields f
SET label = 'Linked Pathway ID'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_id';

COMMIT;
