-- 001222_housing_guides_natural_key.up.sql
-- §96 natural-key integrity pass on housing_guides
-- (destination_space_id × city_space_id × accommodation_type).
--
-- The pre-existing unique index used default NULLS DISTINCT semantics,
-- so every country-level row (city_space_id IS NULL) compared
-- "distinct" and slipped past it — seed drafts accumulated 243+
-- copies of the same combination. This migration:
--   1. normalizes the legacy 'apartment' accommodation_type onto the
--      canonical §96 enum value 'private_rental' (an apartment listing
--      is a private rental; the tool filter only offers §96 values),
--   2. collapses duplicate natural-key rows to a single keeper,
--      merging guide_id / intel_record_id from discarded copies so no
--      link is lost,
--   3. recreates the unique index NULLS NOT DISTINCT (PG15+) so NULL
--      city/type rows compare equal — country-level guides get the
--      constraint — and INSERT ... ON CONFLICT can arbitrate on the
--      natural key for concurrent auto-provisioning.
--
-- Ref: japaconnect-engineering/sections/96-housing-guide.md
--      japaconnect-engineering/adr/ADR-104-housing-guide.md

BEGIN;

-- 1. Canonical enum (done before dedup so a legacy 'apartment' row and
--    an already-canonical 'private_rental' row collapse into one
--    partition instead of surviving as a semantic duplicate).
UPDATE housing_guides
SET accommodation_type = 'private_rental'
WHERE accommodation_type = 'apartment';

-- 2. Keeper selection — one row per natural key, preferring the copy
--    that carries an editorial guide link, then an intel link, then
--    the earliest insert. DISTINCT ON/GROUP BY treats NULL city/type
--    as one partition, matching NULLS NOT DISTINCT semantics.
CREATE TEMP TABLE housing_dedup_keepers ON COMMIT DROP AS
SELECT DISTINCT ON (destination_space_id, city_space_id, accommodation_type)
       id, destination_space_id, city_space_id, accommodation_type
FROM housing_guides
ORDER BY destination_space_id, city_space_id, accommodation_type,
         (guide_id IS NOT NULL) DESC,
         (intel_record_id IS NOT NULL) DESC,
         created_at ASC, id ASC;

-- Merge link columns from discarded copies onto the keeper — a
-- guide_id or intel_record_id held only by a deleted dup would
-- otherwise be stranded.
UPDATE housing_guides k
SET guide_id = COALESCE(k.guide_id, src.guide_id),
    intel_record_id = COALESCE(k.intel_record_id, src.intel_record_id)
FROM housing_dedup_keepers kp
JOIN (
    SELECT destination_space_id AS d, city_space_id AS c, accommodation_type AS a,
           MAX(guide_id::text)::uuid        AS guide_id,
           MAX(intel_record_id::text)::uuid AS intel_record_id
    FROM housing_guides
    GROUP BY 1, 2, 3
) src
  ON src.d = kp.destination_space_id
 AND src.c IS NOT DISTINCT FROM kp.city_space_id
 AND src.a IS NOT DISTINCT FROM kp.accommodation_type
WHERE k.id = kp.id
  AND (k.guide_id IS NULL OR k.intel_record_id IS NULL);

DELETE FROM housing_guides
WHERE id NOT IN (SELECT id FROM housing_dedup_keepers);

-- 3. NULL-safe natural-key uniqueness.
DROP INDEX housing_guides_destination_space_id_city_space_id_key;
CREATE UNIQUE INDEX housing_guides_destination_space_id_city_space_id_key
    ON housing_guides (destination_space_id, city_space_id, accommodation_type)
    NULLS NOT DISTINCT;

COMMIT;
