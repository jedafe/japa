-- 000791_wiki_content_tools_admin_skip_hitl.up.sql
--
-- 1. Wiki content authoring for the agent/MCP surface (§80 revision queue,
--    §103 catalog). wiki_page.create gains a content-bearing path (page +
--    initial pending revision); wiki_page.submit_revision is the body-edit
--    tool; wiki_page.review_revision is the moderator approve/reject step.
--    All submissions remain pending revisions — agent content never goes
--    live unreviewed.
--
-- 2. Grant the Admin Group role (admin-group-role) the permission nodes
--    needed to actually use the agent/MCP surface: agent.use (the door
--    permission), agent.skip_hitl (bypass the HITL review queue — the
--    executor already honors the node; 000616 seeded it for
--    super_admin_role), the wiki tool nodes, docs.admin, and
--    learning.admin.manage. Previously the role held only 7 unrelated
--    nodes, so the HITL bypass alone would have been unusable.
--
-- Ref: .jeos/sections/80-destination-wiki.md
--      .jeos/sections/103-ai-agent-executor.md

BEGIN;

-- ---- 1. Wiki content tools -------------------------------------------------

-- wiki_page.create now accepts content/summary/position/space_slug in the
-- payload in addition to the provisioning fields — document them in the
-- advertised schema.
UPDATE ai_tools
SET description = 'Create a wiki page. Payload carrying "content" (TipTap JSON or HTML string) creates the page with an initial pending revision via the §80 revision queue; a payload without content provisions an empty page shell.',
    config = config || '{"description":"Create a wiki page; with \"content\" in the payload the page lands with an initial pending revision (§80), without it an empty shell is provisioned.","parameters":{"type":"object","properties":{"payload":{"type":"object","description":"Page fields: space_id or space_slug, slug, title, category, optional content (TipTap JSON object or HTML string), summary, position","additionalProperties":true}},"additionalProperties":true}}'::jsonb,
    updated_at = now()
WHERE name = 'wiki_page.create';

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('wiki_page.submit_revision', 'Submit a body edit and/or structured field ops to a wiki page as a revision (§80 revision queue). Subject to the wiki.community_edits setting and the wiki.min_reputation_to_edit reputation gate. Requires wiki.edit.', 'agent_community', '{"function_name":"wiki_page.submit_revision","description":"Submit a body edit and/or structured field ops to a wiki page as a revision (§80 revision queue). Subject to the wiki.community_edits setting and the wiki.min_reputation_to_edit reputation gate. Requires wiki.edit.","handler":"wiki_page.submit_revision","parameters":{"type":"object","properties":{"space_slug":{"type":"string","description":"Destination space slug"},"page_slug":{"type":"string","description":"Wiki page slug"},"content":{"description":"New page body — TipTap JSON object or HTML/markdown string"},"ops":{"type":"array","description":"§80 field ops — e.g. [{\"op\":\"set_value\",\"entity_type\":\"visa_route\",\"entity_id\":\"<uuid>\",\"field_key\":\"processing_time\",\"value\":12}]"},"summary":{"type":"string","description":"Edit summary shown in the review queue"}},"required":["space_slug","page_slug"],"additionalProperties":true},"writes_user_data":true,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'wiki.edit') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('wiki_page.review_revision', 'Approve or reject a pending wiki revision (moderator queue action; action: "approve" | "reject"). Requires wiki.review.', 'agent_community', '{"function_name":"wiki_page.review_revision","description":"Approve or reject a pending wiki revision (moderator queue action; action: \"approve\" | \"reject\"). Requires wiki.review.","handler":"wiki_page.review_revision","parameters":{"type":"object","properties":{"revision_id":{"type":"string","description":"Revision UUID"},"action":{"type":"string","description":"approve or reject","enum":["approve","reject"]},"notes":{"type":"string","description":"Optional review notes"}},"required":["revision_id","action"],"additionalProperties":true},"writes_user_data":true,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'wiki.review') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

-- ---- 2. Admin Group agent/wiki/docs/learn grants ---------------------------
--
-- agent.skip_hitl: bypass the HITL review queue (the executor already
-- honors the node; migration 000616 seeded it for super_admin_role).
-- agent.use: the door permission for the agent/MCP surface — without it
-- Admin Group members cannot call any agent tool at all.
-- wiki.* / wiki_page.manage: the wiki tools seeded above plus the existing
-- wiki_page catalog ops. wiki.fields.manage additionally auto-applies
-- field-ops revisions (§80 canManage path).
-- docs.admin / learning.admin.manage: the docs.* and learn.* write tools.

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'admin-group-role'
  AND p.key IN (
      'agent.use',
      'agent.skip_hitl',
      'wiki.edit',
      'wiki.review',
      'wiki.manage',
      'wiki_page.manage',
      'wiki.fields.manage',
      'docs.admin',
      'learning.admin.manage'
  )
ON CONFLICT DO NOTHING;

-- agent.draft_request gates content.create_draft; content.regenerate gates
-- content.regenerate_from_source — the two agent content paths that honor
-- agent.skip_hitl. Without them the group could not reach those tools.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'admin-group-role'
  AND p.key IN (
      'agent.draft_request',
      'content.regenerate'
  )
ON CONFLICT DO NOTHING;

-- ---- 3. content.create_draft description ------------------------------------
--
-- The draft publishes directly for actors holding agent.skip_hitl instead
-- of queueing a HITL review — reflect that in the advertised description.

UPDATE ai_tools
SET description = 'Create a draft content row for admin review. Use this when pre-search found nothing and the user lacks content.create permission. The author is the AI persona (not the calling user). A HITL review is created unless the caller holds agent.skip_hitl, in which case the draft publishes directly. Set guide_category when content_type is "guide".',
    config = config || '{"description":"Create a draft content row for admin review; publishes directly when the caller holds agent.skip_hitl."}'::jsonb,
    updated_at = now()
WHERE name = 'content.create_draft';

UPDATE ai_tools
SET description = 'Enqueue the content:regenerate_from_source worker to rewrite an existing contents row from a linked intelligence record extraction. Respects HITL, high-stakes, and manual-edit guards — callers holding agent.skip_hitl take the in-place update path with no review detour.',
    config = config || '{"description":"Rewrite an existing contents row from a linked intelligence record extraction; in-place for agent.skip_hitl holders, draft-and-review otherwise."}'::jsonb,
    updated_at = now()
WHERE name = 'content.regenerate_from_source';

COMMIT;
