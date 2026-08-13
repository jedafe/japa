-- 000433_drop_redundant_tenant_indexes.down.sql
-- Re-creates the 12 standalone (tenant_id)-only indexes dropped in the up
-- migration. Only needed if a second tenant is provisioned.

CREATE INDEX IF NOT EXISTS idx_subscription_plan_tracks_tenant ON subscription_plan_tracks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dm_requests_tenant ON dm_requests(tenant_id);
CREATE INDEX IF NOT EXISTS idx_push_tokens_tenant ON push_tokens(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ai_providers_tenant_id ON ai_providers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_reaction_types_tenant ON reaction_types(tenant_id);
CREATE INDEX IF NOT EXISTS idx_custom_field_entity_types_tenant ON custom_field_entity_types(tenant_id);
CREATE INDEX IF NOT EXISTS idx_moderation_alerts_tenant ON moderation_alerts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_ad_networks_tenant ON ad_networks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_dm_messages_tenant ON dm_messages(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_dm_settings_tenant ON user_dm_settings(tenant_id);
CREATE INDEX IF NOT EXISTS idx_user_blocks_tenant ON user_blocks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_share_platforms_tenant ON share_platforms(tenant_id);
