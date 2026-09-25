-- 000433_drop_redundant_tenant_indexes.up.sql
-- ADR-004 Amendment: drop 12 standalone (tenant_id)-only indexes that are
-- redundant in a single-tenant deployment. With one tenant, every row carries
-- the same UUID, so these indexes have zero selectivity — the planner cannot
-- use them for filtering, yet they are written on every INSERT/UPDATE/DELETE
-- (write amplification) and consume disk + RAM for nothing.
--
-- Composite indexes that LEAD with tenant_id (e.g. (tenant_id, slug)) are
-- NOT dropped — they support real queries today and would matter if a second
-- tenant is ever provisioned.
--
-- If a second tenant is provisioned in the future, re-create these indexes
-- with a single migration (see the .down.sql file).

DROP INDEX IF EXISTS idx_subscription_plan_tracks_tenant;
DROP INDEX IF EXISTS idx_dm_requests_tenant;
DROP INDEX IF EXISTS idx_push_tokens_tenant;
DROP INDEX IF EXISTS idx_ai_providers_tenant_id;
DROP INDEX IF EXISTS idx_reaction_types_tenant;
DROP INDEX IF EXISTS idx_custom_field_entity_types_tenant;
DROP INDEX IF EXISTS idx_moderation_alerts_tenant;
DROP INDEX IF EXISTS idx_ad_networks_tenant;
DROP INDEX IF EXISTS idx_dm_messages_tenant;
DROP INDEX IF EXISTS idx_user_dm_settings_tenant;
DROP INDEX IF EXISTS idx_user_blocks_tenant;
DROP INDEX IF EXISTS idx_share_platforms_tenant;
