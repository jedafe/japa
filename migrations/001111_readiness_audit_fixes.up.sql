-- 001110_readiness_audit_fixes.up.sql
-- Remaining consistency fixes from the full-questionnaire audit (§71,
-- ADR-137).
--
-- 1. business_readiness gate misses the investment route: PurposeForVisaRoute
--    maps investment->business for JourneyContext, but the visible_if only
--    opened on visa_type=business — a post-move investor (purpose hidden)
--    had the question suppressed despite a business-route answer.
-- 2. job_search_progress had no honest top answer for a user already
--    employed at the destination (common for relocated/settled): best fit
--    was "Offer secured". New "employed" option scored 100 like "offer".
-- 3. Option labels still tense-bound for post-move users:
--    pets_relocation "Pets coming, not started", dependents_plan
--    "Dependants coming, still planning", health_insurance "Fully covered
--    from day one" (day one is past for relocated/settled users).
-- 4. journey_stage is the primary conditional driver — optional meant a
--    skipped stage failed every gate open and shed nothing. Required so
--    each stage choice determines the rest as designed.

BEGIN;

UPDATE custom_fields
SET config = jsonb_set(config, '{visible_if}',
  '{"any":[{"field":"purpose","op":"eq","value":"business"},{"field":"visa_type","op":"in","value":["business","investment"]}]}'::jsonb)
WHERE id = '00000000-0000-0000-0000-00000000f027';

UPDATE custom_fields
SET options = options || '[{"value":"employed","label":"Already employed at destination"}]'::jsonb,
    config = jsonb_set(config, '{option_scores}', (config->'option_scores') || '{"employed":100}'::jsonb)
WHERE id = '00000000-0000-0000-0000-00000000c007'
  AND NOT options @> '[{"value":"employed"}]'::jsonb;

UPDATE custom_fields SET options = (
  SELECT jsonb_agg(CASE WHEN o->>'value' = 'not_started'
                   THEN o || '{"label":"Not started yet"}'::jsonb ELSE o END)
  FROM jsonb_array_elements(options) o)
WHERE id = '00000000-0000-0000-0000-00000000f024';

UPDATE custom_fields SET options = (
  SELECT jsonb_agg(CASE WHEN o->>'value' = 'planning'
                   THEN o || '{"label":"Still being planned"}'::jsonb ELSE o END)
  FROM jsonb_array_elements(options) o)
WHERE id = '00000000-0000-0000-0000-00000000c009';

UPDATE custom_fields SET options = (
  SELECT jsonb_agg(CASE WHEN o->>'value' = 'covered'
                   THEN o || '{"label":"Fully covered"}'::jsonb ELSE o END)
  FROM jsonb_array_elements(options) o)
WHERE id = '00000000-0000-0000-0000-00000000c004';

UPDATE custom_fields SET is_required = true
WHERE id = '00000000-0000-0000-0000-00000000b004';

COMMIT;
