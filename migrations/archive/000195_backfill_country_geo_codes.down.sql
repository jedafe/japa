-- Reverse the geo_code backfill. Sets the four backfilled country spaces
-- back to NULL geo_code. Only reverses the exact rows the up migration
-- touched (identified by slug + the known ISO codes).

UPDATE spaces
SET geo_code = NULL
WHERE level = 'country'
  AND slug IN ('nigeria', 'united-kingdom', 'canada', 'united-states')
  AND geo_code IN ('NG', 'GB', 'CA', 'US');
