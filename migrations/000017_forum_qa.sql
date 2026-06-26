-- Section 08: Forum Q&A / Solved
-- Nested threads with per-thread solved status, upvote/downvote voting,
-- tag system with moderators, polls, anonymous posting, bookmarks,
-- and reply view tracking.

-- =====================
-- Forum Threads
-- =====================
CREATE TABLE forum_threads (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id          uuid REFERENCES contents(id) ON DELETE SET NULL,
    space_id            uuid REFERENCES spaces(id) ON DELETE SET NULL,
    author_user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_reply_id     uuid REFERENCES forum_replies(id) ON DELETE SET NULL,
    title               text NOT NULL,
    slug                text NOT NULL,
    body                jsonb NOT NULL DEFAULT '[]'::jsonb,
    is_anonymous        boolean NOT NULL DEFAULT false,
    solved              boolean NOT NULL DEFAULT false,
    solved_at           timestamptz,
    solved_by_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    vote_score          int NOT NULL DEFAULT 0,
    reply_count         int NOT NULL DEFAULT 0,
    view_count          int NOT NULL DEFAULT 0,
    bookmark_count      int NOT NULL DEFAULT 0,
    edit_window_minutes int NOT NULL DEFAULT 30,
    is_edited           boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    deleted_at          timestamptz
);
CREATE INDEX idx_forum_threads_tenant ON forum_threads(tenant_id);
CREATE INDEX idx_forum_threads_space ON forum_threads(tenant_id, space_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_forum_threads_author ON forum_threads(tenant_id, author_user_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_forum_threads_slug ON forum_threads(tenant_id, slug) WHERE deleted_at IS NULL;

-- =====================
-- Forum Replies (nested)
-- =====================
CREATE TABLE forum_replies (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    thread_id       uuid NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    parent_reply_id uuid REFERENCES forum_replies(id) ON DELETE SET NULL,
    author_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body            jsonb NOT NULL DEFAULT '[]'::jsonb,
    vote_score      int NOT NULL DEFAULT 0,
    view_count      int NOT NULL DEFAULT 0,
    is_anonymous    boolean NOT NULL DEFAULT false,
    is_edited       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    deleted_at      timestamptz
);
CREATE INDEX idx_forum_replies_thread ON forum_replies(tenant_id, thread_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_forum_replies_parent ON forum_replies(tenant_id, parent_reply_id) WHERE deleted_at IS NULL;

-- =====================
-- Forum Votes (upvote/downvote)
-- =====================
CREATE TABLE forum_votes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type text NOT NULL CHECK (entity_type IN ('thread', 'reply')),
    entity_id   uuid NOT NULL,
    value       smallint NOT NULL CHECK (value IN (-1, 1)),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, entity_type, entity_id)
);
CREATE INDEX idx_forum_votes_entity ON forum_votes(tenant_id, entity_type, entity_id);
CREATE INDEX idx_forum_votes_user ON forum_votes(tenant_id, user_id);

-- =====================
-- Forum Polls
-- =====================
CREATE TABLE forum_polls (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id       uuid NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    question        text NOT NULL,
    is_multi_select boolean NOT NULL DEFAULT false,
    close_at        timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_forum_polls_thread ON forum_polls(thread_id);

-- =====================
-- Forum Poll Options
-- =====================
CREATE TABLE forum_poll_options (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id   uuid NOT NULL REFERENCES forum_polls(id) ON DELETE CASCADE,
    label     text NOT NULL,
    position  int NOT NULL DEFAULT 0
);
CREATE INDEX idx_forum_poll_options_poll ON forum_poll_options(poll_id);

-- =====================
-- Forum Poll Votes
-- =====================
CREATE TABLE forum_poll_votes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id    uuid NOT NULL REFERENCES forum_polls(id) ON DELETE CASCADE,
    option_id  uuid NOT NULL REFERENCES forum_poll_options(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(poll_id, user_id)
);
CREATE INDEX idx_forum_poll_votes_poll ON forum_poll_votes(poll_id);
CREATE INDEX idx_forum_poll_votes_user ON forum_poll_votes(user_id);

-- =====================
-- Forum Tags
-- =====================
CREATE TABLE forum_tags (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name                text NOT NULL,
    slug                text NOT NULL,
    description         text,
    rules               text,
    cover_image_id      uuid REFERENCES media(id) ON DELETE SET NULL,
    follower_count      int NOT NULL DEFAULT 0,
    moderator_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_forum_tags_tenant ON forum_tags(tenant_id);

-- =====================
-- Forum Thread ↔ Tag junction
-- =====================
CREATE TABLE forum_thread_tags (
    thread_id uuid NOT NULL REFERENCES forum_threads(id) ON DELETE CASCADE,
    tag_id    uuid NOT NULL REFERENCES forum_tags(id) ON DELETE CASCADE,
    PRIMARY KEY(thread_id, tag_id)
);
CREATE INDEX idx_forum_thread_tags_tag ON forum_thread_tags(tag_id);

-- =====================
-- Forum Bookmarks
-- =====================
CREATE TABLE forum_bookmarks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type text NOT NULL CHECK (entity_type IN ('thread', 'reply')),
    entity_id   uuid NOT NULL,
    folder      text NOT NULL DEFAULT 'default',
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, entity_type, entity_id)
);
CREATE INDEX idx_forum_bookmarks_user ON forum_bookmarks(tenant_id, user_id);
CREATE INDEX idx_forum_bookmarks_entity ON forum_bookmarks(tenant_id, entity_type, entity_id);

-- =====================
-- Reply Views
-- =====================
CREATE TABLE reply_views (
    reply_id   uuid NOT NULL REFERENCES forum_replies(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(reply_id, user_id)
);

-- =====================
-- Trigger: increment reply view_count on new reply_views row
-- =====================
CREATE OR REPLACE FUNCTION increment_reply_view_count()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE forum_replies SET view_count = view_count + 1 WHERE id = NEW.reply_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_reply_views_increment
    AFTER INSERT ON reply_views
    FOR EACH ROW
    EXECUTE FUNCTION increment_reply_view_count();
