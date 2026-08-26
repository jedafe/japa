CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL DEFAULT '',
  watched_path TEXT NOT NULL DEFAULT '',
  base_url TEXT NOT NULL DEFAULT '',
  tenant_id TEXT,
  api_key TEXT,
  mode TEXT NOT NULL DEFAULT 'operator',
  auto_sync INTEGER NOT NULL DEFAULT 0,
  auto_sync_content_types TEXT NOT NULL DEFAULT '[]',
  default_content_type TEXT NOT NULL DEFAULT '',
  default_guide_category TEXT NOT NULL DEFAULT '',
  default_persona_id TEXT NOT NULL DEFAULT '',
  default_voice_id TEXT NOT NULL DEFAULT '',
  default_space_id TEXT NOT NULL DEFAULT '',
  default_status TEXT NOT NULL DEFAULT 'draft',
  default_visibility TEXT NOT NULL DEFAULT 'public',
  default_tags TEXT NOT NULL DEFAULT '[]',
  access_token TEXT,
  refresh_token TEXT,
  token_expires_at INTEGER,
  device_id TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS file_states (
  project_id TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  content_id TEXT,
  content_type TEXT,
  slug TEXT,
  title TEXT,
  sync_state TEXT NOT NULL DEFAULT 'new',
  last_hash TEXT NOT NULL DEFAULT '',
  last_published_hash TEXT,
  last_published_at TEXT,
  last_error TEXT,
  space_id TEXT,
  persona_id TEXT,
  guide_category TEXT,
  status TEXT,
  visibility TEXT,
  modified_at TEXT,
  PRIMARY KEY (project_id, relative_path)
);

CREATE TABLE IF NOT EXISTS editor_drafts (
  project_id TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  frontmatter TEXT NOT NULL DEFAULT '{}',
  is_dirty INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (project_id, relative_path)
);

CREATE TABLE IF NOT EXISTS sync_log (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  relative_path TEXT NOT NULL,
  action TEXT NOT NULL,
  content_id TEXT,
  error TEXT,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO projects (id, name, watched_path, base_url) VALUES ('default', '', '', '');
