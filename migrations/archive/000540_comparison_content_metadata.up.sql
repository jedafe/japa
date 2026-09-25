-- Sub-spec 1: Japa Comparison Engine foundation schema.
-- Adds comparison_content_metadata, comparison_type_definitions.config,
-- comparison_items.item_ref, and backfills existing comparison contents.

BEGIN;

-- Metadata table for quick/detailed split and future Smart Compare / JapaScore / AI agent.
CREATE TABLE IF NOT EXISTS comparison_content_metadata (
    content_id        uuid PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    depth             text NOT NULL CHECK (depth IN ('quick','detailed')) DEFAULT 'quick',
    promoted_from_id  uuid REFERENCES contents(id) ON DELETE SET NULL,
    promoted_at       timestamptz,
    smart_mode        text CHECK (smart_mode IN ('none','eligibility','cost','pathway','ai_advisor')) DEFAULT 'none',
    score_version     int NOT NULL DEFAULT 1,
    config            jsonb NOT NULL DEFAULT '{}',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comparison_content_metadata_tenant_depth
    ON comparison_content_metadata(tenant_id, depth);

CREATE INDEX IF NOT EXISTS idx_comparison_content_metadata_promoted_from
    ON comparison_content_metadata(promoted_from_id)
    WHERE promoted_from_id IS NOT NULL;

-- Type-specific config for default dimensions, weights, smart defaults, eligibility rules.
ALTER TABLE comparison_type_definitions
    ADD COLUMN IF NOT EXISTS config jsonb NOT NULL DEFAULT '{}';

-- Polymorphic item reference for cross-domain / nested comparison items.
ALTER TABLE comparison_items
    ADD COLUMN IF NOT EXISTS item_ref jsonb NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_comparison_items_item_ref
    ON comparison_items USING GIN(item_ref)
    WHERE item_ref <> '{}'::jsonb;

-- Backfill existing comparison contents.
-- Body-less comparison rows are treated as quick (private, not indexable).
-- Comparison rows with a non-empty body are treated as detailed.
INSERT INTO comparison_content_metadata (
    content_id,
    tenant_id,
    depth,
    promoted_from_id,
    promoted_at,
    smart_mode,
    score_version,
    config
)
SELECT
    c.id,
    c.tenant_id,
    CASE
        WHEN c.body IS NOT NULL AND length(trim(c.body)) > 0 THEN 'detailed'
        ELSE 'quick'
    END AS depth,
    NULL,
    NULL,
    'none',
    1,
    '{}'
FROM contents c
WHERE c.content_type = 'comparison'
  AND NOT EXISTS (
      SELECT 1 FROM comparison_content_metadata m WHERE m.content_id = c.id
  );

-- Quick backfilled comparisons should be explicitly non-indexable.
UPDATE contents
SET seo_indexable = false
WHERE content_type = 'comparison'
  AND id IN (
      SELECT content_id FROM comparison_content_metadata WHERE depth = 'quick'
  );

COMMIT;
