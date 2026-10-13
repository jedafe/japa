-- 000839_wiki_fields_role_grants_and_cfv_set_hitl.up.sql
--
-- Two fixes from the spaces/wiki/AI/custom-fields integration audit:
--
-- 1. pro_member and business_member were granted wiki.edit but not the
--    wiki.fields.* nodes — paid members can submit prose revisions yet
--    get ErrOpNotPermitted on every structured field edit while free
--    members can do both. Grant wiki.fields.edit and wiki.fields.propose
--    to both roles, matching the member-level grant.
--
-- 2. ai_tools custom_field_values.set had requires_hitl=false while
--    custom_field_values.bulk_set required review — trivially evadable
--    with N single calls. Field-value writes can also materialize
--    comparison items through trg_col_item_from_wiki_cfv (000786), so
--    un-reviewed agent writes flowed into the CoL calculator. Flip
--    requires_hitl to true for parity with bulk_set.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug IN ('pro_member', 'business_member')
  AND p.name IN ('wiki.fields.edit', 'wiki.fields.propose')
ON CONFLICT DO NOTHING;

UPDATE ai_tools
SET config = jsonb_set(config, '{requires_hitl}', 'true'::jsonb),
    updated_at = now()
WHERE name = 'custom_field_values.set';

COMMIT;
