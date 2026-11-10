-- 001110_readiness_audit_fixes.down.sql
-- Restore the pre-001110 gate, options, and optional flag.

BEGIN;

UPDATE custom_fields
SET config = jsonb_set(config, '{visible_if}',
  '{"any":[{"field":"purpose","op":"eq","value":"business"},{"field":"visa_type","op":"eq","value":"business"}]}'::jsonb)
WHERE id = '00000000-0000-0000-0000-00000000f027';

UPDATE custom_fields
SET options = (
      SELECT COALESCE(jsonb_agg(o), '[]'::jsonb)
      FROM jsonb_array_elements(options) o
      WHERE o->>'value' <> 'employed'),
    config = jsonb_set(config, '{option_scores}', (config->'option_scores') - 'employed')
WHERE id = '00000000-0000-0000-0000-00000000c007'
  AND options @> '[{"value":"employed"}]'::jsonb;

UPDATE custom_fields SET options = (
  SELECT jsonb_agg(CASE WHEN o->>'value' = 'not_started'
                   THEN o || '{"label":"Pets coming, not started"}'::jsonb ELSE o END)
  FROM jsonb_array_elements(options) o)
WHERE id = '00000000-0000-0000-0000-00000000f024';

UPDATE custom_fields SET options = (
  SELECT jsonb_agg(CASE WHEN o->>'value' = 'planning'
                   THEN o || '{"label":"Dependants coming, still planning"}'::jsonb ELSE o END)
  FROM jsonb_array_elements(options) o)
WHERE id = '00000000-0000-0000-0000-00000000c009';

UPDATE custom_fields SET options = (
  SELECT jsonb_agg(CASE WHEN o->>'value' = 'covered'
                   THEN o || '{"label":"Fully covered from day one"}'::jsonb ELSE o END)
  FROM jsonb_array_elements(options) o)
WHERE id = '00000000-0000-0000-0000-00000000c004';

UPDATE custom_fields SET is_required = false
WHERE id = '00000000-0000-0000-0000-00000000b004';

COMMIT;
