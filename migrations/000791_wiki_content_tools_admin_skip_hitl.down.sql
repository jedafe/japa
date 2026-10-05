-- 000791_wiki_content_tools_admin_skip_hitl.down.sql
-- Reverts the wiki content tool seeds and the Admin Group agent.skip_hitl
-- grant from migration 000791.

BEGIN;

DELETE FROM ai_tools
WHERE name IN ('wiki_page.submit_revision', 'wiki_page.review_revision');

UPDATE ai_tools
SET description = 'Create a new Wiki Pages record.',
    config = config || '{"description":"Create a Wiki Pages record","parameters":{"type":"object","properties":{"payload":{"type":"object","description":"Tool record fields","additionalProperties":true}},"description":"Create a Wiki Pages record","additionalProperties":true}}'::jsonb,
    updated_at = now()
WHERE name = 'wiki_page.create';

DELETE FROM role_permissions
WHERE role_id = (SELECT id FROM user_roles WHERE slug = 'admin-group-role')
  AND permission_id IN (
      SELECT id FROM permissions WHERE key IN (
          'agent.use',
          'agent.skip_hitl',
          'wiki.edit',
          'wiki.review',
          'wiki.manage',
          'wiki_page.manage',
          'wiki.fields.manage',
          'docs.admin',
          'learning.admin.manage',
          'agent.draft_request',
          'content.regenerate'
      )
  );

UPDATE ai_tools
SET description = 'Create a draft content row for admin review. Use this when pre-search found nothing and the user lacks content.create permission. The author is the AI persona (not the calling user); status is always draft. A HITL review is created. Set guide_category when content_type is "guide".',
    config = config || '{"description":"Create a draft content row for admin review (agent.draft_request path). Author is the AI persona; status is forced to draft; a HITL review is created."}'::jsonb,
    updated_at = now()
WHERE name = 'content.create_draft';

UPDATE ai_tools
SET description = 'Enqueue the content:regenerate_from_source worker to rewrite an existing contents row from a linked intelligence record extraction. Respects HITL, high-stakes, and manual-edit guards.',
    config = config || '{"description":"Rewrite an existing contents row from a linked intelligence record extraction."}'::jsonb,
    updated_at = now()
WHERE name = 'content.regenerate_from_source';

COMMIT;
