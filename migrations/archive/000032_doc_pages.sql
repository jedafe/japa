-- 000032_doc_pages.sql
-- Documentation pages for the docs.japaconnect.com site.

CREATE TABLE doc_pages (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid NOT NULL REFERENCES tenants(id),
    title            text NOT NULL,
    slug             text NOT NULL,
    audience         text NOT NULL,  -- 'users', 'premium', 'organizations', 'moderators', 'api', 'admin', 'superadmin'
    content          jsonb NOT NULL DEFAULT '{}',
    ai_generated     boolean NOT NULL DEFAULT false,
    review_status    text NOT NULL DEFAULT 'draft',  -- 'draft', 'in_review', 'approved', 'published'
    last_reviewed_by uuid REFERENCES users(id),
    last_reviewed_at timestamptz,
    parent_id        uuid REFERENCES doc_pages(id),
    position         int NOT NULL DEFAULT 0,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, audience, slug)
);

CREATE INDEX idx_doc_pages_tenant ON doc_pages(tenant_id);
CREATE INDEX idx_doc_pages_audience ON doc_pages(tenant_id, audience);
CREATE INDEX idx_doc_pages_parent ON doc_pages(parent_id);
