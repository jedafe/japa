# Versioned Roadmap Releases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Discourse-releases-style versioned release surface to the public roadmap — a `roadmap_releases` entity that groups shipped `roadmap_items` plus commit-sourced `roadmap_release_entries` under named releases, with per-surface (platform/mobile) version semantics, a timeline visualization, per-release changelog pages, RSS, admin CRUD, agent-tool automation, and CI-driven commit ingestion (backfill + ongoing).

**Architecture:** Extend the existing `internal/roadmap` module (Handlers → Service → Store, sqlc) rather than a new package — releases are roadmap domain data. `roadmap_items.release_id` FK links each shipped item to at most one release; the release itself carries its own markdown notes body (both data flows). A second table, `roadmap_release_entries`, is the append-only commit log: a CI ingest endpoint parses conventional commits (`type(scope): subject` — ~76% of the last 3 months' commits already conform) into categorized, extension-scoped, SHA-deduped entries that queue as "unreleased" until a publish absorbs them. Per-surface `surface` column (`platform` = wave labels, `mobile` = semver + EOL/is_esr support windows) replaces Discourse's channel model, which doesn't apply to a SaaS but does apply to the shipped Flutter app.

**Tech Stack:** Go + chi + sqlc/pgx backend; SvelteKit 2 + Svelte 5 + Tailwind v4 frontend (`apps/main` SSR, `apps/admin` SPA); golang-migrate; Valkey cache; existing §103 agent tool catalog / §119 MCP bridge.

## Global Constraints

- **JEOS first (Golden Rule 1):** Task 1 writes the spec before any code. §113 is the source of truth afterward.
- **Rule 2:** Never hand-edit `internal/db/gen/*` — edit `db/queries/roadmap.sql`, run `make sqlc-gen`.
- **Rule 4:** New Valkey key `jc:roadmap:releases` must be catalogued in `.jeos/conventions/valkey-keys-catalog.md` first.
- **Rule 7:** No new Asynq tasks in this feature (release publish reuses no fan-out).
- **Rule 11:** Release deletes are soft; `?force=true` hard delete requires `admin.hard_delete` via `rbac.RequireForceDelete`.
- **Rule 16:** All frontend data from API — the timeline, cards, badges all render from `/roadmap/releases` payloads.
- **Rule 17:** No AI/agent attribution in code, commits, release copy, or JEOS files.
- **Rule 18:** `bun`/`bunx`/`bun run` only for JS tooling.
- All new routes live inside the existing `features.Gate(featureChecker, "roadmap.enabled")` group → 404 when off.
- Reuse RBAC nodes: `roadmap.view` / `roadmap.edit` / `roadmap.delete` / `roadmap.moderate`. **No new permission nodes minted.**
- Version format: `surface='mobile'` requires semver (`^v?\d+\.\d+\.\d+`); `surface='platform'` accepts free wave labels (`2026.11`, `November 2026`).
- Commit-entry visibility is derived from `commit_type`: `feature|fix|ui|performance|security|translation|accessibility` render publicly; `docs|chore|refactor|test|ci|other` are `internal_only` (stored, admin-visible, never rendered on public routes). Admins may retype an entry to promote/demote it.

---

### Task 1: JEOS spec — §113 amendment + ADR-135 + catalog entries

**Files:**
- Modify: `.jeos/sections/113-public-product-roadmap.md`
- Create: `.jeos/adr/ADR-135-versioned-roadmap-releases.md`
- Modify: `.jeos/JEOS.md` (§113 row ~L181, ADR table ~L283, counts ~L383–388)
- Modify: `.jeos/conventions/api-routes-catalog.md` (roadmap block ~L2972)
- Modify: `.jeos/conventions/valkey-keys-catalog.md` (~L375)

- [ ] **Step 1: Append the Versioned Releases subsection to §113**

Insert after the "Agent & MCP Access" section, before "Edge Cases & Error Handling". Bump the header `**Status:** 📋 V3 (ADR-127)` → `**Status:** 📋 V4 (ADR-127, ADR-135)`.

```markdown
## Versioned Releases (ADR-135, migration `000942_roadmap_releases`)

`roadmap_releases` is a curated release/train entity — the JapaConnect
equivalent of releases.discourse.org — that groups shipped `roadmap_items`
into named versions. Releases are **per-surface**: `surface='platform'`
releases are wave labels (`2026.11`, "November 2026 Update") with no support
semantics (japa.to is a SaaS — there is no installed version to support);
`surface='mobile'` releases are real semver (`v2.4.0`) with `eol_at` /
`is_esr` support windows, because the Flutter app is versioned software users
actually run.

```sql
CREATE TABLE roadmap_releases (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  surface     text NOT NULL DEFAULT 'platform'
              CHECK (surface IN ('platform','mobile')),
  version     text NOT NULL,              -- 'v2.4.0' (mobile) | '2026.11' wave label (platform)
  slug        text NOT NULL UNIQUE,       -- URL-safe; deduped like item slugs
  title       text,                       -- optional wave/release name
  summary     text,                       -- one-liner for cards/feed
  body        text,                       -- markdown release notes
  status      text NOT NULL DEFAULT 'draft'
              CHECK (status IN ('draft','published','yanked')),
  is_esr      bool NOT NULL DEFAULT false,-- mobile: extended support line
  released_at timestamptz,                -- stamped on → published if unset
  eol_at      timestamptz,                -- mobile: end-of-support date
  sort_order  int  NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  UNIQUE (surface, version)
);
ALTER TABLE roadmap_items ADD COLUMN release_id uuid
  REFERENCES roadmap_releases(id) ON DELETE SET NULL;
```

**Lifecycle:** `draft → published → yanked` (no-op transitions allowed;
`yanked → published` re-publish allowed). `→ published` stamps
`released_at = now()` when unset; explicit future `released_at` = scheduled
release (`upcoming`).

**Computed `support_state`** (service layer, never stored):
`draft` | `yanked` | `upcoming` (released_at > now) | `latest` (newest
published, released_at <= now, per surface) | `esr` (is_esr, within eol_at) |
`eol` (eol_at <= now) | `supported`. Platform supersession end date =
`LEAD(released_at) OVER (PARTITION BY surface ORDER BY released_at)`.

**Item linkage:** `roadmap_items.release_id` — one item, at most one
release. Only `status='shipped'` items are attachable; attaching a
non-shipped item → `409 item_not_shipped`. Detach sets NULL. A mobile +
platform launch of the same feature is two items (or unlink/relink) —
a join table is explicitly out of scope (YAGNI).

**Routes** (inside the existing `roadmap.enabled` gate):

```
GET  /roadmap/releases              — public; published releases grouped by
                                      surface incl. support_state, item_count,
                                      superseded_at, unreleased_count;
                                      jc:roadmap:releases cached
GET  /roadmap/releases/feed.xml     — public; RSS 2.0 of published releases
GET  /roadmap/releases/unreleased   — public; queued public-type commit entries
GET  /roadmap/releases/{slug}       — public; release detail: notes body +
                                      shipped items + entries (?scope= filter)
GET  /admin/roadmap/releases        — roadmap.view; DataTable
POST /admin/roadmap/releases        — roadmap.edit
GET  /admin/roadmap/releases/{id}   — roadmap.view
PATCH /admin/roadmap/releases/{id}  — roadmap.edit
DELETE /admin/roadmap/releases/{id} — roadmap.delete; ?force=true + admin.hard_delete
POST /admin/roadmap/releases/{id}/restore   — roadmap.edit
PATCH /admin/roadmap/releases/{id}/status   — roadmap.moderate; publish stamps released_at
                                      + absorb_unreleased sweep
PUT  /admin/roadmap/releases/{id}/items     — roadmap.edit; {item_ids:[]} replaces
                                             the attachment set (shipped-only)
POST /admin/roadmap/releases/entries/ingest — roadmap.edit; GitHub push-shape
                                             commits → parse/dedupe/scope
GET  /admin/roadmap/releases/entries        — roadmap.view; entry queue
PATCH /admin/roadmap/releases/entries/{id}  — roadmap.edit; retype/retitle/relink
DELETE /admin/roadmap/releases/entries/{id} — roadmap.edit; hard delete
PATCH /admin/roadmap/items/{id}     — gains optional `release_id` (ship-only attach)
```

`GET /roadmap/changelog` items gain `release_version` + `release_slug` so
changelog rows badge-link into the release page.

**Agent tools (§103, seeded in the same migration):**
`roadmap.releases.list` / `roadmap.release.get` (`roadmap.view`),
`roadmap.release.create` / `roadmap.release.update` /
`roadmap.release.attach_items` / `roadmap.entries.list` /
`roadmap.entries.ingest` (`roadmap.edit`),
`roadmap.release.set_status` (`roadmap.moderate`). `attach_items` accepts
`item_ids[]` OR a `shipped_since`/`shipped_until` window for "cut release
from ship window" automation; `entries.ingest` is the CI path — callers
use `POST /api/v1/mcp` `tools/call` with an `agent.use`+`agent.write`
key owned by a principal holding `roadmap.edit` (API keys carry scopes,
not RBAC nodes — X-API-Key does not satisfy `auth.RequireAuth` on the
REST route, which stays for admin UI/JWT use).

**Valkey:** `jc:roadmap:releases` — JSON public releases payload; TTL
`roadmap.board_cache_ttl_seconds` (reused — no new setting); invalidated by
any release, release-entry (ingest/CRUD/absorb), or item-release mutation
— `unreleased_count` lives in that payload.

**Frontend:** `/roadmap/releases` — month-axis timeline (one band per
release, released_at → eol_at|superseded_at|now, color by support_state)
above surface-tabbed release cards plus an "In development" unreleased
strip; `/roadmap/releases/[slug]` — notes + shipped items + commit entries
grouped by type section; `/roadmap/releases/unreleased` — the queued
public-type entries; `/roadmap/changelog` rows badge-link to their release.
The `roadmap.japa.to` host shell renders these unchanged. Sitemap gains
release URLs.

## Commit-Sourced Entries (`roadmap_release_entries`)

The release's second data source: an append-only log of parsed commits —
the JapaConnect equivalent of the commit lists on releases.discourse.org
`/changelog/{version}` pages. Entries are **not** roadmap items (no
lifecycle, no votes); `item_id` optionally links an entry to the voted
item it shipped.

```sql
CREATE TABLE roadmap_release_entries (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  release_id  uuid REFERENCES roadmap_releases(id) ON DELETE SET NULL,
              -- NULL = unreleased queue
  item_id     uuid REFERENCES roadmap_items(id) ON DELETE SET NULL,
  commit_sha  text,                     -- NULL for manual entries
  commit_url  text,
  commit_type text NOT NULL DEFAULT 'other'
              CHECK (commit_type IN ('feature','fix','ui','performance',
                                     'security','translation','accessibility',
                                     'docs','chore','refactor','test','ci','other')),
  scope       text,                     -- resolved flag key ('blog.enabled')
                                        -- or raw commit scope ('main','admin')
  is_breaking bool NOT NULL DEFAULT false,
  title       text NOT NULL,            -- cleaned subject line
  author_name text,
  committed_at timestamptz,
  sort_order  int  NOT NULL DEFAULT 0,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_release_entries_sha
  ON roadmap_release_entries (commit_sha) WHERE commit_sha IS NOT NULL;
CREATE INDEX idx_release_entries_queue ON roadmap_release_entries (release_id);
```

**Parsing (conventional commits):** `type(scope)!: subject` —
`feat→feature`, `fix→fix`, `ui|ux|style→ui`, `perf→performance`,
`security|sec→security`, `i18n|l10n→translation`, `a11y→accessibility`,
`docs→docs`, `refactor→refactor`, `test→test`, `ci|build→ci`,
`chore|revert→chore`, anything else → `other`. `!` or `BREAKING CHANGE`
footer → `is_breaking` flag column → renders as a badge. Non-conforming
subjects land as `other` (internal-only; admins retype to promote).

**Scope resolution order:** (1) conventional `scope` → exact
`feature_flags.key` match, then `{scope}.enabled`, then `feature.{scope}`;
(2) if unresolved, the commit's file list is walked against the
`system_settings` JSON map `roadmap.commit_scope_paths`
(`{"internal/blog":"blog.enabled","apps/mobile":"mobile",…}` — longest
prefix wins); (3) else the raw scope string is stored for display-only
chips (e.g. `main`, `admin` are surfaces, not extensions).

**Ingest:** accepts GitHub push-event commit objects verbatim
(`{id,message,url,timestamp,author{name},added,modified,removed}`) via
REST `POST /admin/roadmap/releases/entries/ingest` (JWT/admin UI) or the
`roadmap.entries.ingest` tool via `POST /api/v1/mcp` (CI/automation);
dedupes by sha (`ON CONFLICT DO NOTHING`), returns `{created,skipped}`.
New system_settings row `roadmap.commit_scope_paths` (JSON object,
default seeded map) joins the settings catalog.

**Publish absorb:** `PATCH …/releases/{id}/status → published` accepts
`{"absorb_unreleased": bool}` (default true) — sets `release_id` on every
queued entry, so cutting a release sweeps the commit log into it.

**Backfill:** `scripts/changelog-backfill.sh` emits
`git log --since=<date>` in the GitHub commit shape and batches to the
ingest endpoint — sha dedupe makes re-runs idempotent.

**Public render:** `GET /roadmap/releases` gains `unreleased_count`
(public-type entries only); `GET /roadmap/releases/unreleased` lists
public-type queued entries; `GET /roadmap/releases/{slug}` returns
`entries` ordered by `commit_type` (frontend groups into sections —
same pattern as the changelog's month grouping) with a `?scope=` filter
applied to both items and entries.

**Slug reservations:** `unreleased` and `feed.xml` are reserved public
route segments — slug generation rejects them (`409 validation_error`)
like it dedupes collisions.
```

- [ ] **Step 2: Create ADR-135**

```markdown
# ADR-135 — Versioned Releases on the Public Roadmap: Per-Surface Release Entity

**Status:** Accepted · **Date:** 2026-10-27 · **Section:** §113 (amends ADR-127)

## Decision

Add `roadmap_releases` inside the existing `internal/roadmap` module —
a curated release entity that groups shipped `roadmap_items`
(`release_id` FK) under named, per-surface versions
(`surface='platform'|'mobile'`). Public routes expose a release list with
computed `support_state`, per-release changelog pages, and an RSS feed;
admin/agent surfaces manage the lifecycle `draft → published → yanked`.
This revises ADR-127's "no separate changelog store exists by design":
the *store* remains `roadmap_items` — releases are a grouping layer, not
a second changelog.

## Rationale

- **Per-surface over channels.** Discourse's latest/release/ESR channels
  describe install trains for self-hosted software. japa.to is a SaaS —
  platform "versions" are communications waves, not support contracts.
  The Flutter app (`apps/mobile`) IS versioned software with a real
  support window (`device_tokens.app_version` already records what users
  run), so `surface` cleanly splits wave labels from semver+EOL releases.
- **Grouping over duplication.** `release_id` on `roadmap_items` keeps
  one source of truth for what shipped; the release's own `body` carries
  editorial notes (upgrade instructions, highlights) items can't express.
- **No new RBAC nodes.** Release curation is the same trust level as
  item editing/moderation — `roadmap.edit`/`moderate`/`delete` suffice.
- **Support state computed, never stored.** `latest`/`supported`/`eol`
  drift with time and with sibling releases; deriving them from
  `released_at`/`eol_at`/`LEAD()` at read time cannot go stale.
- **Commit log is separate from curated items.** `roadmap_release_entries`
  is an append-only, SHA-deduped commit ledger — not votable items —
  populated by CI ingest (GitHub push payload → conventional-commit
  parse) and absorbing into releases at publish. Items answer "what did
  the community ask for"; entries answer "what actually landed" — merging
  them would let raw commits spam the votable board.
- **Visibility by type, not moderation queue.** ~76% of the repo's last
  3 months already use conventional commits; mapping
  `feat/fix/ui/perf/security/i18n/a11y` to public sections and everything
  else to internal-only gives a curated-looking changelog with zero human
  triage, while admins can still retype outliers.

## Consequences

- New tables `roadmap_releases`, `roadmap_release_entries` +
  `roadmap_items.release_id` (migration `000942_roadmap_releases`); FKs
  recorded in `schemas/MIGRATION-ORDER.md`.
- 16 new routes (4 public, 12 admin) + `release_id` PATCH field on items.
- New Valkey pattern `jc:roadmap:releases`; new system_settings
  `roadmap.commit_scope_paths`; 8 new agent tools seeded in the same
  migration.
- `.github/workflows/changelog-ingest.yml` pushes commit payloads to the
  ingest route on every `push: main`; `scripts/changelog-backfill.sh`
  replays `git log --since` through the same route.
- `apps/main` gains `/roadmap/releases`, `/roadmap/releases/[slug]`,
  `/roadmap/releases/unreleased`; changelog rows gain release badges;
  `apps/admin` gains a Releases EntityPage + entries queue view.
- Mobile EOL data is display-only in this iteration — a future task may
  feed `eol_at` into a min-supported-version gate for `device_tokens`.
```

- [ ] **Step 3: Update JEOS.md index**

- §113 row (~L181): append `+ roadmap_releases per-surface versioned releases (platform waves / mobile semver+EOL) + roadmap_release_entries commit-sourced changelog (CI ingest, conventional-commit parsing, extension scoping); /roadmap/releases timeline + per-release pages + RSS (ADR-135)`.
- ADR table after ADR-134 row: add `| 135 | adr/ADR-135-versioned-roadmap-releases.md | Versioned Roadmap Releases — per-surface release entity in internal/roadmap; groups shipped items via release_id FK + commit-sourced release entries (conventional-commit ingest, SHA dedupe, flag-scope resolution); platform waves vs mobile semver+EOL; computed support_state; amends ADR-127 no-changelog-store consequence |`.
- Counts (~L383): `~164` → `~166` tables `(+2 from §113: roadmap_releases, roadmap_release_entries)`; route count +16; agent tools +8; settings +1 (`roadmap.commit_scope_paths`).

- [ ] **Step 4: Catalog entries**

Append to the roadmap block in `api-routes-catalog.md` (~L2996, after categories):

```
GET    /api/v1/roadmap/releases                    — [public] published releases by surface + support_state; jc:roadmap:releases cached
GET    /api/v1/roadmap/releases/feed.xml           — [public] RSS 2.0 of published releases
GET    /api/v1/roadmap/releases/unreleased         — [public] queued public-type commit entries
GET    /api/v1/roadmap/releases/{slug}             — [public] release detail + shipped items + entries grouped by commit_type (?scope= filter)
GET    /api/v1/admin/roadmap/releases              — [perm: roadmap.view] DataTable
POST   /api/v1/admin/roadmap/releases              — [perm: roadmap.edit]
GET    /api/v1/admin/roadmap/releases/{id}         — [perm: roadmap.view]
PATCH  /api/v1/admin/roadmap/releases/{id}         — [perm: roadmap.edit]
DELETE /api/v1/admin/roadmap/releases/{id}         — [perm: roadmap.delete; ?force=true + admin.hard_delete]
POST   /api/v1/admin/roadmap/releases/{id}/restore — [perm: roadmap.edit]
PATCH  /api/v1/admin/roadmap/releases/{id}/status  — [perm: roadmap.moderate] draft→published→yanked; stamps released_at;
                                                      {absorb_unreleased} sweeps queued entries in
PUT    /api/v1/admin/roadmap/releases/{id}/items   — [perm: roadmap.edit] {item_ids:[]} replace attachments (shipped-only)
POST   /api/v1/admin/roadmap/releases/entries/ingest — [perm: roadmap.edit] GitHub push-shape commits;
                                                      conventional-commit parse + sha dedupe + scope resolution
GET    /api/v1/admin/roadmap/releases/entries      — [perm: roadmap.view] entry queue/DataTable (?unreleased=&type=&scope=)
PATCH  /api/v1/admin/roadmap/releases/entries/{id} — [perm: roadmap.edit] retype/retitle/relink item_id
DELETE /api/v1/admin/roadmap/releases/entries/{id} — [perm: roadmap.edit] hard delete (operational data, Rule 11 n/a)
```

Also append to `system-settings-catalog.md` under the `roadmap.*` block:

```
| `roadmap.commit_scope_paths` | JSON | `{"internal/blog":"blog.enabled","internal/forum":"forum.enabled","internal/spaces":"spaces.enabled","internal/services":"services.enabled","internal/mentorship":"mentorship.enabled","internal/jobs":"jobs.enabled","apps/mobile":"mobile"}` | Longest-prefix file-path → scope map consulted by commit ingest when a conventional-commit scope doesn't resolve to a feature flag |
```

Append to `valkey-keys-catalog.md` after `jc:roadmap:board`:

```
| `jc:roadmap:releases` | JSON-serialized public releases payload (surfaces × releases + support_state + unreleased_count) | `roadmap.board_cache_ttl_seconds` (default 60s) | any roadmap release, release-entry (ingest/CRUD/absorb), or item-release mutation |
```

- [ ] **Step 5: Commit** — `.jeos` is a **git submodule** (`.gitmodules` → `github.com/jedafe/japaconnect-engineering`): commit inside it first, then stage the pointer bump in the parent (the repo's existing "chore: update JEOS submodule" convention):

```bash
cd .jeos && git add -A && git commit -m "docs(113): spec per-surface versioned releases (ADR-135)" && cd ..
git add .jeos
git commit -m "chore: update JEOS submodule for versioned-releases spec"
```

---

### Task 2: Migration `000942_roadmap_releases` + sqlc queries

**Files:**
- Create: `migrations/000942_roadmap_releases.up.sql`
- Create: `migrations/000942_roadmap_releases.down.sql`
- Modify: `internal/db/queries/roadmap.sql` (append releases section)
- Modify: `internal/db/gen/*` (regenerated only)
- Check: `.jeos/schemas/MIGRATION-ORDER.md` — record `roadmap_releases → (none)`, `roadmap_items.release_id → roadmap_releases`

**Interfaces:**
- Produces: `gen.RoadmapRelease`, `gen.RoadmapReleaseEntry` models; `gen.ListPublicRoadmapReleasesRow`, `gen.GetRoadmapReleaseBySlug`, `gen.ListRoadmapReleaseItems`, `gen.ListRoadmapReleaseEntries`, `gen.ListPublicUnreleasedEntries`, `gen.CountUnreleasedEntries`, `gen.ListAdminRoadmapReleases`, `gen.CreateRoadmapRelease`, `gen.UpdateRoadmapRelease`, `gen.SetRoadmapReleaseStatus`, `gen.SoftDeleteRoadmapRelease`, `gen.RestoreRoadmapRelease`, `gen.SetRoadmapItemRelease`, `gen.InsertReleaseEntry` (`:execrows` — 0 rows = sha dupe), `gen.AssignUnreleasedEntries`, `gen.ListAdminReleaseEntries`, `gen.UpdateReleaseEntry`, `gen.DeleteReleaseEntry`, `gen.ListRoadmapReleaseSlugs` (sitemap).

- [ ] **Step 1: Write the up migration**

```sql
-- 000942_roadmap_releases.up.sql
--
-- Versioned releases on the public roadmap (§113, ADR-135): curated
-- per-surface release entity grouping shipped roadmap_items. platform =
-- wave labels (SaaS — no support contract); mobile = semver + eol_at/is_esr
-- support windows (Flutter app is versioned software).
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
INSERT INTO system_settings (key, value, type, description) VALUES
('roadmap.commit_scope_paths',
 '{"internal/blog":"blog.enabled","internal/forum":"forum.enabled","internal/spaces":"spaces.enabled","internal/services":"services.enabled","internal/mentorship":"mentorship.enabled","internal/jobs":"jobs.enabled","apps/mobile":"mobile"}',
 'json', 'Longest-prefix file-path → scope map for roadmap commit ingest')
ON CONFLICT (key) DO NOTHING;

COMMIT;
```

Also append the eight `ai_tools` INSERTs in the same migration, mirroring
`000928_seed_roadmap_agent_tools.up.sql` exactly (category `agent_roadmap`,
`handler` names `RoadmapReleasesList`/`RoadmapReleaseGet`/`RoadmapReleaseCreate`/
`RoadmapReleaseUpdate`/`RoadmapReleaseSetStatus`/`RoadmapReleaseAttachItems`/
`RoadmapEntriesList`/`RoadmapEntriesIngest`, `required_permission` per the
route table in Task 1).

- [ ] **Step 2: Write the down migration**

```sql
BEGIN;
DELETE FROM ai_tools WHERE name IN (
  'roadmap.releases.list','roadmap.release.get','roadmap.release.create',
  'roadmap.release.update','roadmap.release.set_status','roadmap.release.attach_items',
  'roadmap.entries.list','roadmap.entries.ingest');
DELETE FROM system_settings WHERE key = 'roadmap.commit_scope_paths';
DROP TABLE roadmap_release_entries;
ALTER TABLE roadmap_items DROP COLUMN release_id;
DROP TABLE roadmap_releases;
COMMIT;
```

- [ ] **Step 3: Append sqlc queries to `internal/db/queries/roadmap.sql`**

```sql
-- =====================
-- Roadmap releases (ADR-135)
-- =====================

-- name: ListPublicRoadmapReleases :many
-- Published releases for the public timeline; superseded_at = next
-- release's released_at per surface (LEAD window) — the band end when
-- eol_at is unset.
SELECT r.*,
    LEAD(r.released_at) OVER (PARTITION BY r.surface ORDER BY r.released_at) AS superseded_at,
    (SELECT count(*) FROM roadmap_items i
     WHERE i.release_id = r.id AND i.deleted_at IS NULL AND i.status = 'shipped') AS item_count
FROM roadmap_releases r
WHERE r.status = 'published' AND r.deleted_at IS NULL
ORDER BY r.surface, r.released_at DESC;

-- name: GetRoadmapReleaseBySlug :one
-- Plain row fetch — superseded_at is only needed by the timeline list
-- (LEAD must run over the whole surface set, not a post-WHERE single row).
SELECT * FROM roadmap_releases
WHERE slug = @slug AND status IN ('published','yanked') AND deleted_at IS NULL;

-- name: ListRoadmapReleaseItems :many
SELECT i.id, i.slug, i.title, i.summary, i.shipped_at, i.vote_count,
       c.name AS category_name, c.color AS category_color
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id
WHERE i.release_id = @release_id AND i.status = 'shipped' AND i.deleted_at IS NULL
ORDER BY i.shipped_at DESC;

-- name: ListAdminRoadmapReleases :many
SELECT r.*,
    (SELECT count(*) FROM roadmap_items i
     WHERE i.release_id = r.id AND i.deleted_at IS NULL) AS item_count
FROM roadmap_releases r
WHERE (@include_deleted::bool OR r.deleted_at IS NULL)
  AND (@surface::text = '' OR r.surface = @surface)
ORDER BY r.created_at DESC
LIMIT @page_size OFFSET @page_offset;

-- name: CountAdminRoadmapReleases :one
SELECT count(*) FROM roadmap_releases r
WHERE (@include_deleted::bool OR r.deleted_at IS NULL)
  AND (@surface::text = '' OR r.surface = @surface);

-- name: GetRoadmapReleaseByID :one
SELECT * FROM roadmap_releases WHERE id = @id;

-- name: CreateRoadmapRelease :one
INSERT INTO roadmap_releases
    (surface, version, slug, title, summary, body, status, is_esr,
     released_at, eol_at, sort_order)
VALUES (@surface, @version, @slug, sqlc.narg(title), sqlc.narg(summary),
        sqlc.narg(body), @status, @is_esr,
        sqlc.narg(released_at), sqlc.narg(eol_at), @sort_order)
RETURNING *;

-- name: UpdateRoadmapRelease :one
UPDATE roadmap_releases SET
    version = @version, slug = @slug,
    title = sqlc.narg(title), summary = sqlc.narg(summary),
    body = sqlc.narg(body), surface = @surface, is_esr = @is_esr,
    released_at = sqlc.narg(released_at), eol_at = sqlc.narg(eol_at),
    sort_order = @sort_order, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetRoadmapReleaseStatus :one
UPDATE roadmap_releases SET
    status = @status,
    released_at = CASE WHEN @status = 'published' AND released_at IS NULL
                       THEN now() ELSE released_at END,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SoftDeleteRoadmapRelease :exec
UPDATE roadmap_releases SET deleted_at = now(), updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: RestoreRoadmapRelease :exec
UPDATE roadmap_releases SET deleted_at = NULL, updated_at = now()
WHERE id = @id;

-- name: HardDeleteRoadmapRelease :exec
DELETE FROM roadmap_releases WHERE id = @id AND deleted_at IS NOT NULL;

-- name: ClearRoadmapReleaseItems :exec
UPDATE roadmap_items SET release_id = NULL, updated_at = now()
WHERE release_id = @release_id;

-- name: SetRoadmapItemRelease :exec
UPDATE roadmap_items SET release_id = @release_id, updated_at = now()
WHERE id = @item_id AND status = 'shipped' AND deleted_at IS NULL;

-- name: ListRoadmapReleaseSlugs :many
SELECT slug, updated_at FROM roadmap_releases
WHERE status = 'published' AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- =====================
-- Release entries (commit log)
-- =====================

-- name: InsertReleaseEntry :execrows
-- 0 rows = sha already ingested (dedupe). NULL sha always inserts.
INSERT INTO roadmap_release_entries
    (release_id, commit_sha, commit_url, commit_type, scope, is_breaking,
     title, author_name, committed_at)
VALUES (NULL, sqlc.narg(commit_sha), sqlc.narg(commit_url), @commit_type,
        sqlc.narg(scope), @is_breaking, @title,
        sqlc.narg(author_name), sqlc.narg(committed_at))
ON CONFLICT DO NOTHING;

-- name: ListRoadmapReleaseEntries :many
-- Public-type entries for a release, grouped by commit_type for the
-- section render; ?scope= filter applied when non-empty.
SELECT * FROM roadmap_release_entries
WHERE release_id = @release_id
  AND commit_type IN ('feature','fix','ui','performance','security',
                      'translation','accessibility')
  AND (@scope::text = '' OR scope = @scope)
ORDER BY commit_type, committed_at DESC;

-- name: ListPublicUnreleasedEntries :many
SELECT * FROM roadmap_release_entries
WHERE release_id IS NULL
  AND commit_type IN ('feature','fix','ui','performance','security',
                      'translation','accessibility')
  AND (@scope::text = '' OR scope = @scope)
ORDER BY committed_at DESC
LIMIT 200;

-- name: CountUnreleasedEntries :one
-- Public-type count only — this backs the public "In development" card,
-- so it must match what ListPublicUnreleasedEntries renders.
SELECT count(*) FROM roadmap_release_entries
WHERE release_id IS NULL
  AND commit_type IN ('feature','fix','ui','performance','security',
                      'translation','accessibility');

-- name: AssignUnreleasedEntries :execrows
-- Publish absorb: sweep the whole queue into the release.
UPDATE roadmap_release_entries SET release_id = @release_id
WHERE release_id IS NULL;

-- name: ListAdminReleaseEntries :many
SELECT * FROM roadmap_release_entries
WHERE (@unreleased::bool = false OR release_id IS NULL)
  AND (@commit_type::text = '' OR commit_type = @commit_type)
  AND (@scope::text = '' OR scope = @scope)
ORDER BY committed_at DESC NULLS LAST
LIMIT @page_size OFFSET @page_offset;

-- name: UpdateReleaseEntry :one
UPDATE roadmap_release_entries SET
    commit_type = @commit_type, title = @title,
    scope = sqlc.narg(scope), item_id = sqlc.narg(item_id)
WHERE id = @id
RETURNING *;

-- name: DeleteReleaseEntry :exec
DELETE FROM roadmap_release_entries WHERE id = @id;
```

Also add `release_id` to `UpdateRoadmapItem` (sqlc.narg) and `release_version`,
`release_slug` (LEFT JOIN roadmap_releases) to `ListRoadmapChangelog`.

- [ ] **Step 4: Run migration + regen**

```bash
migrate -path migrations -database "$JC_DATABASE_URL" up  # or project make target
make sqlc-gen
```

Expected: `gen.RoadmapRelease` exists; `roadmap_items` model has `ReleaseID`.

- [ ] **Step 5: Commit**

```bash
git add migrations/000942_roadmap_releases.* internal/db/
git commit -m "feat(roadmap): roadmap_releases table + sqlc queries (ADR-135)"
```

---

### Task 3: Backend — release domain in `internal/roadmap`

**Files:**
- Modify: `internal/roadmap/types.go` (release types, lifecycle, support_state)
- Modify: `internal/roadmap/store.go` (release store methods)
- Modify: `internal/roadmap/service.go` (release service)
- Test: `internal/roadmap/service_test.go` (lifecycle + support_state table tests)

**Interfaces:**
- Produces:
  - `type ReleaseStatus string` — `ReleaseDraft|ReleasePublished|ReleaseYanked`
  - `type ReleaseSurface string` — `SurfacePlatform|SurfaceMobile`
  - `type ReleaseDTO struct { ID, Surface, Version, Slug, Title, Summary, SupportState string; ItemCount int32; IsESR bool; ReleasedAt, EOLAt, SupersededAt *time.Time }`
  - `type ReleaseListResponse struct { Releases []ReleaseDTO; UnreleasedCount int64 }` — `GET /roadmap/releases` payload
  - `type EntryDTO struct { ID, CommitType, Title, Scope, CommitSHA, CommitURL, AuthorName string; IsBreaking bool; CommittedAt *time.Time }`
  - `type ReleaseDetail struct { ReleaseDTO; Body *string; Items []ChangelogItem; Entries []EntryDTO }` — `Entries` ordered by commit_type (frontend groups into sections)
  - `func IsValidReleaseTransition(from, to ReleaseStatus) bool` — `draft→published`, `published→yanked`, `yanked→published`; no-op transitions permitted.
  - `func ComputeSupportState(r gen.RoadmapRelease, supersededAt *time.Time, now time.Time) string`
  - `Service.ListPublicReleases(ctx) (ReleaseListResponse, error)` — Valkey `jc:roadmap:releases` cached
  - `Service.GetReleaseBySlug(ctx, slug) (ReleaseDetail, error)`
  - `Service.AdminListReleases/CreateRelease/UpdateRelease/SetReleaseStatus/DeleteRelease/RestoreRelease`
  - `Service.ReplaceReleaseItems(ctx, releaseID, itemIDs []pgtype.UUID) error` — validates each item `status='shipped'` else `ErrItemNotShipped`
  - `type GitHubCommit struct { ID, Message, URL, Timestamp string; Author struct{ Name string }; Added, Modified, Removed []string }` — mirrors the GitHub push-event commit object so `github.event.commits` JSON decodes directly
  - `Service.IngestCommits(ctx, []GitHubCommit) (created, skipped int, err error)`
  - `Service.ListUnreleasedEntries(ctx, scope string)` / `Service.AdminListEntries(ctx, filter)` / `Service.UpdateReleaseEntry` / `Service.DeleteReleaseEntry`
  - `ErrItemNotShipped = errors.New("item_not_shipped")` → 409; `ErrInvalidReleaseTransition` aliases `errpkg.ErrInvalidStatusTransition`; version format: mobile requires `regexp ^v?\d+\.\d+\.\d+` → `400 invalid_version`.

- [ ] **Step 1: Write failing tests** — table test for `IsValidReleaseTransition`, table test for `ComputeSupportState` (draft/yanked/upcoming/latest/esr/eol/supported/superseded-platform), attach-non-shipped → `ErrItemNotShipped`, mobile version regex reject, `ParseConventionalCommit` table test (every type alias, scope extraction, `!` breaking flag, non-conforming → `other`), `ResolveScope` test (flag match / `{scope}.enabled` / `feature.{scope}` / path-map longest-prefix / raw scope fallback).

- [ ] **Step 2: Run tests, confirm FAIL** — `go test ./internal/roadmap/ -run Release -v`

- [ ] **Step 3: Implement types + store + service.** Mirror existing item flows: slug generation + shortid dedupe on 23505; every release/entry mutation deletes `jc:roadmap:releases`. `ReplaceReleaseItems`: `ClearRoadmapReleaseItems` + `SetRoadmapItemRelease` per id inside `db.WithTx`, collect per-item execrows=0 → `ErrItemNotShipped`. `IngestCommits(commits []GitHubCommit)`: parse → resolve scope → `InsertReleaseEntry`, count created/skipped by execrows. `SetReleaseStatus` → `published` with `absorb_unreleased` runs `AssignUnreleasedEntries` in the same tx. New funcs: `ParseConventionalCommit(subject string) (commitType, scope, title string, breaking bool)` (regex `^(\w+)(\(([^)]*)\))?(!)?:\s*(.+)$` + alias map), `ResolveScope(ctx, scope string, files []string) string` (flag lookup → `{scope}.enabled`/`feature.{scope}` → `roadmap.commit_scope_paths` longest-prefix → raw scope).

- [ ] **Step 4: Tests PASS** — `go test ./internal/roadmap/ -v`

- [ ] **Step 5: Commit** — `git commit -m "feat(roadmap): release domain — lifecycle, support_state, item attach"`

---

### Task 4: Backend — handlers + route registration

**Files:**
- Modify: `internal/roadmap/handlers.go` (10 new handlers + `release_id` on item PATCH)
- Modify: `internal/server/server.go` (~L5178 public, ~L5218 admin)
- Test: `internal/roadmap/handlers_test.go`

- [ ] **Step 1: Write failing handler tests** — public list 404-when-flag-off already covered by gate; cover: get-by-slug 404 on draft, publish stamps `released_at`, publish with `absorb_unreleased` sweeps the queue, attach non-shipped → `409 item_not_shipped`, mobile non-semver → `400 invalid_version`, ingest dedupes a repeated sha (`skipped=1`), entry PATCH retype.

- [ ] **Step 2: Implement handlers** mirroring `AdminListItems`/`AdminSetStatus` shapes; `PutReleaseItems` decodes `{item_ids:[]}` → `ReplaceReleaseItems`; `IngestEntries` decodes `{commits: []GitHubCommit}` → `IngestCommits` → `{created, skipped}`; `ListEntries/UpdateEntry/DeleteEntry` for the admin queue; `GetUnreleasedEntries` public; `GetReleasesFeed` renders RSS 2.0 XML inline (`Content-Type: application/rss+xml`; channel title `{SiteName} Releases`, items = published releases, link = `{SiteURL}/roadmap/releases/{slug}`, pubDate = released_at, description = summary).

- [ ] **Step 3: Register routes** inside the existing `features.Gate(featureChecker, "roadmap.enabled")` group in `server.go` — public `GET /roadmap/releases`, `GET /roadmap/releases/feed.xml`, `GET /roadmap/releases/unreleased` (static segments registered **before** `{slug}` wildcard), `GET /roadmap/releases/{slug}`; admin routes per the Task 1 route table with the same `auth.RequireAuth + rbac.RequirePermission` middleware stacks as the item routes.

- [ ] **Step 4: `go test ./internal/roadmap/ ./internal/server/ -v` PASS; `go build ./...` clean.**

- [ ] **Step 5: Commit** — `git commit -m "feat(roadmap): release REST routes — public timeline/detail/feed + admin CRUD"`

---

### Task 5: Agent tools — `roadmap.release[s].*`

**Files:**
- Create: `internal/ai/tools_releases.go`
- Modify: `internal/ai/tools_register.go` (~L358, after roadmap block)
- Modify: `internal/ai/tools_deps.go` (adapter methods if the deps interface needs release passthroughs)
- Test: `internal/ai/tools_releases_test.go` (mirror `tools_roadmap_test.go`)

- [ ] **Step 1: Write failing tests** — arg decoding (pointer-vs-absent per `roadmapItemArgs` pattern), `attach_items` with `item_ids` and with `shipped_since`/`shipped_until` window (window mode: `roadmap.items.list status=shipped` in window → ids → `ReplaceReleaseItems`), `entries.ingest` with a commits array, error mapping via `roadmapErrResult`.

- [ ] **Step 2: Implement** — handlers `RoadmapReleasesList/Get/Create/Update/SetStatus/AttachItems` + `RoadmapEntriesList/EntriesIngest`; `set_status` validates `draft|published|yanked` only and accepts `absorb_unreleased`; create accepts `status:"published"` as direct-publish shortcut (same as item create's shipped shortcut). Also add `ReleaseID *string` to `roadmapItemArgs`/`toInput` so `roadmap.item.update` (and the REST item PATCH) can link a shipped item to a release.

- [ ] **Step 3: Register** the eight handlers in `tools_register.go` under the §113 block.

- [ ] **Step 4: `go test ./internal/ai/ -run Release -v` PASS; `go build ./...` clean.**

- [ ] **Step 5: Commit** — `git commit -m "feat(ai): roadmap release agent tools (§103/§119)"`

---

### Task 6: Frontend — public releases pages (`apps/main`)

**Files:**
- Create: `apps/main/src/routes/(app)/roadmap/releases/+page.server.ts`, `+page.svelte`
- Create: `apps/main/src/routes/(app)/roadmap/releases/[slug]/+page.server.ts`, `+page.svelte`
- Modify: `apps/main/src/lib/types.ts` (`RoadmapRelease`, `RoadmapReleaseDetail`; `release_version`/`release_slug` on `RoadmapChangelogItem`)
- Modify: `apps/main/src/routes/(app)/roadmap/changelog/+page.svelte` (release badge link)
- Modify: `apps/main/src/lib/i18n/en.json`, `fr.json` (`roadmap.releases.*` keys)
- Check: `apps/main/src/lib/components/roadmap/` for reusable bits (`StatusBadge.svelte` → a `ReleaseStateBadge`)

- [ ] **Step 1: `+page.server.ts` for `/roadmap/releases`** — same gate as changelog (`features?.['roadmap.enabled']` → 404), `apiGet<RoadmapRelease[]>(ctx, '/roadmap/releases')`, `loadError` fallback.

- [ ] **Step 2: `/roadmap/releases/+page.svelte`** — the Discourse anatomy:
  - **Timeline band viz:** horizontal month axis spanning min(released_at)−1mo → max(eol_at, now)+2mo; CSS grid `grid-template-columns: repeat(N months, 1fr)`; each release = one row band positioned by `grid-column: startMonth / span M`, color by `support_state` (latest=primary, supported=green, esr=violet, eol=grey, upcoming=dashed border, yanked=red strike). Bands link to the detail page. This is plain grid — no LayerChart needed for a Gantt strip.
  - **Surface tabs** (Platform / Mobile) + release cards below: version, title, support-state chip, released/EOL dates, item count, summary.
- [ ] **Step 3: `[slug]` page** — header (version + state chip + dates), markdown `body` (same renderer the item detail uses), shipped-items list styled like the changelog `ul`, then **commit-entry sections**: group `entries` by `commit_type` under headed sections (Features / Fixes / UI / Performance / Security / Translations / Accessibility — i18n keys `roadmap.releases.types.*`), each entry a row with scope chip + title + commit link icon (external `commit_url`). `?scope=` param filters both lists and renders scope chips as toggle links.
- [ ] **Step 4: `/roadmap/releases/unreleased` page** — "In development" list of queued public-type entries grouped by `commit_type`; `/roadmap/releases` index shows a card linking here when `unreleased_count > 0`.
- [ ] **Step 5: changelog badge** — in `changelog/+page.svelte` item row, `item.release_version && <a href="/roadmap/releases/{release_slug}">` chip.
- [ ] **Step 6: `bun run check` clean; verify on the `roadmap.japa.to` host shell too (same pages render — no extra work, just confirm `isRoadmapHost` nav links to releases).**

- [ ] **Step 7: Commit** — `git commit -m "feat(main): /roadmap/releases timeline + per-release changelog pages"`

---

### Task 7: Admin UI — releases management (`apps/admin`)

**Files:**
- Create: `apps/admin/src/routes/roadmap/releases/+page.svelte` (EntityPage scaffold, DataTable spec: sort/search/filter/paginate/actions/bulk/create per jc-admin-develop)
- Modify: `apps/admin/src/routes/roadmap/+page.svelte` or items page — `release_id` select on item edit (shipped items only)

- [ ] **Step 1: Releases DataTable** — columns: version, title, surface, status, support_state, released_at, eol_at, item_count; filters: surface, status; row actions: edit, publish/yank (`PATCH …/status`), attach items, restore, delete.
- [ ] **Step 2: Attach-items dialog** — multi-select of shipped items (fetch `GET /admin/roadmap/items?status=shipped`) → `PUT …/items {item_ids}`.
- [ ] **Step 3: Create/edit form** — surface select toggles version validation hint (semver for mobile); `is_esr`/`eol_at` fields visible only when `surface=mobile`.
- [ ] **Step 4: Entries queue view** — `apps/admin/src/routes/roadmap/releases/entries/+page.svelte` DataTable over `GET /admin/roadmap/releases/entries` (`?unreleased=true` default): columns type, scope, title, sha (linked), committed_at, release; row actions: retype (select), retitle, link item, delete. This is where noise gets filtered and `other`-typed commits get promoted.
- [ ] **Step 5: `bun run check` clean; RBAC gates verified (`roadmap.view`/`edit`/`moderate`/`delete` per control).**

- [ ] **Step 6: Commit** — `git commit -m "feat(admin): roadmap releases management"`

---

### Task 8: Commit-ingest pipeline — CI workflow + backfill script

**Files:**
- Create: `.github/workflows/changelog-ingest.yml`
- Create: `scripts/changelog-backfill.sh`

- [ ] **Step 1: GitHub Actions workflow** — fires on every push to `main`, calls `roadmap.entries.ingest` through the §119 MCP bridge. CI uses `tools/call` rather than the REST route because X-API-Key does not satisfy `auth.RequireAuth` on `/admin/*` — MCP key auth resolves the key to its owning principal and enforces `roadmap.edit` via the tool's `required_permission`:

```yaml
name: Changelog ingest
on:
  push:
    branches: [main]
jobs:
  ingest:
    runs-on: ubuntu-latest
    steps:
      - name: Ingest commits into release entries
        env:
          API_BASE: ${{ vars.JC_API_BASE }}
          INGEST_KEY: ${{ secrets.JC_CHANGELOG_INGEST_KEY }}
          COMMITS: ${{ toJSON(github.event.commits) }}
        run: |
          jq -n --argjson c "$COMMITS" \
            '{jsonrpc:"2.0",id:1,method:"tools/call",
              params:{name:"roadmap.entries.ingest",arguments:{commits:$c}}}' |
          curl -fsS -X POST "$API_BASE/api/v1/mcp" \
            -H "X-API-Key: $INGEST_KEY" -H "Content-Type: application/json" -d @-
```

`JC_CHANGELOG_INGEST_KEY` is an API key carrying `agent.use` + `agent.write`
scopes, owned by a principal holding `roadmap.edit` (mint via the control
panel or `POST /api/v1/control/api-keys`). `github.event.commits` decodes
into `[]GitHubCommit` as-is (GitHub's `id` field is the sha).

- [ ] **Step 2: Backfill script** — `scripts/changelog-backfill.sh <since>` emits `git log` in the GitHub commit shape and posts in batches of 100:

```bash
#!/usr/bin/env bash
# Usage: JC_API_BASE=https://api.japa.to JC_INGEST_KEY=<key> ./scripts/changelog-backfill.sh "3 months ago"
set -euo pipefail
SINCE="${1:-3 months ago}"
git log --since="$SINCE" --pretty=format:'%H%x1f%s%x1f%an%x1f%aI%x1f' --name-only |
python3 -c '
import sys, json
commits, cur = [], None
for line in sys.stdin:
    line = line.rstrip("\n")
    if "\x1f" in line:
        if cur: commits.append(cur)
        sha, subj, author, ts, _ = line.split("\x1f")
        cur = {"id": sha, "message": subj, "url": "", "timestamp": ts,
               "author": {"name": author}, "added": [], "modified": [], "removed": []}
    elif line and cur:
        cur["modified"].append(line)
if cur: commits.append(cur)
for i in range(0, len(commits), 100):
    print(json.dumps({"commits": commits[i:i+100]}))
' | while read -r batch; do
  jq -n --argjson c "$batch" \
    '{jsonrpc:"2.0",id:1,method:"tools/call",
      params:{name:"roadmap.entries.ingest",arguments:$c}}' |
  curl -fsS -X POST "$JC_API_BASE/api/v1/mcp" \
    -H "X-API-Key: $JC_INGEST_KEY" -H "Content-Type: application/json" -d @-
done
```

- [ ] **Step 3: Run backfill** against the deployed API (`"3 months ago"` ≈ 1,770 commits — expect `created`+`skipped` to match after parse; verify a few entries land with correct `commit_type`/`scope` in the admin entries view).

- [ ] **Step 4: Commit** — `git commit -m "ci(roadmap): push-event commit ingest + changelog backfill script"` — note this very commit lands in the unreleased queue once the workflow is live.

---

### Task 9: Sitemap + verification sweep

**Files:**
- Modify: `internal/seo/sitemap.go` (roadmap sitemap ~L572–593: add `/roadmap/releases` + per-release URLs from `ListRoadmapReleaseSlugs`)

- [ ] **Step 1: Wire release slugs into the roadmap sitemap source.**
- [ ] **Step 2: Full sweep** — `go build ./... && go test ./internal/roadmap/ ./internal/ai/ ./internal/seo/`; `bun run check` in `apps/main` + `apps/admin`; confirm `feed.xml` route order (`/roadmap/releases/feed.xml` before `/{slug}`).
- [ ] **Step 3: Manual smoke** — create a platform wave + a mobile semver release via the agent tools (`POST /api/v1/mcp` or `/admin` routes), attach shipped items, ingest a few test commits (unreleased queue fills + `unreleased_count` on the index), publish with `absorb_unreleased` (queue empties, entries land on the release), verify `/roadmap/releases` timeline + `/roadmap/releases/{slug}` grouped sections + `/roadmap/releases/unreleased` + `feed.xml` + changelog badges + `?scope=` filter.
- [ ] **Step 4: Commit** — `git commit -m "feat(roadmap): releases in sitemap"`

## Out of scope (note for reviewer)

- `roadmap:release_notify` Asynq fan-out to voters on publish — follow-up.
- Mobile `eol_at` → min-supported-version enforcement for `device_tokens` — follow-up.
- Auto-cutting a release on git tag — possible today via the MCP tools; a dedicated CI workflow is follow-up.
- Join table for multi-surface items — YAGNI per ADR-135.
- Non-conventional commits (~24% of history) land as `other`/`internal_only` — they never render publicly unless an admin retypes them. Widening adoption of conventional commits improves coverage automatically; no parser changes needed.
