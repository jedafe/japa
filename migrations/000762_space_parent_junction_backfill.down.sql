-- Undo the backfill for the two seeded spaces it was written for. Scoped to
-- the known seed artifacts rather than the generic predicate so junction rows
-- legitimately mirroring parent_id (e.g. accra -> greater-accra) are kept.
DELETE FROM public.space_parent_spaces sps
USING public.spaces s
WHERE sps.space_id = s.id
  AND sps.parent_space_id = s.parent_id
  AND s.slug IN ('greater-accra', 'nairobi-county');
