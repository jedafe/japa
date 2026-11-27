-- 001233 — space_invites: member→user invitations for custom spaces.
-- §05 hub-redesign amendment: the space hub gains an Invite action on
-- member-created custom spaces (primary spaces are platform-owned,
-- follow-only, and excluded). A pending invite satisfies the space's
-- joining_perm='request'/'invite' gate — the invitee accepts through the
-- normal POST /spaces/{slug}/join flow, which flips the row to 'accepted'.
--
-- The unique partial index enforces "one pending invite per
-- (space, invitee)" — duplicates 409 at the handler before insert, and
-- the index is the race-condition backstop.
--
-- Ref: .jeos/sections/05-geo-first-space-system.md
--      (Amendment addendum 2026-11-27: Space Hub Redesign)

BEGIN;

CREATE TABLE space_invites (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id     uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    inviter_id   uuid NOT NULL REFERENCES users(id),
    invitee_id   uuid NOT NULL REFERENCES users(id),
    status       text NOT NULL DEFAULT 'pending',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    responded_at timestamptz,
    CONSTRAINT space_invites_status_check
        CHECK (status IN ('pending', 'accepted', 'declined', 'revoked')),
    CONSTRAINT space_invites_no_self CHECK (inviter_id <> invitee_id)
);

-- One pending invite per (space, invitee); resolved rows may repeat.
CREATE UNIQUE INDEX space_invites_pending_unique
    ON space_invites (space_id, invitee_id) WHERE status = 'pending';
CREATE INDEX space_invites_invitee_pending
    ON space_invites (invitee_id) WHERE status = 'pending';
CREATE INDEX space_invites_space_pending
    ON space_invites (space_id) WHERE status = 'pending';
CREATE INDEX space_invites_space_history
    ON space_invites (space_id, created_at DESC);

-- Notification type for the invitee alert (FK target on
-- notifications.notification_type — must exist before the first insert).
INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category) VALUES
    ('space_invite', 'Space Invitation',
     'You have been invited to join a space', true, true, false, true, 'spaces')
ON CONFLICT (key) DO NOTHING;

COMMIT;
