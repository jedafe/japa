BEGIN;

DELETE FROM notification_types
WHERE key IN ('ticket_sla_breached')
  AND NOT EXISTS (
      SELECT 1 FROM notifications n WHERE n.notification_type = notification_types.key
  )
  AND NOT EXISTS (
      SELECT 1 FROM notification_preferences p WHERE p.notification_type = notification_types.key
  );

COMMIT;
