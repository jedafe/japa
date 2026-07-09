-- 000111_content_reports.up.sql
-- Content Reports & Flagging (JEOS Section 51). Users can report content,
-- comments, users, spaces, and DM messages; reports feed the admin
-- moderation queue (Section 22).
--
-- reporter_user_id is nullable (not NOT NULL as the section's schema block
-- states) to allow the AI moderation agent (Section 20) to create reports
-- with reporter_user_id = NULL, per that same section's "Auto-Flagging
-- Integration" — the two parts of the spec conflict; nullable is required
-- for the described AI-flag capability to be possible at all. User-submitted
-- report routes enforce a non-null reporter at the application layer.

BEGIN;

CREATE TABLE content_reports (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    reporter_user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    entity_type      text NOT NULL CHECK (entity_type IN ('content', 'comment', 'user', 'space', 'message')),
    entity_id        uuid NOT NULL,
    reason           text NOT NULL CHECK (reason IN (
        'spam', 'misinformation', 'harassment', 'hate_speech',
        'illegal_content', 'self_harm', 'copyright', 'other'
    )),
    notes            text,
    evidence_urls    text[],
    status           text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'reviewing', 'actioned', 'dismissed')),
    reviewed_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    resolution_note  text,
    resolved_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now()
);

-- Prevent duplicate reports from the same user on the same entity. A partial
-- unique index (not a table UNIQUE constraint) because reporter_user_id is
-- nullable and multiple NULL (AI-flag) rows for the same entity must be
-- allowed — a plain UNIQUE treats NULLs as distinct already, but this makes
-- the intent explicit and only applies the constraint to real reporters.
CREATE UNIQUE INDEX idx_reports_reporter_entity_unique
    ON content_reports (reporter_user_id, entity_type, entity_id)
    WHERE reporter_user_id IS NOT NULL;

CREATE INDEX idx_reports_status   ON content_reports (tenant_id, status, created_at DESC);
CREATE INDEX idx_reports_entity   ON content_reports (entity_type, entity_id, status);
CREATE INDEX idx_reports_reporter ON content_reports (reporter_user_id);

COMMIT;
