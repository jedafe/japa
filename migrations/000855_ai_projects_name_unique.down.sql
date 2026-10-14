-- Down: drop the per-user live-name unique index.
BEGIN;

DROP INDEX IF EXISTS idx_ai_projects_user_name;

COMMIT;
