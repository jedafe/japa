-- 000863_agent_wiki_discovery_tools.up.sql
-- Agent wiki discovery tools: let the agent find wiki pages (e.g. unfilled
-- stubs), read a page's approved content + bound field group, inspect the
-- revision queues, and read an entity's field schema/values — the read
-- surface that makes "find the stubs and fill them with real data" a
-- runnable agent workflow alongside the existing
-- create/submit_revision/review_revision write tools.
--
-- 1. wiki_page.list        — the admin page index (GET /admin/wiki);
--                            required_permission wiki.review, matching the
--                            REST gate on the review queue's page index.
-- 2. wiki_page.read        — the public page detail (GET /wiki/{s}/{p});
--                            ungated, like the REST route.
-- 3. wiki_page.revisions.list — revision discovery; entity-scoped and
--                            page-scoped listings mirror the public
--                            redacted-history routes (ungated), while the
--                            global queue requires wiki.review and the
--                            handler enforces that itself (the catalog's
--                            single required_permission column cannot
--                            express a conditional gate).
-- 4. wiki.entity_fields.get — an entity's assigned field groups with
--                            values + per-field permissions
--                            (GET /wiki/entities/{type}/{id}/fields);
--                            ungated, like the REST route.
--
-- All four are reads: writes_user_data=false, requires_hitl=false.
--
-- Ref: .jeos/sections/103-ai-agent-executor.md (Wiki tools)
--      .jeos/sections/80-destination-wiki.md

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('wiki_page.list', 'List wiki pages (the admin page index) with optional space, category, latest-revision status, active flag, and text filters. Returns each page''s slug, bound field group, latest revision id/status, and excerpt. Requires wiki.review.', 'agent_community', '{"function_name":"wiki_page.list","description":"List wiki pages (the admin page index) with optional space, category, latest-revision status, active flag, and text filters. Returns each page''s slug, bound field group, latest revision id/status, and excerpt. Requires wiki.review.","handler":"wiki_page.list","parameters":{"type":"object","properties":{"space_slug":{"type":"string","description":"Destination space slug — scopes the listing to one space"},"category":{"type":"string","description":"Page category filter"},"status":{"type":"string","description":"Latest revision status filter","enum":["pending","approved","rejected"]},"q":{"type":"string","description":"Search term matched against page title/slug"},"is_active":{"type":"boolean","description":"Filter by page visibility (false surfaces disabled stubs)"},"page":{"type":"integer","description":"Page number (default 1)"},"limit":{"type":"integer","description":"Page size (default 20, max 200)"}},"additionalProperties":true},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, 'wiki.review') ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('wiki_page.read', 'Read a wiki page by space and page slug: title, category, active/locked flags, current approved content (TipTap JSON), and the bound field group''s schema with current values. Public read — no permission required.', 'agent_community', '{"function_name":"wiki_page.read","description":"Read a wiki page by space and page slug: title, category, active/locked flags, current approved content (TipTap JSON), and the bound field group''s schema with current values. Public read — no permission required.","handler":"wiki_page.read","parameters":{"type":"object","properties":{"space_slug":{"type":"string","description":"Destination space slug"},"page_slug":{"type":"string","description":"Wiki page slug"}},"required":["space_slug","page_slug"],"additionalProperties":true},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, NULL) ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('wiki_page.revisions.list', 'List wiki revisions in three scopes: pass entity_type+entity_id for field-ops revisions targeting an entity, space_slug+page_slug for one page''s history (bodies redacted for non-reviewers), or neither for the global review queue — the global queue requires wiki.review.', 'agent_community', '{"function_name":"wiki_page.revisions.list","description":"List wiki revisions in three scopes: pass entity_type+entity_id for field-ops revisions targeting an entity, space_slug+page_slug for one page''s history (bodies redacted for non-reviewers), or neither for the global review queue — the global queue requires wiki.review.","handler":"wiki_page.revisions.list","parameters":{"type":"object","properties":{"entity_type":{"type":"string","description":"Entity type — with entity_id, lists field-ops revisions for the entity"},"entity_id":{"type":"string","description":"Entity UUID — required together with entity_type"},"space_slug":{"type":"string","description":"Destination space slug — with page_slug, lists the page''s revision history"},"page_slug":{"type":"string","description":"Wiki page slug"},"status":{"type":"string","description":"Status filter (global queue scope)","enum":["pending","approved","rejected"]},"kind":{"type":"string","description":"Revision kind filter (global queue scope)","enum":["content","fields","mixed"]},"q":{"type":"string","description":"Search term against summary/submitter (global queue scope)"},"page":{"type":"integer","description":"Page number (default 1)"},"limit":{"type":"integer","description":"Page size (default 20)"}},"additionalProperties":true},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, NULL) ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES ('wiki.entity_fields.get', 'Read an entity''s assigned wiki field groups: field definitions (key, type, options, section), current values, and the caller''s per-field can_edit flags — the schema a set_value op may fill via wiki_page.submit_revision. Public read — no permission required.', 'agent_community', '{"function_name":"wiki.entity_fields.get","description":"Read an entity''s assigned wiki field groups: field definitions (key, type, options, section), current values, and the caller''s per-field can_edit flags — the schema a set_value op may fill via wiki_page.submit_revision. Public read — no permission required.","handler":"wiki.entity_fields.get","parameters":{"type":"object","properties":{"entity_type":{"type":"string","description":"Entity type (the page''s data_entity_type — e.g. space)"},"entity_id":{"type":"string","description":"Entity UUID (the page''s bound entity — e.g. space_id)"}},"required":["entity_type","entity_id"],"additionalProperties":true},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb, true, NULL) ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, category = EXCLUDED.category, config = EXCLUDED.config, is_enabled = EXCLUDED.is_enabled, required_permission = EXCLUDED.required_permission;

COMMIT;
