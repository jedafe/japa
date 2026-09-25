BEGIN;

CREATE TABLE source_collections (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug        text NOT NULL,
    name        text NOT NULL,
    description text,
    collection_type text NOT NULL CHECK (collection_type IN ('manual', 'query')),
    query_json  jsonb,
    sort        text,
    "limit"     int NOT NULL DEFAULT 10,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE TABLE source_collection_items (
    collection_id   uuid NOT NULL REFERENCES source_collections(id) ON DELETE CASCADE,
    source_id       uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
    position        int NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, source_id)
);

CREATE INDEX idx_source_collections_tenant ON source_collections(tenant_id, collection_type);
CREATE INDEX idx_source_collection_items_collection ON source_collection_items(collection_id, position);

-- Seed default collections.
INSERT INTO source_collections (tenant_id, slug, name, description, collection_type, query_json, sort, "limit")
SELECT id, 'trending', 'Trending', 'Sources gaining traction', 'query', '{"sort": "trending"}', 'trending', 10
FROM tenants
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO source_collections (tenant_id, slug, name, description, collection_type, query_json, sort, "limit")
SELECT id, 'recently-added', 'Recently added', 'Newly added sources', 'query', '{"sort": "recent"}', 'recent', 10
FROM tenants
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO source_collections (tenant_id, slug, name, description, collection_type, query_json, sort, "limit")
SELECT id, 'top-video-sources', 'Top video sources', 'Popular video sources', 'query', '{"category": "video", "sort": "popular"}', 'popular', 10
FROM tenants
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO source_collections (tenant_id, slug, name, description, collection_type, query_json, sort, "limit")
SELECT id, 'rising', 'Rising', 'Sources trending up this week', 'query', '{"sort": "trending", "created_after": "7d"}', 'trending', 10
FROM tenants
ON CONFLICT (tenant_id, slug) DO NOTHING;

COMMIT;
