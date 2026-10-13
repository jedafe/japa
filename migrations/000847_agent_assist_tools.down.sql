-- 000847 down: re-block the six assist tools (restore the 000788 dead-row
-- state) and clear required_permission. The feature_flags backfill row is
-- intentionally NOT removed — mirrors 000811's down-file convention.
BEGIN;

UPDATE ai_tools
SET is_enabled = false,
    required_permission = NULL,
    config = '{"is_blocked": true}'::jsonb,
    updated_at = now()
WHERE name IN (
    'content.improve',
    'content.summarize',
    'platform.translate_string',
    'content.suggest_tags',
    'content.generate_og',
    'content.generate_draft'
);

COMMIT;
