-- 000862_agent_docs_review_tools.up.sql
-- Agent docs tools: reviewer transitions + taxonomy fields + organize enable.
--
-- 1. docs.organize: §44 amendment 2 added parent_id/position to
--    UpdateDocPage (with hierarchy validation), so the DocsOrganize bridge
--    resolves through the service — enable the seeded row.
--
-- 2. docs.approve / docs.publish: dedicated reviewer-transition tools, the
--    agent equivalents of POST /admin/docs/pages/{id}/approve|publish.
--    The §44 status machine keeps its invariants at the service layer —
--    publish requires current status 'approved'. Required permission
--    docs.admin, matching the rest of the docs.* catalog. requires_hitl is
--    false: the caller is already permission-gated and the action mirrors
--    an explicit reviewer click (wiki_page.review_revision precedent).
--
-- 3. docs.create / docs.update parameter schemas gain the doc_pages
--    taxonomy columns (category, subcategory) and docs.update gains
--    parent_id/position (preserve-on-absent semantics). The stale audience
--    enum in docs.create's description is corrected to the real
--    ValidAudiences set.
--
-- Ref: .jeos/sections/103-ai-agent-executor.md (Docs tools)
--      .jeos/sections/44-documentation-system.md

BEGIN;

-- ---- 1. Enable docs.organize ----------------------------------------------

UPDATE ai_tools
SET is_enabled = true,
    description = 'Move/reorder documentation pages in the nav tree (parent_id + position).',
    updated_at = now()
WHERE name = 'docs.organize';

-- ---- 2. Reviewer-transition tools ------------------------------------------

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('docs.approve', 'Approve a documentation page awaiting review (sets review_status=approved, records you as reviewer).', 'agent_docs', '{"function_name":"docs.approve","description":"Approve a documentation page awaiting review (sets review_status=approved, records you as reviewer).","handler":"docs.approve","parameters":{"type":"object","properties":{"doc_id":{"type":"string","description":"Doc page UUID"}},"required":["doc_id"],"additionalProperties":false},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'docs.admin') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;
INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('docs.publish', 'Publish an approved documentation page — the page goes live (requires review_status=approved).', 'agent_docs', '{"function_name":"docs.publish","description":"Publish an approved documentation page — the page goes live (requires review_status=approved).","handler":"docs.publish","parameters":{"type":"object","properties":{"doc_id":{"type":"string","description":"Doc page UUID"}},"required":["doc_id"],"additionalProperties":false},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'docs.admin') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

-- ---- 3. Taxonomy + hierarchy fields on create/update ------------------------

UPDATE ai_tools
SET config = jsonb_set(config, '{parameters}', '{
	"type": "object",
	"required": ["title", "slug", "audience"],
	"properties": {
		"title": {"type": "string", "description": "Page title"},
		"slug": {"type": "string", "description": "URL slug"},
		"audience": {"type": "string", "description": "Audience (users, premium, organizations, moderators, api, admin, superadmin)"},
		"content": {"type": "string", "description": "Page content (JSON)"},
		"parent_id": {"type": "string", "description": "Parent page UUID (optional)"},
		"position": {"type": "integer", "description": "Nav position — default 0"},
		"category": {"type": "string", "description": "Top-level taxonomy category (optional)"},
		"subcategory": {"type": "string", "description": "Subcategory under category (optional)"},
		"review_status": {"type": "string", "description": "Review status (draft, in_review) — default draft. Approve/publish via docs.approve and docs.publish."}
	}
}'::jsonb), updated_at = now()
WHERE name = 'docs.create';

UPDATE ai_tools
SET config = jsonb_set(config, '{parameters}', '{
	"type": "object",
	"required": ["doc_id"],
	"properties": {
		"doc_id": {"type": "string", "description": "Doc page UUID"},
		"title": {"type": "string", "description": "New title (optional)"},
		"content": {"type": "string", "description": "New content (optional)"},
		"review_status": {"type": "string", "description": "New review status (draft, in_review) — approve/publish via docs.approve and docs.publish."},
		"parent_id": {"type": "string", "description": "Move under a parent page UUID; empty string = top level (optional)"},
		"position": {"type": "integer", "description": "New nav position (optional)"},
		"category": {"type": "string", "description": "New category; empty string clears (optional)"},
		"subcategory": {"type": "string", "description": "New subcategory; empty string clears (optional)"}
	}
}'::jsonb), updated_at = now()
WHERE name = 'docs.update';

COMMIT;
