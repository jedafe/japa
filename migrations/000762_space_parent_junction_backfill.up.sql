-- Backfill space_parent_spaces rows for spaces whose spaces.parent_id is not
-- mirrored in the junction. The geo hierarchy (admin Geo tool, /geo routes,
-- JCE resolvers) reads only space_parent_spaces; the baseline seed left
-- 'greater-accra' and 'nairobi-county' parented via parent_id alone, so they
-- were invisible to those listings. parent_id ⊆ junction is the invariant the
-- create/SetSpaceParent paths maintain.
INSERT INTO public.space_parent_spaces (space_id, parent_space_id)
SELECT s.id, s.parent_id
FROM public.spaces s
WHERE s.parent_id IS NOT NULL
  AND s.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1
      FROM public.space_parent_spaces sps
      WHERE sps.space_id = s.id
        AND sps.parent_space_id = s.parent_id
  )
ON CONFLICT DO NOTHING;
