-- Reverses 001233: drops space_invites and unseeds the space_invite
-- notification type (only when no notifications/preferences reference it).

BEGIN;

DROP TABLE IF EXISTS space_invites;

DELETE FROM notification_types
WHERE key = 'space_invite'
  AND NOT EXISTS (
      SELECT 1 FROM notifications n WHERE n.notification_type = notification_types.key
  )
  AND NOT EXISTS (
      SELECT 1 FROM notification_preferences p WHERE p.notification_type = notification_types.key
  );

COMMIT;
