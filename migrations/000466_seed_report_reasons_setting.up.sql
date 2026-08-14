-- Seed moderation.report_reasons as a public system setting (BUG-009 fix).
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md
-- Rule 16: report reason options must come from the backend, not be
-- hardcoded in frontend components. This setting is public so the frontend
-- can fetch it via GET /site/settings/public without authentication.

INSERT INTO system_settings (tenant_id, key, value, is_public)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'moderation.report_reasons',
    '["spam","harassment","hate_speech","misinformation","illegal_content","self_harm","copyright","other"]'::jsonb,
    true
)
ON CONFLICT (tenant_id, key) DO UPDATE SET value = EXCLUDED.value, is_public = true;
