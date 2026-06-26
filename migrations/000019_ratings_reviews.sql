-- Section 29: Universal Ratings & Reviews
-- Polymorphic ratings/reviews system shared across Organizations,
-- Universities/Mentors, Services, and any future rateable entity.
-- One review per user per entity; maintained aggregate rollup.

-- =====================
-- Reviews
-- =====================
CREATE TABLE reviews (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type       text NOT NULL,
    entity_id         uuid NOT NULL,
    reviewer_user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating            smallint NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title             text,
    body              text,
    status            text NOT NULL DEFAULT 'published' CHECK (status IN ('published', 'flagged', 'removed')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, entity_type, entity_id, reviewer_user_id)
);
CREATE INDEX idx_reviews_entity ON reviews(tenant_id, entity_type, entity_id);
CREATE INDEX idx_reviews_reviewer ON reviews(tenant_id, reviewer_user_id);
CREATE INDEX idx_reviews_status ON reviews(tenant_id, status) WHERE status = 'flagged';

-- =====================
-- Review Aggregates (maintained rollup)
-- =====================
CREATE TABLE review_aggregates (
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type     text NOT NULL,
    entity_id       uuid NOT NULL,
    review_count    int NOT NULL DEFAULT 0,
    average_rating  numeric(3,2) NOT NULL DEFAULT 0,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(tenant_id, entity_type, entity_id)
);
CREATE INDEX idx_review_aggregates_entity ON review_aggregates(tenant_id, entity_type, entity_id);
