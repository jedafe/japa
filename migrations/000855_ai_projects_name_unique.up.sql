-- 000855_ai_projects_name_unique.up.sql
-- AI Projects: one live project name per user (case-insensitive) so the
-- sidebar can't accumulate ambiguous "Project"/"Project" groups. The
-- partial unique index is enforced per user across non-deleted rows;
-- soft-deleted names are reusable.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
--      (Amendment — AI Projects)

BEGIN;

CREATE UNIQUE INDEX idx_ai_projects_user_name
    ON ai_projects (user_id, lower(name))
    WHERE deleted_at IS NULL;

COMMIT;
