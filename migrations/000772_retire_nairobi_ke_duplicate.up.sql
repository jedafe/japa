-- 000772: retire duplicate Nairobi city space created by the GeoNames/ISO
-- numeric admin1 code collision (GeoNames KE.05 "Nairobi County" was
-- code-matched to the seeded state with geo_code '05', Elgeyo-Marakwet).
-- The canonical row 'nairobi' already sits under the real Nairobi County
-- state, and the dataset dedup prevents re-creation, so the redundant
-- 'nairobi-ke' row is soft-deleted.
UPDATE spaces
SET deleted_at = now()
WHERE id = '0dbae971-ae7e-45e9-bc56-1d3a9d5b7049'
  AND slug = 'nairobi-ke'
  AND deleted_at IS NULL;
