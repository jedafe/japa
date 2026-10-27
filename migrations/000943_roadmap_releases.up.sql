-- 000943_roadmap_releases.up.sql
--
-- Versioned releases on the public roadmap (§113, ADR-135): a curated
-- per-surface release entity grouping shipped roadmap_items plus an
-- append-only commit ledger (roadmap_release_entries) populated by CI
-- ingest. surface='platform' releases are wave labels (SaaS — no support
-- contract); surface='mobile' releases are semver + eol_at/is_esr support
-- windows (the Flutter app is versioned software).
--
-- Also seeds: the roadmap.commit_scope_paths system_setting (file-path →
-- scope map for ingest fallback) and eight agent tools for release/entry
-- management, reusing the existing roadmap.* RBAC nodes — no new
-- permission keys are minted.
--
-- Ref: .jeos/sections/113-public-product-roadmap.md (Versioned Releases)
--      .jeos/adr/ADR-135-versioned-roadmap-releases.md

BEGIN;

CREATE TABLE roadmap_releases (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    surface     text NOT NULL DEFAULT 'platform'
                CHECK (surface IN ('platform','mobile')),
    version     text NOT NULL,
    slug        text NOT NULL UNIQUE,
    title       text,
    summary     text,
    body        text,
    status      text NOT NULL DEFAULT 'draft'
                CHECK (status IN ('draft','published','yanked')),
    is_esr      bool NOT NULL DEFAULT false,
    released_at timestamptz,
    eol_at      timestamptz,
    sort_order  int  NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    UNIQUE (surface, version)
);

ALTER TABLE roadmap_items
    ADD COLUMN release_id uuid REFERENCES roadmap_releases(id) ON DELETE SET NULL;

CREATE INDEX idx_roadmap_items_release ON roadmap_items (release_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_roadmap_releases_public ON roadmap_releases (surface, released_at DESC)
    WHERE deleted_at IS NULL AND status = 'published';

CREATE TABLE roadmap_release_entries (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    release_id  uuid REFERENCES roadmap_releases(id) ON DELETE SET NULL,
    item_id     uuid REFERENCES roadmap_items(id) ON DELETE SET NULL,
    commit_sha  text,
    commit_url  text,
    commit_type text NOT NULL DEFAULT 'other'
                CHECK (commit_type IN ('feature','fix','ui','performance',
                                       'security','translation','accessibility',
                                       'docs','chore','refactor','test','ci','other')),
    scope       text,
    is_breaking bool NOT NULL DEFAULT false,
    title       text NOT NULL,
    author_name text,
    committed_at timestamptz,
    sort_order  int  NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_release_entries_sha
    ON roadmap_release_entries (commit_sha) WHERE commit_sha IS NOT NULL;
CREATE INDEX idx_release_entries_queue ON roadmap_release_entries (release_id);

-- File-path → scope map for commit ingest (fallback when a conventional
-- scope doesn't resolve to a feature flag). Longest prefix wins.
INSERT INTO system_settings (key, value, is_public) VALUES
('roadmap.commit_scope_paths',
 to_jsonb('{"internal/blog":"blog.enabled","internal/forum":"forum.enabled","internal/spaces":"spaces.enabled","internal/services":"services.enabled","internal/mentorship":"mentorship.enabled","internal/jobs":"jobs.enabled","apps/mobile":"mobile"}'::jsonb),
 false)
ON CONFLICT (key) DO NOTHING;

-- §103 agent tools — the release/entry admin surface for the built-in
-- agent and MCP callers (§119). Same roadmap.* nodes as the REST routes.
INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
('roadmap.releases.list',
 'List roadmap releases with optional surface/status filters (paginated). Releases group shipped items and commit entries under named versions.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.releases.list",
    "description": "List roadmap releases with optional surface/status filters and pagination.",
    "parameters": {
        "type": "object",
        "properties": {
            "surface": { "type": "string", "enum": ["platform", "mobile"], "description": "Optional surface filter" },
            "status":  { "type": "string", "enum": ["draft", "published", "yanked"], "description": "Optional status filter" },
            "page":    { "type": "integer", "description": "Page number, 1-based (default 1)" },
            "limit":   { "type": "integer", "description": "Page size, max 100 (default 25)" }
        },
        "required": []
    },
    "handler": "RoadmapReleasesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.view'),

('roadmap.release.get',
 'Read a single roadmap release by UUID — full notes body, surface, version, status, release/EOL dates, ESR flag.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.release.get",
    "description": "Read a single roadmap release by UUID.",
    "parameters": {
        "type": "object",
        "properties": {
            "id": { "type": "string", "description": "Release UUID" }
        },
        "required": ["id"]
    },
    "handler": "RoadmapReleaseGet",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.view'),

('roadmap.release.create',
 'Create a roadmap release. status="published" lands it publicly immediately; mobile versions must be semver (v2.4.0).',
 'agent_roadmap',
 '{
    "function_name": "roadmap.release.create",
    "description": "Create a roadmap release.",
    "parameters": {
        "type": "object",
        "properties": {
            "surface":     { "type": "string", "enum": ["platform", "mobile"], "description": "platform = wave label; mobile = semver + support window (default platform)" },
            "version":     { "type": "string", "description": "Version label — semver for mobile (v2.4.0), free label for platform (2026.11)" },
            "slug":        { "type": "string", "description": "URL slug (optional — derived from version when omitted)" },
            "title":       { "type": "string", "description": "Optional release/wave name" },
            "summary":     { "type": "string", "description": "One-line card/feed summary" },
            "body":        { "type": "string", "description": "Markdown release notes" },
            "status":      { "type": "string", "enum": ["draft", "published"], "description": "Initial status (default draft); published goes live immediately" },
            "is_esr":      { "type": "boolean", "description": "Mobile only: extended support line" },
            "released_at": { "type": "string", "description": "ISO 8601; future date schedules the release (upcoming)" },
            "eol_at":      { "type": "string", "description": "ISO 8601 end-of-support date (mobile)" }
        },
        "required": ["version"]
    },
    "handler": "RoadmapReleaseCreate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.edit'),

('roadmap.release.update',
 'Patch a roadmap release — version, slug, title, summary, body, surface, ESR flag, release/EOL dates.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.release.update",
    "description": "Update a roadmap release.",
    "parameters": {
        "type": "object",
        "properties": {
            "id":          { "type": "string", "description": "Release UUID" },
            "version":     { "type": "string" },
            "slug":        { "type": "string" },
            "title":       { "type": "string" },
            "summary":     { "type": "string" },
            "body":        { "type": "string", "description": "Markdown release notes" },
            "surface":     { "type": "string", "enum": ["platform", "mobile"] },
            "is_esr":      { "type": "boolean" },
            "released_at": { "type": "string", "description": "ISO 8601" },
            "eol_at":      { "type": "string", "description": "ISO 8601" }
        },
        "required": ["id"]
    },
    "handler": "RoadmapReleaseUpdate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.edit'),

('roadmap.release.set_status',
 'Drive a release lifecycle transition (draft → published → yanked; yanked → published re-publishes). Publishing stamps released_at when unset and, by default, sweeps all unreleased commit entries into the release.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.release.set_status",
    "description": "Transition a release lifecycle status.",
    "parameters": {
        "type": "object",
        "properties": {
            "id":                { "type": "string", "description": "Release UUID" },
            "status":            { "type": "string", "enum": ["draft", "published", "yanked"] },
            "absorb_unreleased": { "type": "boolean", "description": "On publish, sweep queued commit entries into the release (default true)" }
        },
        "required": ["id", "status"]
    },
    "handler": "RoadmapReleaseSetStatus",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.moderate'),

('roadmap.release.attach_items',
 'Attach shipped roadmap items to a release — by explicit item_ids or by a shipped_at window. Replaces the release attachment set.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.release.attach_items",
    "description": "Attach shipped roadmap items to a release.",
    "parameters": {
        "type": "object",
        "properties": {
            "id":             { "type": "string", "description": "Release UUID" },
            "item_ids":       { "type": "array", "items": { "type": "string" }, "description": "Roadmap item UUIDs (shipped only)" },
            "shipped_since":  { "type": "string", "description": "ISO 8601 — attach items shipped on/after this (window mode)" },
            "shipped_until":  { "type": "string", "description": "ISO 8601 — attach items shipped before this (window mode)" }
        },
        "required": ["id"]
    },
    "handler": "RoadmapReleaseAttachItems",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.edit'),

('roadmap.entries.list',
 'List release changelog entries (parsed commits) with optional unreleased/type/scope filters — the queue a publish absorbs.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.entries.list",
    "description": "List release changelog entries with optional filters.",
    "parameters": {
        "type": "object",
        "properties": {
            "unreleased": { "type": "boolean", "description": "Only entries not yet assigned to a release" },
            "type":       { "type": "string", "description": "commit_type filter (feature, fix, ui, performance, security, translation, accessibility, docs, chore, refactor, test, ci, other)" },
            "scope":      { "type": "string", "description": "Scope/flag filter (e.g. blog.enabled)" },
            "page":       { "type": "integer", "description": "Page number, 1-based (default 1)" },
            "limit":      { "type": "integer", "description": "Page size, max 100 (default 25)" }
        },
        "required": []
    },
    "handler": "RoadmapEntriesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.view'),

('roadmap.entries.ingest',
 'Ingest commits into the release changelog queue. Accepts GitHub push-event commit objects; parses conventional commits into type/scope, dedupes by SHA. Entries queue as unreleased until a release publish absorbs them.',
 'agent_roadmap',
 '{
    "function_name": "roadmap.entries.ingest",
    "description": "Ingest commits into the release changelog queue.",
    "parameters": {
        "type": "object",
        "properties": {
            "commits": {
                "type": "array",
                "description": "GitHub push-event commit objects (id, message, url, timestamp, author{name}, added, modified, removed)",
                "items": { "type": "object" }
            }
        },
        "required": ["commits"]
    },
    "handler": "RoadmapEntriesIngest",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'roadmap.edit');

COMMIT;
