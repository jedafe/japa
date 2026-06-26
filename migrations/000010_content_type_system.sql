-- Section 14: Content Type System
-- Core content table with content_type discriminator, JSONB field_values,
-- revision history, slug redirects, polymorphic reactions, content scores
-- (pgvector-driven), JSON-LD templates, and space pinned posts.

-- 1. Core content table
CREATE TABLE contents (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    author_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    space_id          uuid REFERENCES spaces(id) ON DELETE SET NULL,
    content_type      text NOT NULL DEFAULT 'discussion',
    title             text NOT NULL,
    slug              text NOT NULL,
    body              text NOT NULL DEFAULT '',
    body_html         text NOT NULL DEFAULT '',
    field_values      jsonb NOT NULL DEFAULT '{}',
    content_hash      text,
    geo_space_id      uuid REFERENCES spaces(id) ON DELETE SET NULL,
    header_image_id   uuid,
    featured_image_id uuid,
    canonical_url     text,
    status            text NOT NULL DEFAULT 'draft',
    visibility        text NOT NULL DEFAULT 'public',
    is_locked         boolean NOT NULL DEFAULT false,
    is_pinned         boolean NOT NULL DEFAULT false,
    read_time_minutes int NOT NULL DEFAULT 0,
    view_count        int NOT NULL DEFAULT 0,
    comment_count     int NOT NULL DEFAULT 0,
    share_count       int NOT NULL DEFAULT 0,
    reshare_count     int NOT NULL DEFAULT 0,
    publish_at        timestamptz,
    published_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_contents_tenant_id ON contents(tenant_id);
CREATE INDEX idx_contents_author_id ON contents(author_id);
CREATE INDEX idx_contents_space_id ON contents(space_id);
CREATE INDEX idx_contents_content_type ON contents(content_type);
CREATE INDEX idx_contents_status ON contents(status);
CREATE INDEX idx_contents_slug ON contents(slug);
CREATE INDEX idx_contents_geo_space_id ON contents(geo_space_id);
CREATE INDEX idx_contents_publish_at ON contents(publish_at) WHERE publish_at IS NOT NULL;
CREATE UNIQUE INDEX idx_contents_tenant_slug ON contents(tenant_id, slug);
CREATE UNIQUE INDEX idx_contents_tenant_content_hash ON contents(tenant_id, content_hash) WHERE content_hash IS NOT NULL;

-- 2. Content revisions (one row per save, full history for rollback/audit)
CREATE TABLE content_revisions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id   uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    field_values jsonb NOT NULL DEFAULT '{}',
    title        text NOT NULL,
    body         text NOT NULL,
    body_html    text NOT NULL,
    edited_by    uuid NOT NULL REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_content_revisions_content_id ON content_revisions(content_id);

-- 3. Slug history for 301 redirects (SEO equity preservation)
CREATE TABLE slug_history (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL,
    entity_id   uuid NOT NULL,
    old_slug    text NOT NULL,
    new_slug    text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_slug_history_lookup ON slug_history(entity_type, old_slug);
CREATE INDEX idx_slug_history_entity ON slug_history(entity_type, entity_id);

-- 4. Reaction types (admin-configurable emoji set)
CREATE TABLE reaction_types (
    key        varchar(50) PRIMARY KEY,
    emoji      text NOT NULL,
    label      text NOT NULL,
    enabled    boolean NOT NULL DEFAULT true,
    position   int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- 5. Reactions (polymorphic — works for any content entity)
CREATE TABLE reactions (
    entity_type  text NOT NULL,
    entity_id    uuid NOT NULL,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reaction_key varchar(50) NOT NULL REFERENCES reaction_types(key) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entity_type, entity_id, user_id, reaction_key)
);

CREATE INDEX idx_reactions_entity ON reactions(entity_type, entity_id);
CREATE INDEX idx_reactions_user ON reactions(user_id);

-- 6. Content scores (drives Activity Feed sorting — Section 32)
CREATE TABLE content_scores (
    content_id          uuid PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
    vote_score          int NOT NULL DEFAULT 0,
    comment_count       int NOT NULL DEFAULT 0,
    view_count          int NOT NULL DEFAULT 0,
    bookmark_count      int NOT NULL DEFAULT 0,
    velocity_score      numeric NOT NULL DEFAULT 0,
    relevance_embedding vector(1536),
    last_scored_at      timestamptz NOT NULL DEFAULT now()
);

-- 7. JSON-LD templates per content type (render-time structured data)
CREATE TABLE content_type_jsonld_templates (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_type text NOT NULL,
    template     jsonb NOT NULL DEFAULT '{}',
    is_active    boolean NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, content_type)
);

CREATE INDEX idx_content_type_jsonld_templates_tenant ON content_type_jsonld_templates(tenant_id);

-- 8. Space pinned posts (deferred from Section 05 — both tables now exist)
CREATE TABLE space_pinned_posts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id   uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    pinned_by  uuid NOT NULL REFERENCES users(id),
    position   int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(space_id, content_id)
);

CREATE INDEX idx_space_pinned_posts_space ON space_pinned_posts(space_id);

-- Seed default reaction types
INSERT INTO reaction_types (key, emoji, label, position) VALUES
    ('like', '👍', 'Like', 0),
    ('support', '🤝', 'Support', 1),
    ('celebrate', '🎉', 'Celebrate', 2),
    ('insightful', '💡', 'Insightful', 3),
    ('grateful', '🙏', 'Grateful', 4),
    ('funny', '😄', 'Funny', 5);