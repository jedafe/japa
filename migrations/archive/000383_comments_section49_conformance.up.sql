-- 000383_comments_section49_conformance.up.sql
-- Brings the comments table and RBAC into conformance with Section 49
-- (Comments & Replies — Locked). Adds the is_anonymous, reply_count,
-- status, removed_by, removed_reason, author_profile_id columns omitted
-- from migration 000103; creates the content_thread_links cross-link
-- table that backs GET/POST /content/{id}/discussions; seeds the
-- comments.pin RBAC node (moderators + admin + super_admin).
--
-- Ref: japaconnect-engineering/sections/49-comments-replies.md

BEGIN;

-- =====================
-- comments: missing Section 49 columns
-- =====================
ALTER TABLE comments ADD COLUMN IF NOT EXISTS is_anonymous    boolean      NOT NULL DEFAULT false;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS reply_count     int          NOT NULL DEFAULT 0;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS author_profile_id uuid       REFERENCES professional_profiles(id) ON DELETE SET NULL;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS status          text         NOT NULL DEFAULT 'published'
    CHECK (status IN ('published', 'flagged', 'removed', 'spam'));
ALTER TABLE comments ADD COLUMN IF NOT EXISTS removed_by      uuid         REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS removed_reason  text;

-- Section 49 thread-sorted index (published, non-deleted, by depth + score).
CREATE INDEX IF NOT EXISTS idx_comments_thread
    ON comments (content_id, depth, vote_score DESC)
    WHERE status = 'published' AND deleted_at IS NULL;

-- =====================
-- content_thread_links: blog/non-forum content ↔ linked discussion threads
-- (Section 49 — Comment Thread Link Flow)
-- =====================
CREATE TABLE IF NOT EXISTS content_thread_links (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id   uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    thread_id    uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    link_count   int  NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(content_id, thread_id)
);
CREATE INDEX IF NOT EXISTS idx_content_thread_links_content
    ON content_thread_links (tenant_id, content_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_content_thread_links_thread
    ON content_thread_links (tenant_id, thread_id);

-- =====================
-- RBAC: seed comments.pin (Section 49 — moderators + content author).
-- The content-author path is resolved at request time in the handler;
-- the static grant covers moderators + admin + super_admin.
-- =====================
INSERT INTO permissions (key, description) VALUES
    ('comments.pin', 'Pin a comment as best answer (moderators + content author)')
ON CONFLICT (key) DO NOTHING;

-- Grant comments.pin to moderator, admin, and super_admin roles.
-- Role slugs follow the modern convention (see migrations 000334/000379).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'comments.pin'
WHERE r.slug IN ('moderator', 'admin', 'super_admin')
ON CONFLICT DO NOTHING;

COMMIT;
