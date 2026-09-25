-- 000556_jce_scores_unique_tenant.down.sql

BEGIN;

ALTER TABLE comparison_scores
    DROP CONSTRAINT IF EXISTS comparison_scores_tenant_content_item_factor_profile_key;

ALTER TABLE comparison_scores
    ADD CONSTRAINT comparison_scores_content_id_item_id_factor_id_profile_id_key
    UNIQUE (content_id, item_id, factor_id, profile_id);

COMMIT;
