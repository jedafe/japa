-- 000956 — seed notification types used by code but never registered.
-- The notifications.notification_type FK rejects unseeded keys
-- (SQLSTATE 23503), so these deliveries were silently failing:
--   * admin_notification    — §06 form notify_admin trigger fan-out
--     (internal/server/server.go SetNotifyAdmin adapter)
--   * intel.source_decision — intel source proposal approve/reject notice
--     (internal/server/server.go sourceDecisionNotifierAdapter)
-- Mirrors the 000895 fix pattern.
--
-- Ref: .jeos/sections/35-notification-system.md
--      .jeos/sections/06-custom-field-builder-form-builder.md

BEGIN;

INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category) VALUES
    ('admin_notification', 'Admin Alert',
     'Alert sent to platform administrators', true, true, false, true, 'system'),
    ('intel.source_decision', 'Source Proposal Decision',
     'Your source directory proposal was approved or rejected', true, true, false, false, 'system')
ON CONFLICT (key) DO NOTHING;

COMMIT;
