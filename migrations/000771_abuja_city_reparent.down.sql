-- Reverses 000771: restore Abuja's parent/junction to the soft-deleted
-- NG-FC 'federal-capital-territory' state and undelete 'abuja-ng'.
UPDATE public.spaces
SET parent_id = '00000000-0000-0000-0000-000000000202', updated_at = now()
WHERE id = '00000000-0000-0000-0000-000000000302' AND deleted_at IS NULL;

UPDATE public.space_parent_spaces
SET parent_space_id = '00000000-0000-0000-0000-000000000202'
WHERE space_id = '00000000-0000-0000-0000-000000000302';

UPDATE public.spaces
SET status = 'active', deleted_at = NULL
WHERE slug = 'abuja-ng' AND deleted_at IS NOT NULL;
