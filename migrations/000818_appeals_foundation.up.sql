-- 000818_appeals_foundation.up.sql — §22 (Appeals) + §38 (Ticket System)
-- integration.
--
-- Turns moderation_appeals into the backing record for the user-facing
-- appeal flow (POST /api/v1/appeals): links each appeal to a staff-facing
-- ticket (system type 'appeal'), records resolution metadata, enforces
-- one pending appeal per (user, entity), and aligns the status CHECK with
-- the admin-panel values (pending|upheld|overturn|dismissed) — the admin
-- UI already submits these, so resolutions previously failed the
-- approved|rejected CHECK.

-- 1. Resolution + ticket-link columns.
ALTER TABLE moderation_appeals
    ADD COLUMN IF NOT EXISTS ticket_id uuid REFERENCES tickets(id),
    ADD COLUMN IF NOT EXISTS reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS resolution_note text,
    ADD COLUMN IF NOT EXISTS resolved_at timestamptz;

-- 2. Status enum: approved→overturn (appeal granted), rejected→upheld
--    (decision stands). CHECK relaxed first so the writes pass.
ALTER TABLE moderation_appeals DROP CONSTRAINT moderation_appeals_status_check;
UPDATE moderation_appeals SET status = 'overturn' WHERE status = 'approved';
UPDATE moderation_appeals SET status = 'upheld'  WHERE status = 'rejected';
ALTER TABLE moderation_appeals
    ADD CONSTRAINT moderation_appeals_status_check
    CHECK (status = ANY (ARRAY['pending','upheld','overturn','dismissed']));

-- 3. One pending appeal per (user, entity) — mirrors the §22 spec clause
--    UNIQUE(user_id, subject_type, subject_id) WHERE status='pending'.
CREATE UNIQUE INDEX IF NOT EXISTS idx_moderation_appeals_pending_unique
    ON moderation_appeals (user_id, entity_type, entity_id)
    WHERE status = 'pending';

-- 4. System ticket type for appeal threads. is_public=false keeps it out
--    of the /support type picker; tickets are created internally by the
--    appeals flow with field_values carrying the appeal context.
INSERT INTO ticket_types (name, slug, description, default_priority, is_public, is_system)
VALUES ('Appeal', 'appeal', 'User appeals against moderation decisions and rejected claims.', 'normal', false, true)
ON CONFLICT (slug) DO NOTHING;
