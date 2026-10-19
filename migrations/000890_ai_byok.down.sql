-- 000890_ai_byok.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'ai.byok');
DELETE FROM permissions WHERE key = 'ai.byok';
DROP TABLE IF EXISTS user_ai_keys;
DROP TABLE IF EXISTS ai_byok_providers;
COMMIT;
