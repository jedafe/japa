-- Down: drop the one-running-turn-per-conversation guard.
BEGIN;

DROP INDEX IF EXISTS ai_agent_sessions_one_running_per_conv;

COMMIT;
