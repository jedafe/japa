-- Section 24: Blog Extension
-- Blog series (multi-part posts), content review workflow,
-- featured/curated posts, and RSS feed support.

-- =====================
-- Blog Series
-- =====================
CREATE TABLE blog_series (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title           text NOT NULL,
    slug            text NOT NULL,
    description     text,
    cover_image_id  uuid REFERENCES media(id) ON DELETE SET NULL,
    author_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'archived')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_blog_series_tenant ON blog_series(tenant_id);
CREATE INDEX idx_blog_series_author ON blog_series(tenant_id, author_user_id);
CREATE INDEX idx_blog_series_slug ON blog_series(tenant_id, slug);

-- =====================
-- Blog Series ↔ Posts junction
-- =====================
CREATE TABLE blog_series_posts (
    series_id   uuid NOT NULL REFERENCES blog_series(id) ON DELETE CASCADE,
    content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    position    int NOT NULL DEFAULT 0,
    PRIMARY KEY(series_id, content_id)
);
CREATE INDEX idx_blog_series_posts_series ON blog_series_posts(series_id);
CREATE INDEX idx_blog_series_posts_content ON blog_series_posts(content_id);

-- =====================
-- Content Reviews (editorial workflow)
-- =====================
CREATE TABLE content_reviews (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id        uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    reviewer_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status            text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'changes_requested', 'rejected')),
    feedback          text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_content_reviews_content ON content_reviews(tenant_id, content_id);
CREATE INDEX idx_content_reviews_reviewer ON content_reviews(tenant_id, reviewer_user_id);
CREATE INDEX idx_content_reviews_status ON content_reviews(tenant_id, status) WHERE status = 'pending';

-- =====================
-- Content Featured (curated posts)
-- =====================
CREATE TABLE content_featured (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id      uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    featured_order  int NOT NULL DEFAULT 0,
    featured_until  timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, content_id)
);
CREATE INDEX idx_content_featured_tenant ON content_featured(tenant_id);
CREATE INDEX idx_content_featured_order ON content_featured(tenant_id, featured_order);
