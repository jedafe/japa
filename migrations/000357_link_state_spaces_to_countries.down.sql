-- Reverse of 000357: clear parent_id on state spaces that were linked
-- by the up migration. We only clear states whose parent was set by
-- the slug-prefix match (i.e. parent_id was NULL before the up migration).
-- Since we can't distinguish post-hoc, we clear all state parent_ids
-- that match the slug-prefix pattern — this is safe because the up
-- migration is the only place this pattern was used.

UPDATE spaces s
SET parent_id = NULL, updated_at = now()
FROM spaces c
WHERE s.level = 'state'
  AND s.parent_id = c.id
  AND s.tenant_id = c.tenant_id
  AND c.level = 'country'
  AND UPPER(split_part(s.slug, '-', 1)) = c.geo_code
  AND s.tenant_id = '00000000-0000-0000-0000-000000000001';
