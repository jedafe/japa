-- 000556_jce_scores_unique_tenant.up.sql
-- JCE Sub-spec 2/3: include tenant_id in the comparison_scores unique
-- constraint so the ON CONFLICT target is fully tenant-scoped.

BEGIN;

ALTER TABLE comparison_scores
    DROP CONSTRAINT IF EXISTS comparison_scores_content_id_item_id_factor_id_profile_id_key;

ALTER TABLE comparison_scores
    ADD CONSTRAINT comparison_scores_tenant_content_item_factor_profile_key
    UNIQUE (tenant_id, content_id, item_id, factor_id, profile_id);

COMMIT;
