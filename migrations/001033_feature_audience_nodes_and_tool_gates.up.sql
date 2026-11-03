-- 001033_feature_audience_nodes_and_tool_gates.up.sql
-- 1) Completes the feature.* permission-node family used as `perm:` audience
--    targets in feature_flags.config. feature.ads / feature.ai_agent /
--    feature.japabot exist as flag keys (features.Gate callsites) but had no
--    permission node, so they could never be referenced as audience grants.
--    Granted to the same member tiers as the other feature.* nodes.
-- 2) Sets required_permission='agent.use' on the two write tools that had a
--    NULL gate (comparison.promote, user.update_profile). Both are self-only
--    via the executor privacy gate; agent.use makes the admission gate
--    explicit instead of implicit.

BEGIN;

INSERT INTO permissions (id, key, description, created_at) VALUES
    (gen_random_uuid(), 'feature.ads',      'Audience gate for the ads surface (perm: audience in feature flags)', now()),
    (gen_random_uuid(), 'feature.ai_agent', 'Audience gate for the AI agent surface (perm: audience in feature flags)', now()),
    (gen_random_uuid(), 'feature.japabot',  'Audience gate for JapaBot surface (perm: audience in feature flags)', now())
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r, permissions p
WHERE r.slug IN ('member', 'pro_member', 'business_member')
  AND p.key IN ('feature.ads', 'feature.ai_agent', 'feature.japabot')
ON CONFLICT DO NOTHING;

UPDATE ai_tools
SET required_permission = 'agent.use', updated_at = now()
WHERE name IN ('comparison.promote', 'user.update_profile')
  AND required_permission IS NULL;

COMMIT;
