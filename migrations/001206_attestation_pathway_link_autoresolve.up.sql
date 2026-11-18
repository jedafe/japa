BEGIN;

-- pathway_id is now an optional override: guides link to their pathway
-- automatically via the document_type + issuing_country +
-- destination_country combination (the pathway's natural key). Relabel
-- the field so editors know a manual UUID is only needed to override.
UPDATE custom_fields f
SET label = 'Linked Pathway ID (optional override)'
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_id';

COMMIT;
