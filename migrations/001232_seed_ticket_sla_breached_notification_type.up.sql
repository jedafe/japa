-- 001232 — seed the ticket_sla_breached notification type.
-- internal/worker/handlers.go (task:maintenance:check_sla_breaches) inserts
-- notifications rows with notification_type='ticket_sla_breached', but the
-- type was never seeded — the notifications.notification_type FK rejects
-- every insert (SQLSTATE 23503), so assigned agents never receive the
-- SLA-breach alert. Mirrors the 000956 fix pattern.
--
-- Ref: .jeos/sections/35-notification-system.md
--      .jeos/sections/38-ticket-system.md (SLA Tracking)

BEGIN;

INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category) VALUES
    ('ticket_sla_breached', 'Ticket SLA Breached',
     'A ticket assigned to you has breached its SLA deadline', true, true, false, true, 'system')
ON CONFLICT (key) DO NOTHING;

COMMIT;
