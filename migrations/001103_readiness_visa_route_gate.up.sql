-- 001103_readiness_visa_route_gate.up.sql
--
-- "Which visa route are you pursuing?" (visa_type) and "Why are you
-- relocating?" (purpose) asked nearly the same fact twice — their option
-- sets correlate 1:1 for the common routes (study/work/business/tourism).
-- Purpose is the intent axis; visa_type is the legal-route axis and only
-- adds signal when the intent doesn't determine the route (relocation,
-- family, vacation, other) or is unanswered. Gate visa_type on purpose
-- with an ADR-137 visible_if, and rotate the three general-step positions
-- so the driver question precedes the gated one (purpose → visa_type →
-- journey_stage). JourneyContext derives VisaRoute from Purpose when
-- visa_type is unanswered — normalizeVisaRoute already accepts the
-- purpose values.
--
-- Ref: .jeos/sections/71-readiness-assessment.md
-- Ref: .jeos/adr/ADR-137-conditional-form-logic.md

BEGIN;

-- Order: purpose (driver) → visa_type (gated) → journey_stage. Rotate each
-- field into the next field's slot so both the seeded (9/10/11) and live
-- (3/4/5) layouts land on the same relative order.
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
  WHEN '00000000-0000-0000-0000-00000000b005' THEN (SELECT position FROM pos WHERE id = '00000000-0000-0000-0000-00000000b003')
  WHEN '00000000-0000-0000-0000-00000000b003' THEN (SELECT position FROM pos WHERE id = '00000000-0000-0000-0000-00000000b004')
  WHEN '00000000-0000-0000-0000-00000000b004' THEN (SELECT position FROM pos WHERE id = '00000000-0000-0000-0000-00000000b005')
END
WHERE cf.id IN (
  '00000000-0000-0000-0000-00000000b003',
  '00000000-0000-0000-0000-00000000b004',
  '00000000-0000-0000-0000-00000000b005'
);

-- visa_type only renders when purpose can't determine the route —
-- unanswered, or an ambiguous intent (relocation/family/vacation/other).
UPDATE public.custom_fields
SET config = config || '{
  "visible_if": {
    "any": [
      {"field": "purpose", "op": "not_answered"},
      {"field": "purpose", "op": "in", "value": ["relocation", "family", "vacation", "other"]}
    ]
  }
}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000b003';

COMMIT;
