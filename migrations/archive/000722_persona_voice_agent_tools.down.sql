-- 000722_persona_voice_agent_tools.down.sql

BEGIN;

DELETE FROM ai_tools
WHERE name IN ('persona_voices.list', 'persona_voices.read', 'personas.list', 'personas.read');

COMMIT;
