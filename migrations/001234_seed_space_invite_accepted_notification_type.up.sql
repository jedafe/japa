-- 001234 — seed the space_invite_accepted notification type.
-- ToggleSpaceMembership notifies the inviter when their space_invites row
-- resolves to 'accepted' — the notifications.notification_type FK needs
-- the type seeded before the first insert. Mirrors the 001232 pattern.
--
-- Ref: .jeos/sections/05-geo-first-space-system.md
--      .jeos/sections/35-notification-system.md

BEGIN;

INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category) VALUES
    ('space_invite_accepted', 'Space Invite Accepted',
     'Your invitation to a space was accepted', true, true, false, true, 'spaces')
ON CONFLICT (key) DO NOTHING;

COMMIT;
