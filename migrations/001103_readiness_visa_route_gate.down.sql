-- 001103_readiness_visa_route_gate.down.sql
--
-- Reverts the visa_type visible_if gate and restores the pre-migration
-- question order (visa_type → journey_stage → purpose).

BEGIN;

-- Reverse the rotation: purpose → visa_type → journey_stage becomes
-- visa_type → journey_stage → purpose again.
WITH pos AS (
  SELECT id, position FROM public.custom_fields
  WHERE id IN (
    '00000000-0000-0000-0000-00000000b003',
    '00000000-0000-0000-0000-00000000b004',
    '00000000-0000-0000-0000-00000000b005'
  )
)
UPDATE public.custom_fields cf
SET position = CASE cf.id
  WHEN '00000000-0000-0000-0000-00000000b003' THEN (SELECT position FROM pos WHERE id = '00000000-0000-0000-0000-00000000b005')
  WHEN '00000000-0000-0000-0000-00000000b004' THEN (SELECT position FROM pos WHERE id = '00000000-0000-0000-0000-00000000b003')
  WHEN '00000000-0000-0000-0000-00000000b005' THEN (SELECT position FROM pos WHERE id = '00000000-0000-0000-0000-00000000b004')
END
WHERE cf.id IN (
  '00000000-0000-0000-0000-00000000b003',
  '00000000-0000-0000-0000-00000000b004',
  '00000000-0000-0000-0000-00000000b005'
);

UPDATE public.custom_fields
SET config = config - 'visible_if'
WHERE id = '00000000-0000-0000-0000-00000000b003';

COMMIT;
