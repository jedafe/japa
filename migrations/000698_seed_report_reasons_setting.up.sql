-- 000698_seed_report_reasons_setting.up.sql
-- Renumbered from 000466 — it collided with 000466_gamification_awarded_by_set_null.
-- De-tenanted form: system_settings.tenant_id was dropped by 000668 and rows are
-- keyed globally on `key`. Tail-position body; databases that applied the 000466
-- file skip this via the 000000 registry marker.

INSERT INTO system_settings (key, value, is_public)
VALUES (
    'moderation.report_reasons',
    '["spam","harassment","hate_speech","misinformation","illegal_content","self_harm","copyright","other"]'::jsonb,
    true
)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, is_public = true;
