-- 001089 — intel discovery fan-out: notification type + recipient cap.
-- The intel:discovery_fanout task consumes {entity}.discovered DomainEvents
-- and creates intel.discovery notifications for followers of the entity's
-- geo space. The notifications.notification_type FK rejects unseeded keys
-- (SQLSTATE 23503), so the type must exist before the first delivery.
-- Mirrors the 000956 fix pattern.
--
-- Ref: .jeos/sections/35-notification-system.md
--      .jeos/sections/11-job-queue-asynq.md (intel:discovery_fanout)

BEGIN;

INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category) VALUES
    ('intel.discovery', 'Intel Discovery',
     'A new item was discovered in a space you follow', true, true, false, false, 'system')
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_settings (key, value) VALUES
    ('intel.discovery_fanout.max_recipients', to_jsonb(200))
ON CONFLICT (key) DO NOTHING;

COMMIT;
