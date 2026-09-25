-- 000405_seed_ai_agent_phase9_tools.up.sql
-- Renumbered from 000400 — it collided with 000400_fix_digest_rbac_admin_grant
-- (both committed in the same commit); 000405 is the first free slot.
-- Idempotent (ON CONFLICT DO UPDATE) — safe to re-apply on databases that
-- ran it under the old filename.
-- Section 103 (AI Agent Executor) — Phase 9 Extended Tool Catalog.
--
-- Seeds the Phase 9 extended tool catalog into the ai_tools table. Each
-- row's config JSONB contains the function schema (parameters), the
-- handler reference string (the Go handler registered in
-- ToolHandlerRegistry via RegisterPhase9Tools), and the behavioral flags
-- (requires_hitl, reads_user_data, writes_user_data, is_blocked) that
-- drive the executor pipeline.
--
-- The required_permission column holds the RBAC node the executor's RBAC
-- gate checks (Phase 3, executor.go step 3). Tools with NULL
-- required_permission are available to every user who holds agent.use.
--
-- Tools are grouped by domain:
--   * Spaces     (tasks 9.01) — agent_spaces
--   * Learning   (tasks 9.02) — agent_learning
--   * Docs       (tasks 9.03) — agent_docs
--   * Tools app  (tasks 9.04) — agent_tools  (DISABLED — §78 discrepancy)
--   * Menus      (tasks 9.05) — agent_menus
--   * User       (tasks 9.06) — agent_user
--   * Custom fields (9.07)    — agent_custom_fields
--   * Content    (tasks 9.08) — agent_content
--
-- The tools-app tools (tasks 9.04) are seeded with is_enabled = false
-- because the backing 'tools' table does not exist in the schema yet
-- (§78 describes apps/tools as a frontend surface with no unified
-- backend CRUD entity). The handlers are registered as gracefully-
-- degrading stubs (see tools_tools.go). When a tools table is added in
-- a future migration, these rows can be flipped to is_enabled = true.
--
-- docs.organize (task 9.03e) is also seeded with is_enabled = false
-- because the docs service does not expose a parent_id/position update
-- query (no UpdateDocPageParent/UpdateDocPagePosition SQL query exists).
-- The handler is registered and returns "not configured" when called.
-- When the backing query is added, flip is_enabled to true.
--
-- Permission nodes used (all pre-seeded, per Rule: do not invent nodes):
--   spaces.create, spaces.edit
--   learning.admin.manage
--   docs.admin, docs.manage
--   menus.manage
--   custom_fields.field.create, custom_fields.field.edit,
--   custom_fields.group.create
--   content.delete, content.publish
--   (user.read_profile / user.update_profile / user.read_own_fields are
--    self-scoped — NULL required_permission; the handler enforces self.)
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Tool Catalog Expansion)
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 9, tasks 9.01-9.08)
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- =====================================================================
-- Space tools (tasks 9.01)
-- =====================================================================

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'spaces.find',
 'Find a space by city name, geo code, or keywords. Returns the first match (city/geo) or all keyword matches.',
 'agent_spaces',
 '{
    "function_name": "spaces.find",
    "description": "Find a space by city name, geo code, or keywords.",
    "parameters": {
        "type": "object",
        "properties": {
            "city_name": { "type": "string", "description": "City name to look up" },
            "geo_code": { "type": "string", "description": "Country geo code (ISO 2-letter)" },
            "keywords": { "type": "array", "items": { "type": "string" }, "description": "Keywords to match" }
        }
    },
    "handler": "SpacesFind",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'spaces.read',
 'Read a single space by ID, including its name, slug, level, member/post/follower counts, visibility, permissions, and status.',
 'agent_spaces',
 '{
    "function_name": "spaces.read",
    "description": "Read a single space by ID.",
    "parameters": {
        "type": "object",
        "properties": {
            "space_id": { "type": "string", "description": "Space UUID" }
        },
        "required": ["space_id"]
    },
    "handler": "SpacesRead",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'spaces.list',
 'List all spaces visible to the calling user (viewer-scoped). Returns id, name, slug, level, counts, visibility, and status.',
 'agent_spaces',
 '{
    "function_name": "spaces.list",
    "description": "List spaces visible to the calling user.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "SpacesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'spaces.create',
 'Create a new space. Requires spaces.create permission. HITL-queued — a human reviewer approves before the space goes live.',
 'agent_spaces',
 '{
    "function_name": "spaces.create",
    "description": "Create a new space.",
    "parameters": {
        "type": "object",
        "properties": {
            "parent_id": { "type": "string", "description": "Parent space UUID (optional)" },
            "space_type_id": { "type": "string", "description": "Space type UUID (optional)" },
            "name": { "type": "string", "description": "Space name" },
            "slug": { "type": "string", "description": "URL slug" },
            "description": { "type": "string", "description": "Description (optional)" },
            "geo_code": { "type": "string", "description": "Country geo code (optional)" },
            "level": { "type": "string", "description": "Level (country, state, city, community, university)" },
            "visibility": { "type": "string", "description": "Visibility (public, private) — default public" },
            "posting_perm": { "type": "string", "description": "Posting permission (members, mods, admins) — default members" },
            "replying_perm": { "type": "string", "description": "Replying permission — default members" },
            "commenting_perm": { "type": "string", "description": "Commenting permission — default members" },
            "joining_perm": { "type": "string", "description": "Joining permission (open, approval, invite) — default open" },
            "status": { "type": "string", "description": "Status (active, dormant) — default active" }
        },
        "required": ["name", "slug", "level"]
    },
    "handler": "SpacesCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'spaces.create'),

('00000000-0000-0000-0000-000000000001', 'spaces.update',
 'Update an existing space. Requires spaces.edit permission. HITL-queued.',
 'agent_spaces',
 '{
    "function_name": "spaces.update",
    "description": "Update an existing space.",
    "parameters": {
        "type": "object",
        "properties": {
            "space_id": { "type": "string", "description": "Space UUID to update" },
            "name": { "type": "string", "description": "New name" },
            "description": { "type": "string", "description": "New description (optional)" },
            "geo_code": { "type": "string", "description": "New geo code (optional)" },
            "visibility": { "type": "string", "description": "New visibility (optional)" },
            "posting_perm": { "type": "string", "description": "New posting permission (optional)" },
            "replying_perm": { "type": "string", "description": "New replying permission (optional)" },
            "commenting_perm": { "type": "string", "description": "New commenting permission (optional)" },
            "joining_perm": { "type": "string", "description": "New joining permission (optional)" },
            "status": { "type": "string", "description": "New status (optional)" }
        },
        "required": ["space_id", "name"]
    },
    "handler": "SpacesUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'spaces.edit'),

-- =====================================================================
-- Learning tools (tasks 9.02)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'learn.tracks.list',
 'List all learning tracks for the tenant (admin view, including drafts).',
 'agent_learning',
 '{
    "function_name": "learn.tracks.list",
    "description": "List all learning tracks.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "LearnTracksList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'learn.tracks.create',
 'Create a learning track. Requires learning.admin.manage. HITL-queued.',
 'agent_learning',
 '{
    "function_name": "learn.tracks.create",
    "description": "Create a learning track.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Track title" },
            "slug": { "type": "string", "description": "URL slug" },
            "description": { "type": "string", "description": "Description (optional)" },
            "difficulty": { "type": "string", "description": "Difficulty (beginner, intermediate, advanced) — default beginner" },
            "cover_media_id": { "type": "string", "description": "Cover media UUID (optional)" },
            "is_published": { "type": "boolean", "description": "Published state — default false" },
            "sort_order": { "type": "integer", "description": "Sort order — default 0" },
            "category": { "type": "string", "description": "Category (optional)" },
            "subscription_eligible": { "type": "boolean", "description": "Subscription eligibility — default false" }
        },
        "required": ["title", "slug"]
    },
    "handler": "LearnTracksCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'learning.admin.manage'),

('00000000-0000-0000-0000-000000000001', 'learn.tracks.update',
 'Update a learning track. Requires learning.admin.manage. HITL-queued.',
 'agent_learning',
 '{
    "function_name": "learn.tracks.update",
    "description": "Update a learning track.",
    "parameters": {
        "type": "object",
        "properties": {
            "track_id": { "type": "string", "description": "Track UUID" },
            "title": { "type": "string", "description": "New title" },
            "description": { "type": "string", "description": "New description (optional)" },
            "difficulty": { "type": "string", "description": "New difficulty (optional)" },
            "cover_media_id": { "type": "string", "description": "New cover media UUID (optional)" },
            "is_published": { "type": "boolean", "description": "New published state (optional)" },
            "sort_order": { "type": "integer", "description": "New sort order (optional)" },
            "category": { "type": "string", "description": "New category (optional)" },
            "subscription_eligible": { "type": "boolean", "description": "New subscription eligibility (optional)" }
        },
        "required": ["track_id", "title"]
    },
    "handler": "LearnTracksUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'learning.admin.manage'),

('00000000-0000-0000-0000-000000000001', 'learn.lessons.list',
 'List all lessons for the tenant (admin view, including drafts).',
 'agent_learning',
 '{
    "function_name": "learn.lessons.list",
    "description": "List all lessons.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "LearnLessonsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'learn.lessons.create',
 'Create a lesson in a module. Requires learning.admin.manage. HITL-queued. module_id is required — lessons attach to modules.',
 'agent_learning',
 '{
    "function_name": "learn.lessons.create",
    "description": "Create a lesson in a module.",
    "parameters": {
        "type": "object",
        "properties": {
            "module_id": { "type": "string", "description": "Module UUID (required — lessons attach to modules)" },
            "title": { "type": "string", "description": "Lesson title" },
            "slug": { "type": "string", "description": "URL slug" },
            "lesson_type": { "type": "string", "description": "Lesson type (text, video, quiz) — default text" },
            "content": { "type": "string", "description": "Lesson content (optional)" },
            "media_id": { "type": "string", "description": "Media UUID (optional)" },
            "duration_minutes": { "type": "integer", "description": "Duration in minutes (optional)" },
            "sort_order": { "type": "integer", "description": "Sort order — default 0" },
            "is_published": { "type": "boolean", "description": "Published state — default false" },
            "is_preview": { "type": "boolean", "description": "Preview (free) state — default false" }
        },
        "required": ["module_id", "title", "slug"]
    },
    "handler": "LearnLessonsCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'learning.admin.manage'),

('00000000-0000-0000-0000-000000000001', 'learn.lessons.update',
 'Update a lesson. Requires learning.admin.manage. HITL-queued.',
 'agent_learning',
 '{
    "function_name": "learn.lessons.update",
    "description": "Update a lesson.",
    "parameters": {
        "type": "object",
        "properties": {
            "lesson_id": { "type": "string", "description": "Lesson UUID" },
            "title": { "type": "string", "description": "New title" },
            "lesson_type": { "type": "string", "description": "New lesson type (optional)" },
            "content": { "type": "string", "description": "New content (optional)" },
            "media_id": { "type": "string", "description": "New media UUID (optional)" },
            "duration_minutes": { "type": "integer", "description": "New duration (optional)" },
            "sort_order": { "type": "integer", "description": "New sort order (optional)" },
            "is_published": { "type": "boolean", "description": "New published state (optional)" },
            "is_preview": { "type": "boolean", "description": "New preview state (optional)" }
        },
        "required": ["lesson_id", "title"]
    },
    "handler": "LearnLessonsUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'learning.admin.manage'),

('00000000-0000-0000-0000-000000000001', 'learn.lessons.reorder',
 'Reorder lessons by updating their sort_order values in bulk. Requires learning.admin.manage.',
 'agent_learning',
 '{
    "function_name": "learn.lessons.reorder",
    "description": "Reorder lessons in bulk.",
    "parameters": {
        "type": "object",
        "properties": {
            "entries": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "lesson_id": { "type": "string", "description": "Lesson UUID" },
                        "sort_order": { "type": "integer", "description": "New sort order" }
                    },
                    "required": ["lesson_id", "sort_order"]
                },
                "description": "Reorder entries"
            }
        },
        "required": ["entries"]
    },
    "handler": "LearnLessonsReorder",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'learning.admin.manage'),

-- =====================================================================
-- Docs tools (tasks 9.03)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'docs.list',
 'List all documentation pages for the tenant (any status, any audience — admin view).',
 'agent_docs',
 '{
    "function_name": "docs.list",
    "description": "List all documentation pages.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "DocsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'docs.read',
 'Read a single documentation page by ID, including its content, audience, review status, and nav position.',
 'agent_docs',
 '{
    "function_name": "docs.read",
    "description": "Read a documentation page by ID.",
    "parameters": {
        "type": "object",
        "properties": {
            "doc_id": { "type": "string", "description": "Doc page UUID" }
        },
        "required": ["doc_id"]
    },
    "handler": "DocsRead",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'docs.create',
 'Create a documentation page. Requires docs.admin. HITL-queued. The agent never publishes directly — review_status defaults to draft.',
 'agent_docs',
 '{
    "function_name": "docs.create",
    "description": "Create a documentation page.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Page title" },
            "slug": { "type": "string", "description": "URL slug" },
            "audience": { "type": "string", "description": "Audience (public, premium, organizations, moderators, admin, superadmin)" },
            "content": { "type": "string", "description": "Page content (JSON)" },
            "review_status": { "type": "string", "description": "Review status (draft, in_review, approved) — default draft. Cannot be published." },
            "parent_id": { "type": "string", "description": "Parent page UUID (optional)" },
            "position": { "type": "integer", "description": "Nav position — default 0" }
        },
        "required": ["title", "slug", "audience"]
    },
    "handler": "DocsCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'docs.admin'),

('00000000-0000-0000-0000-000000000001', 'docs.update',
 'Update a documentation page. Requires docs.admin. The agent cannot publish directly — review_status published is rejected.',
 'agent_docs',
 '{
    "function_name": "docs.update",
    "description": "Update a documentation page.",
    "parameters": {
        "type": "object",
        "properties": {
            "doc_id": { "type": "string", "description": "Doc page UUID" },
            "title": { "type": "string", "description": "New title (optional)" },
            "content": { "type": "string", "description": "New content (optional)" },
            "review_status": { "type": "string", "description": "New review status (draft, in_review, approved) — cannot be published" }
        },
        "required": ["doc_id"]
    },
    "handler": "DocsUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'docs.admin'),

('00000000-0000-0000-0000-000000000001', 'docs.organize',
 'Reorganize doc pages in the nav tree by updating parent_id and position. Requires docs.admin. DISABLED — the docs service does not expose a parent_id/position update query (no UpdateDocPageParent/UpdateDocPagePosition SQL query exists). The handler is registered as a gracefully-degrading stub; when the backing query is added, flip is_enabled to true.',
 'agent_docs',
 '{
    "function_name": "docs.organize",
    "description": "Reorganize doc pages in the nav tree.",
    "parameters": {
        "type": "object",
        "properties": {
            "entries": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "doc_id": { "type": "string", "description": "Doc page UUID" },
                        "parent_id": { "type": "string", "description": "New parent UUID (optional, null = top level)" },
                        "position": { "type": "integer", "description": "New nav position" }
                    },
                    "required": ["doc_id", "position"]
                },
                "description": "Organize entries"
            }
        },
        "required": ["entries"]
    },
    "handler": "DocsOrganize",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 false, 'docs.admin'),

-- =====================================================================
-- Tools-app tools (tasks 9.04) — DISABLED (§78 discrepancy)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'tools.list',
 'List available tools/features. DISABLED — the backing tools table does not exist yet (§78 discrepancy).',
 'agent_tools',
 '{
    "function_name": "tools.list",
    "description": "List available tools/features.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "ToolsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 false, NULL),

('00000000-0000-0000-0000-000000000001', 'tools.read',
 'Read a tool details. DISABLED — the backing tools table does not exist yet (§78 discrepancy).',
 'agent_tools',
 '{
    "function_name": "tools.read",
    "description": "Read a tool details.",
    "parameters": {
        "type": "object",
        "properties": {
            "tool_id": { "type": "string", "description": "Tool UUID" }
        },
        "required": ["tool_id"]
    },
    "handler": "ToolsRead",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 false, NULL),

('00000000-0000-0000-0000-000000000001', 'tools.create',
 'Create a new tool page. DISABLED — the backing tools table does not exist yet (§78 discrepancy).',
 'agent_tools',
 '{
    "function_name": "tools.create",
    "description": "Create a new tool page.",
    "parameters": {
        "type": "object",
        "properties": {
            "name": { "type": "string", "description": "Tool name" },
            "slug": { "type": "string", "description": "URL slug" },
            "description": { "type": "string", "description": "Description (optional)" },
            "category": { "type": "string", "description": "Category (optional)" }
        },
        "required": ["name", "slug"]
    },
    "handler": "ToolsCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 false, NULL),

('00000000-0000-0000-0000-000000000001', 'tools.update',
 'Update a tool. DISABLED — the backing tools table does not exist yet (§78 discrepancy).',
 'agent_tools',
 '{
    "function_name": "tools.update",
    "description": "Update a tool.",
    "parameters": {
        "type": "object",
        "properties": {
            "tool_id": { "type": "string", "description": "Tool UUID" },
            "name": { "type": "string", "description": "New name (optional)" },
            "description": { "type": "string", "description": "New description (optional)" }
        },
        "required": ["tool_id"]
    },
    "handler": "ToolsUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 false, NULL),

('00000000-0000-0000-0000-000000000001', 'tools.assign_geo',
 'Geo-scope a tool to a country/region. DISABLED — the backing tools table does not exist yet (§78 discrepancy).',
 'agent_tools',
 '{
    "function_name": "tools.assign_geo",
    "description": "Geo-scope a tool to a country/region.",
    "parameters": {
        "type": "object",
        "properties": {
            "tool_id": { "type": "string", "description": "Tool UUID" },
            "geo_code": { "type": "string", "description": "Country geo code" }
        },
        "required": ["tool_id", "geo_code"]
    },
    "handler": "ToolsAssignGeo",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 false, NULL),

-- =====================================================================
-- Menu tools (tasks 9.05)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'menu.list',
 'List menus with optional placement/status filters. Returns menus with item counts.',
 'agent_menus',
 '{
    "function_name": "menu.list",
    "description": "List menus with optional filters.",
    "parameters": {
        "type": "object",
        "properties": {
            "placement": { "type": "string", "description": "Filter by placement (optional)" },
            "status": { "type": "string", "description": "Filter by status (optional)" },
            "q": { "type": "string", "description": "Search query (optional)" },
            "limit": { "type": "integer", "description": "Max results (default 20, max 100)" },
            "offset": { "type": "integer", "description": "Pagination offset (default 0)" }
        }
    },
    "handler": "MenuList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'menu.create',
 'Create a menu item. Requires menus.manage. HITL-queued.',
 'agent_menus',
 '{
    "function_name": "menu.create",
    "description": "Create a menu item.",
    "parameters": {
        "type": "object",
        "properties": {
            "menu_id": { "type": "string", "description": "Menu UUID to add the item to" },
            "parent_id": { "type": "string", "description": "Parent item UUID (optional)" },
            "label": { "type": "string", "description": "Item label" },
            "url": { "type": "string", "description": "Item URL" },
            "icon": { "type": "string", "description": "Icon name (optional)" },
            "sort_order": { "type": "integer", "description": "Sort order — default 0" },
            "permission_node": { "type": "string", "description": "Required permission node (optional)" },
            "required_group_ids": { "type": "array", "items": { "type": "string" }, "description": "Required group UUIDs (optional)" },
            "visibility": { "type": "string", "description": "Visibility (all, authenticated, anonymous) — default all" },
            "target": { "type": "string", "description": "Link target (_self, _blank) — default _self" },
            "is_ad_slot": { "type": "boolean", "description": "Is this an ad slot — default false" },
            "ad_slot_config": { "type": "object", "description": "Ad slot config (optional)" }
        },
        "required": ["menu_id", "label"]
    },
    "handler": "MenuCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'menus.manage'),

('00000000-0000-0000-0000-000000000001', 'menu.update',
 'Update a menu item. Requires menus.manage. HITL-queued.',
 'agent_menus',
 '{
    "function_name": "menu.update",
    "description": "Update a menu item.",
    "parameters": {
        "type": "object",
        "properties": {
            "item_id": { "type": "string", "description": "Menu item UUID" },
            "label": { "type": "string", "description": "New label" },
            "url": { "type": "string", "description": "New URL (optional)" },
            "icon": { "type": "string", "description": "New icon (optional)" },
            "parent_id": { "type": "string", "description": "New parent UUID (optional)" },
            "sort_order": { "type": "integer", "description": "New sort order (optional)" },
            "permission_node": { "type": "string", "description": "New permission node (optional)" },
            "required_group_ids": { "type": "array", "items": { "type": "string" }, "description": "New required group UUIDs (optional)" },
            "visibility": { "type": "string", "description": "New visibility (optional)" },
            "target": { "type": "string", "description": "New target (optional)" },
            "is_ad_slot": { "type": "boolean", "description": "New ad slot flag (optional)" },
            "ad_slot_config": { "type": "object", "description": "New ad slot config (optional)" },
            "status": { "type": "string", "description": "New status (active, inactive) — default active" }
        },
        "required": ["item_id", "label"]
    },
    "handler": "MenuUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'menus.manage'),

('00000000-0000-0000-0000-000000000001', 'menu.delete',
 'Delete a menu item. Requires menus.manage. HITL-queued.',
 'agent_menus',
 '{
    "function_name": "menu.delete",
    "description": "Delete a menu item.",
    "parameters": {
        "type": "object",
        "properties": {
            "item_id": { "type": "string", "description": "Menu item UUID" }
        },
        "required": ["item_id"]
    },
    "handler": "MenuDelete",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'menus.manage'),

-- =====================================================================
-- User tools (tasks 9.06)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'user.read_profile',
 'Read a user public profile (no PII — email, last_login, groups omitted). Privacy-gated: blocked when the target user is ai_exempt.',
 'agent_user',
 '{
    "function_name": "user.read_profile",
    "description": "Read a user public profile.",
    "parameters": {
        "type": "object",
        "properties": {
            "user_id": { "type": "string", "description": "User UUID" }
        },
        "required": ["user_id"]
    },
    "handler": "UserReadProfile",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'user.update_profile',
 'Update the calling user own profile fields. Self-only — user_id must match the calling user. No permission node required (self-service).',
 'agent_user',
 '{
    "function_name": "user.update_profile",
    "description": "Update own profile fields.",
    "parameters": {
        "type": "object",
        "properties": {
            "user_id": { "type": "string", "description": "User UUID (must be the calling user)" },
            "display_name": { "type": "string", "description": "New display name (optional)" },
            "avatar_url": { "type": "string", "description": "New avatar URL (optional)" },
            "bio": { "type": "string", "description": "New bio (optional)" },
            "timezone": { "type": "string", "description": "New timezone (optional)" },
            "origin_country": { "type": "string", "description": "New origin country (optional)" },
            "destination_country": { "type": "string", "description": "New destination country (optional)" }
        },
        "required": ["user_id"]
    },
    "handler": "UserUpdateProfile",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'user.read_own_fields',
 'Read the calling user own custom field values (including private/ai_exempt fields). Self-only — user_id must match the calling user.',
 'agent_user',
 '{
    "function_name": "user.read_own_fields",
    "description": "Read own custom field values (including private fields).",
    "parameters": {
        "type": "object",
        "properties": {
            "user_id": { "type": "string", "description": "User UUID (must be the calling user)" }
        },
        "required": ["user_id"]
    },
    "handler": "UserReadOwnFields",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

-- =====================================================================
-- Custom field definition tools (tasks 9.07)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'custom_fields.create',
 'Create a custom field definition. Requires custom_fields.field.create. HITL-queued.',
 'agent_custom_fields',
 '{
    "function_name": "custom_fields.create",
    "description": "Create a custom field definition.",
    "parameters": {
        "type": "object",
        "properties": {
            "group_id": { "type": "string", "description": "Field group UUID" },
            "label": { "type": "string", "description": "Field label" },
            "key": { "type": "string", "description": "Field key (machine name)" },
            "field_type": { "type": "string", "description": "Field type (text, textarea, number, boolean, date, select, json)" },
            "options": { "type": "object", "description": "Field options (JSON, optional)" },
            "embed_platforms": { "type": "array", "items": { "type": "string" }, "description": "Embed platforms (optional)" },
            "placeholder": { "type": "string", "description": "Placeholder (optional)" },
            "default_value": { "type": "string", "description": "Default value (optional)" },
            "is_required": { "type": "boolean", "description": "Required — default false" },
            "is_system": { "type": "boolean", "description": "System field — default false" },
            "show_in_frontend": { "type": "boolean", "description": "Show in frontend — default false" },
            "filterable": { "type": "boolean", "description": "Filterable — default false" },
            "sortable": { "type": "boolean", "description": "Sortable — default false" },
            "searchable": { "type": "boolean", "description": "Searchable — default false" },
            "position": { "type": "integer", "description": "Position — default 0" }
        },
        "required": ["group_id", "label", "key", "field_type"]
    },
    "handler": "CustomFieldsCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'custom_fields.field.create'),

('00000000-0000-0000-0000-000000000001', 'custom_fields.update',
 'Update a custom field definition. Requires custom_fields.field.edit. HITL-queued.',
 'agent_custom_fields',
 '{
    "function_name": "custom_fields.update",
    "description": "Update a custom field definition.",
    "parameters": {
        "type": "object",
        "properties": {
            "field_id": { "type": "string", "description": "Field UUID" },
            "label": { "type": "string", "description": "New label" },
            "field_type": { "type": "string", "description": "New field type (optional)" },
            "options": { "type": "object", "description": "New options (optional)" },
            "embed_platforms": { "type": "array", "items": { "type": "string" }, "description": "New embed platforms (optional)" },
            "placeholder": { "type": "string", "description": "New placeholder (optional)" },
            "default_value": { "type": "string", "description": "New default value (optional)" },
            "is_required": { "type": "boolean", "description": "New required flag (optional)" },
            "show_in_frontend": { "type": "boolean", "description": "New frontend flag (optional)" },
            "filterable": { "type": "boolean", "description": "New filterable flag (optional)" },
            "sortable": { "type": "boolean", "description": "New sortable flag (optional)" },
            "searchable": { "type": "boolean", "description": "New searchable flag (optional)" },
            "position": { "type": "integer", "description": "New position (optional)" }
        },
        "required": ["field_id", "label"]
    },
    "handler": "CustomFieldsUpdate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'custom_fields.field.edit'),

('00000000-0000-0000-0000-000000000001', 'custom_field_groups.list',
 'List custom field groups for the tenant.',
 'agent_custom_fields',
 '{
    "function_name": "custom_field_groups.list",
    "description": "List custom field groups.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "CustomFieldGroupsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'custom_field_groups.create',
 'Create a custom field group. Requires custom_fields.group.create. HITL-queued.',
 'agent_custom_fields',
 '{
    "function_name": "custom_field_groups.create",
    "description": "Create a custom field group.",
    "parameters": {
        "type": "object",
        "properties": {
            "name": { "type": "string", "description": "Group name" },
            "slug": { "type": "string", "description": "URL slug" },
            "description": { "type": "string", "description": "Description (optional)" },
            "is_system": { "type": "boolean", "description": "System group — default false" }
        },
        "required": ["name", "slug"]
    },
    "handler": "CustomFieldGroupsCreate",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'custom_fields.group.create'),

-- =====================================================================
-- Content lifecycle tools (tasks 9.08)
-- =====================================================================

('00000000-0000-0000-0000-000000000001', 'content.delete',
 'Soft-delete a content row. Requires content.delete. HITL-queued. Privacy-gated: blocked when the content is ai_exempt.',
 'agent_content',
 '{
    "function_name": "content.delete",
    "description": "Soft-delete a content row.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Content UUID" }
        },
        "required": ["content_id"]
    },
    "handler": "ContentDelete",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'content.delete'),

('00000000-0000-0000-0000-000000000001', 'content.list',
 'List content by type (or tenant-wide when content_type is empty). Exempt content is filtered post-query.',
 'agent_content',
 '{
    "function_name": "content.list",
    "description": "List content by type or tenant-wide.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_type": { "type": "string", "description": "Content type filter (optional)" },
            "limit": { "type": "integer", "description": "Max results (default 20, max 50)" }
        }
    },
    "handler": "ContentList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'content.publish',
 'Change a content row status (default published). Requires content.publish. HITL-queued. Privacy-gated: blocked when the content is ai_exempt.',
 'agent_content',
 '{
    "function_name": "content.publish",
    "description": "Change content status (default published).",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Content UUID" },
            "status": { "type": "string", "description": "New status (published, draft, scheduled, archived) — default published" }
        },
        "required": ["content_id"]
    },
    "handler": "ContentPublish",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'content.publish')

ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
