-- Soft-delete the attestation stub guide posts seeded by 001200 up.
-- Matched by the same generated-slug + pathway_id pair the up migration
-- creates so hand-authored attestation guides are never touched.

UPDATE contents c
SET deleted_at = now(),
    status = 'archived',
    updated_at = now()
FROM attestation_pathways p
JOIN spaces si ON si.id = p.issuing_space_id
JOIN spaces sd ON sd.id = p.destination_space_id
WHERE c.guide_category = 'attestation'
  AND c.deleted_at IS NULL
  AND si.geo_code IS NOT NULL
  AND sd.geo_code IS NOT NULL
  AND c.slug = 'attestation-' || replace(p.document_type, '_', '-')
             || '-' || lower(si.geo_code) || '-' || lower(sd.geo_code)
  AND c.field_values->>'pathway_id' = p.id::text;
