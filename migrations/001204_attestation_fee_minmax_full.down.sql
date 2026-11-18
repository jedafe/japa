-- Revert fee_estimate to its half-width cell (other item_fields keep
-- their own `full` flags).

UPDATE custom_fields f
SET options = jsonb_set(
    options,
    '{item_fields}',
    (
        SELECT jsonb_agg(
            CASE WHEN elem->>'key' = 'fee_estimate'
                THEN elem - 'full'
                ELSE elem
            END
        )
        FROM jsonb_array_elements(options->'item_fields') AS elem
    )
)
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_attestation_fields'
  AND f.key = 'pathway_steps';
