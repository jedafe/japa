-- 000862_agent_docs_review_tools.down.sql
-- Reverts: docs.organize disable, docs.approve/docs.publish removal,
-- taxonomy fields stripped from docs.create/docs.update parameter schemas.

BEGIN;

UPDATE ai_tools
SET is_enabled = false,
    updated_at = now()
WHERE name = 'docs.organize';

DELETE FROM ai_tools WHERE name IN ('docs.approve', 'docs.publish');

UPDATE ai_tools
SET config = jsonb_set(config, '{parameters}', '{
	"type": "object",
	"required": ["title", "slug", "audience"],
	"properties": {
		"slug": {"type": "string", "description": "URL slug"},
		"title": {"type": "string", "description": "Page title"},
		"content": {"type": "string", "description": "Page content (JSON)"},
		"audience": {"type": "string", "description": "Audience (public, premium, organizations, moderators, admin, superadmin)"},
		"position": {"type": "integer", "description": "Nav position — default 0"},
		"parent_id": {"type": "string", "description": "Parent page UUID (optional)"},
		"review_status": {"type": "string", "description": "Review status (draft, in_review, approved) — default draft. Cannot be published."}
	}
}'::jsonb), updated_at = now()
WHERE name = 'docs.create';

UPDATE ai_tools
SET config = jsonb_set(config, '{parameters}', '{
	"type": "object",
	"required": ["doc_id"],
	"properties": {
		"title": {"type": "string", "description": "New title (optional)"},
		"doc_id": {"type": "string", "description": "Doc page UUID"},
		"content": {"type": "string", "description": "New content (optional)"},
		"review_status": {"type": "string", "description": "New review status (draft, in_review, approved) — cannot be published"}
	}
}'::jsonb), updated_at = now()
WHERE name = 'docs.update';

COMMIT;
