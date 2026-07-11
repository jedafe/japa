-- 000122_moderation_alerts.down.sql
DROP TABLE IF EXISTS moderation_alerts;

DELETE FROM permissions WHERE key = 'moderation.alerts.manage';
