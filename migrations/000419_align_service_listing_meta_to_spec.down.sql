-- 000419_align_service_listing_meta_to_spec.down.sql
-- Revert the §98 alignment changes.
-- WARNING: converting text[] back to text loses array structure; existing
-- arrays are JSON-encoded to preserve data.

BEGIN;

-- verified_by / verified_at
ALTER TABLE service_listing_meta
    DROP COLUMN IF EXISTS verified_at,
    DROP COLUMN IF EXISTS verified_by;

-- claim_status default back to 'none' and migrate 'unclaimed' → 'none'.
UPDATE service_listing_meta
    SET claim_status = 'none'
    WHERE claim_status = 'unclaimed';

ALTER TABLE service_listing_meta
    ALTER COLUMN claim_status SET DEFAULT 'none';

-- origin_communities: text[] → text (JSON blob).
ALTER TABLE service_listing_meta
    ALTER COLUMN origin_communities TYPE text
    USING CASE
        WHEN origin_communities IS NULL THEN NULL
        WHEN array_length(origin_communities, 1) IS NULL THEN '[]'
        ELSE array_to_json(origin_communities)::text
    END;

ALTER TABLE service_listing_meta
    ALTER COLUMN origin_communities DROP DEFAULT;

COMMIT;
