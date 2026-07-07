-- Phase 11-15: Missing tables for content tags, shares, bookmarks, and comments.
-- Migration 000103: creates content_tags, content_shares, bookmarks, comments,
-- and comment_votes tables.

-- =====================
-- Content Tags (junction: content ↔ tags)
-- =====================
CREATE TABLE IF NOT EXISTS content_tags (
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    tag_id     uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (content_id, tag_id)
);
CREATE INDEX IF NOT EXISTS idx_content_tags_tag ON content_tags(tag_id);

-- =====================
-- Content Shares
-- =====================
CREATE TABLE IF NOT EXISTS content_shares (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform    text NOT NULL DEFAULT 'link',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_content_shares_content ON content_shares(tenant_id, content_id);
CREATE INDEX IF NOT EXISTS idx_content_shares_user ON content_shares(tenant_id, user_id);

-- =====================
-- Bookmarks (polymorphic — content, forum_thread, forum_reply)
-- =====================
CREATE TABLE IF NOT EXISTS bookmarks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type text NOT NULL CHECK (entity_type IN ('content', 'thread', 'reply')),
    entity_id   uuid NOT NULL,
    folder      text NOT NULL DEFAULT 'default',
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, entity_type, entity_id)
);
CREATE INDEX IF NOT EXISTS idx_bookmarks_user ON bookmarks(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_bookmarks_entity ON bookmarks(tenant_id, entity_type, entity_id);

-- =====================
-- Comments (unified comment system — Section 49)
-- =====================
CREATE TABLE IF NOT EXISTS comments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    author_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id       uuid REFERENCES comments(id) ON DELETE CASCADE,
    body            text NOT NULL,
    depth           int NOT NULL DEFAULT 0,
    vote_score      int NOT NULL DEFAULT 0,
    is_pinned       boolean NOT NULL DEFAULT false,
    is_edited       boolean NOT NULL DEFAULT false,
    edited_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE INDEX IF NOT EXISTS idx_comments_content ON comments(tenant_id, content_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_comments_parent ON comments(tenant_id, parent_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_comments_author ON comments(tenant_id, author_user_id) WHERE deleted_at IS NULL;

-- =====================
-- Comment Votes
-- =====================
CREATE TABLE IF NOT EXISTS comment_votes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    comment_id  uuid NOT NULL REFERENCES comments(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value       smallint NOT NULL CHECK (value IN (-1, 1)),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(comment_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_comment_votes_comment ON comment_votes(tenant_id, comment_id);
CREATE INDEX IF NOT EXISTS idx_comment_votes_user ON comment_votes(tenant_id, user_id);
