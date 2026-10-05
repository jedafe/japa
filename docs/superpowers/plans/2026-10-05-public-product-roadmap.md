# Public Product Roadmap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a public product roadmap (`/roadmap` + `roadmap.japaconnect.com`) with voting, submissions triage, comments, and admin management — leading with Spaces & Content Creation as the flagship item.

**Architecture:** Dedicated `internal/roadmap` Go module (Handlers → Service → Store, sqlc) backed by `roadmap_items` / `roadmap_categories` / `roadmap_votes` tables. Each published item owns a hidden `contents` row (`content_type='roadmap'`) so §49 comments work unchanged. Public pages in `apps/main`; admin management in `apps/admin` via the `EntityPage` scaffold; `roadmap.japaconnect.com` via host detection (ADR-063 precedent).

**Tech Stack:** Go + chi + sqlc/pgx + Valkey + Asynq; SvelteKit 2 + Svelte 5 runes + Tailwind v4; bun for all JS tooling.

## Global Constraints

- **JEOS first:** section file + ADR + all catalog updates land before code (AGENTS.md).
- All commits go to `main`. Never edit `internal/db/gen/*` — edit `internal/db/queries/*.sql` then `make sqlc-gen` (Rule 2).
- Configurable values via `system_settings` / `JC_*` env — no magic values (Rule 3).
- Valkey keys `jc:`-prefixed and catalogued (Rule 4). Error envelope via `errpkg.WriteError`/`WriteJSON`/`HandleServiceError` (Rule 5).
- Fan-out tasks fire-and-forget (Rule 8). Soft deletes only; hard delete via `?force=true` + `admin.hard_delete` (Rule 11).
- All frontend data from the API (Rule 16). No AI attribution anywhere (Rule 17). `bun`/`bunx` only (Rule 18).
- New Asynq task names must be registered in §11 catalog + `internal/worker/tasks.go` + `enqueuer.go` (Rule 7).
- `content_hash` computed before every `contents` save (Rule 12).
- Feature flag `roadmap.enabled` (default `true`) gates every new route via `features.Gate` → 404 when disabled (ADR-042).

---

### Task 1: JEOS docs — section, ADR, catalog updates

**Files:**
- Create: `.jeos/sections/113-public-product-roadmap.md`
- Create: `.jeos/adr/ADR-127-public-product-roadmap.md`
- Modify: `.jeos/JEOS.md` (section index row + ADR index row)
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/conventions/permission-key-naming.md`
- Modify: `.jeos/schemas/MIGRATION-ORDER.md`
- Modify: `.jeos/sections/11-job-queue-asynq.md`
- Modify: `docs/superpowers/specs/2026-10-05-public-product-roadmap-design.md` (drop `body_html`/`comment_count` columns; add `roadmap.vote`/`roadmap.submit` nodes + `roadmap.shipped_window_days` setting)

**Interfaces:**
- Produces: §113 as the implementation source of truth; catalog names used by every later task.

- [ ] **Step 1: Write `sections/113-public-product-roadmap.md`**

Structure it like `sections/38-ticket-system.md` (read it for the house style: status machine, tables, routes, settings, RBAC, Valkey keys). Content must cover: purpose (incremental launch comms, Spaces & Content Creation flagship), the status lifecycle `pending → under_review → planned → in_progress → shipped` (+ `declined`, `archived`), the three tables (schema in Task 2), the shadow-`contents`-row comments mechanism, public + admin routes (Task 6 list), RBAC nodes, Valkey key `jc:roadmap:board`, system settings `roadmap.submissions_enabled` / `roadmap.custom_domain` / `roadmap.board_cache_ttl_seconds` / `roadmap.shipped_window_days`, the `roadmap:ship_notify` task, and the `roadmap.japaconnect.com` host-detection mode.

- [ ] **Step 2: Write `adr/ADR-127-public-product-roadmap.md`**

Decisions to record: (a) dedicated module vs contents-native (feed/search pollution, vote semantics, `content_hash` collision risk), (b) shadow `contents` row for §49 comments reuse, (c) host-detection reuse of the ADR-063 `isUniversityHost` pattern for `roadmap.japaconnect.com`, (d) logged-in-only voting, (e) self-healing `vote_count` via recount query instead of increment counters.

- [ ] **Step 3: Update `JEOS.md`**

Add a section-index row `| 113 | sections/113-public-product-roadmap.md | ...` and an ADR row `| 127 | adr/ADR-127-public-product-roadmap.md | ...` matching the existing table formats.

- [ ] **Step 4: Update `api-routes-catalog.md`**

Add a `## Roadmap` block (base path `/api/v1`):

```
GET    /roadmap/board                        [public] Board: items grouped by status + categories (+ has_voted when authed)
GET    /roadmap/changelog                    [public] Shipped items grouped by ship date
GET    /roadmap/items/{slug}                 [public] Item detail incl. content_id for comments
POST   /roadmap/items/{id}/vote              [perm: roadmap.vote] Cast vote
DELETE /roadmap/items/{id}/vote              [perm: roadmap.vote] Remove vote
POST   /roadmap/submissions                  [perm: roadmap.submit] Submit feature request (status=pending)
GET    /admin/roadmap/items                  [perm: roadmap.view]
POST   /admin/roadmap/items                  [perm: roadmap.create]
GET    /admin/roadmap/items/{id}             [perm: roadmap.view]
PATCH  /admin/roadmap/items/{id}             [perm: roadmap.edit]
DELETE /admin/roadmap/items/{id}             [perm: roadmap.delete; ?force=true + admin.hard_delete]
PATCH  /admin/roadmap/items/{id}/status      [perm: roadmap.moderate]
POST   /admin/roadmap/items/reorder          [perm: roadmap.edit]
GET    /admin/roadmap/submissions            [perm: roadmap.moderate] pending queue
GET    /admin/roadmap/categories             [perm: roadmap.view]
POST   /admin/roadmap/categories             [perm: roadmap.edit]
PATCH  /admin/roadmap/categories/{id}        [perm: roadmap.edit]
DELETE /admin/roadmap/categories/{id}        [perm: roadmap.edit]
```

- [ ] **Step 5: Update `valkey-keys-catalog.md`**

Add row:

```
| `jc:roadmap:board` | JSON-serialized public roadmap board payload (statuses × items + categories) | `roadmap.board_cache_ttl_seconds` (default 60s) | any roadmap item/category/vote mutation |
```

- [ ] **Step 6: Update `system-settings-catalog.md`**

```
| `roadmap.submissions_enabled` | `true` | bool | Public feature-request submissions open |
| `roadmap.custom_domain` | `` | string | Extra hostnames that render the roadmap surface (comma-separated; checked by apps/main host detection) |
| `roadmap.board_cache_ttl_seconds` | `60` | int | TTL for jc:roadmap:board |
| `roadmap.shipped_window_days` | `90` | int | How many days a shipped item stays on the public board |
```

- [ ] **Step 7: Update `permission-key-naming.md`** — add `roadmap.vote`, `roadmap.submit`, `roadmap.view`, `roadmap.create`, `roadmap.edit`, `roadmap.delete`, `roadmap.moderate` in the domain-prefix format used there.

- [ ] **Step 8: Update `schemas/MIGRATION-ORDER.md`** — append `000795_roadmap` after `000794` in the ordered list; note FK dependencies: `roadmap_items → roadmap_categories, contents, users`; `roadmap_votes → roadmap_items, users`.

- [ ] **Step 9: Update `sections/11-job-queue-asynq.md`** — add `roadmap:ship_notify` to the task catalog: queue `default`, payload `{item_id, title, slug}`, notifies all voters + `submitted_by` (in-app notification type `roadmap_shipped`).

- [ ] **Step 10: Sync the spec file** — in `docs/superpowers/specs/2026-10-05-public-product-roadmap-design.md` remove `body_html` and `comment_count` from the `roadmap_items` column table (frontend renders markdown like comments; comment counts come from the comments API), add `roadmap.vote` + `roadmap.submit` to the RBAC node list, and `roadmap.shipped_window_days` to the settings list.

- [ ] **Step 11: Commit**

```bash
git add .jeos/ docs/superpowers/specs/2026-10-05-public-product-roadmap-design.md
git commit -m "jeos: section 113 + ADR-127 public product roadmap, catalog entries"
```

---

### Task 2: Migration `000795_roadmap`

**Files:**
- Create: `migrations/000795_roadmap.up.sql`
- Create: `migrations/000795_roadmap.down.sql`

**Interfaces:**
- Produces: tables `roadmap_categories`, `roadmap_items`, `roadmap_votes`; permission nodes `roadmap.*`; role grants for `member`/`pro_member`/`business_member` (vote+submit) and admin roles (all nodes). Role slugs verified in `000735_baseline_seed.up.sql`: `super_admin`, `super_admin_role`, `admin-group-role`, `member`, `pro_member`, `business_member`, `guest`.

- [ ] **Step 1: Write the up migration**

```sql
-- 000795_roadmap.up.sql
--
-- Public product roadmap (§113): community-votable, admin-curated feature
-- roadmap. Items carry a hidden contents row (content_type='roadmap') so the
-- §49 comments system serves item discussion threads. vote_count is a
-- denormalized counter kept in sync by a recount query (ADR-127).

BEGIN;

CREATE TABLE roadmap_categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    name        text NOT NULL,
    description text,
    color       text,
    sort_order  int  NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TABLE roadmap_items (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title            text NOT NULL,
    slug             text NOT NULL UNIQUE,
    summary          text,
    body             text,
    category_id      uuid REFERENCES roadmap_categories(id) ON DELETE SET NULL,
    status           text NOT NULL DEFAULT 'under_review'
                     CHECK (status IN ('pending','under_review','planned','in_progress','shipped','declined','archived')),
    sort_order       int  NOT NULL DEFAULT 0,
    is_flagship      bool NOT NULL DEFAULT false,
    target_window    text,
    feature_flag_key text,
    content_id       uuid REFERENCES contents(id) ON DELETE SET NULL,
    vote_count       int  NOT NULL DEFAULT 0,
    submitted_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    approved_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    shipped_at       timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);

CREATE INDEX idx_roadmap_items_status ON roadmap_items (status, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX idx_roadmap_items_shipped ON roadmap_items (shipped_at DESC) WHERE deleted_at IS NULL AND status = 'shipped';

CREATE TABLE roadmap_votes (
    item_id    uuid NOT NULL REFERENCES roadmap_items(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, user_id)
);

INSERT INTO roadmap_categories (slug, name, description, color, sort_order) VALUES
    ('spaces-content', 'Spaces & Content', 'Spaces, content creation, and publishing', '#6366f1', 0),
    ('community',      'Community',        'Forums, groups, and social features',      '#22c55e', 1),
    ('tools',          'Tools',            'Calculators, checklists, and guides',      '#f59e0b', 2),
    ('mobile',         'Mobile',           'iOS and Android apps',                     '#06b6d4', 3),
    ('platform',       'Platform',         'API, infrastructure, and developer experience', '#a855f7', 4);

INSERT INTO permissions (key, description) VALUES
    ('roadmap.vote',     'Vote on public roadmap items'),
    ('roadmap.submit',   'Submit feature requests to the public roadmap'),
    ('roadmap.view',     'View roadmap items in the admin panel'),
    ('roadmap.create',   'Create roadmap items'),
    ('roadmap.edit',     'Edit roadmap items and categories'),
    ('roadmap.delete',   'Delete roadmap items'),
    ('roadmap.moderate', 'Triage submissions, change statuses, and inline-edit the public roadmap')
ON CONFLICT (key) DO NOTHING;

-- Members may vote and submit; tier members inherit the same abilities.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member')
  AND p.key IN ('roadmap.vote', 'roadmap.submit')
ON CONFLICT DO NOTHING;

-- Admin roles get every roadmap node.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin', 'super_admin_role', 'admin-group-role')
  AND p.key LIKE 'roadmap.%'
ON CONFLICT DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000795_roadmap.down.sql
BEGIN;

DELETE FROM role_permissions WHERE permission_id IN
    (SELECT id FROM permissions WHERE key LIKE 'roadmap.%');
DELETE FROM permissions WHERE key LIKE 'roadmap.%';

DROP TABLE IF EXISTS roadmap_votes;
DROP TABLE IF EXISTS roadmap_items;
DROP TABLE IF EXISTS roadmap_categories;

COMMIT;
```

- [ ] **Step 3: Run the migration**

```bash
make migrate   # or: go run ./cmd/migrate up — use the repo's existing command
```

Expected: clean apply; `psql -c '\d roadmap_items'` shows the table.

- [ ] **Step 4: Commit**

```bash
git add migrations/000795_roadmap.up.sql migrations/000795_roadmap.down.sql
git commit -m "db: roadmap tables, categories seed, and roadmap.* permission nodes (§113)"
```

---

### Task 3: sqlc queries — `internal/db/queries/roadmap.sql`

**Files:**
- Create: `internal/db/queries/roadmap.sql`
- Regenerate: `internal/db/gen/roadmap.sql.go` (via `make sqlc-gen`)

**Interfaces:**
- Produces gen functions consumed by `internal/roadmap/store.go` (Task 5): `CreateRoadmapItem`, `GetRoadmapItemByID`, `GetRoadmapItemBySlug`, `ListPublicRoadmapItems`, `ListRoadmapItemsByStatus`, `ListAdminRoadmapItems`, `CountAdminRoadmapItems`, `UpdateRoadmapItem`, `UpdateRoadmapItemStatus`, `SetRoadmapItemContent`, `SoftDeleteRoadmapItem`, `ReorderRoadmapItems`, `ListRoadmapCategories`, `CreateRoadmapCategory`, `UpdateRoadmapCategory`, `SoftDeleteRoadmapCategory`, `TryCreateRoadmapVote`, `DeleteRoadmapVote`, `SyncRoadmapVoteCount`, `ListRoadmapVoterIDs`, `ListRoadmapSitemapItems`, `CreateRoadmapShadowContent`.

- [ ] **Step 1: Write the query file**

```sql
-- name: CreateRoadmapItem :one
INSERT INTO roadmap_items (
    title, slug, summary, body, category_id, status, sort_order,
    is_flagship, target_window, feature_flag_key, content_id,
    submitted_by, approved_by
) VALUES (
    @title, @slug, sqlc.narg(summary), sqlc.narg(body),
    sqlc.narg(category_id), @status, @sort_order,
    @is_flagship, sqlc.narg(target_window), sqlc.narg(feature_flag_key),
    sqlc.narg(content_id), sqlc.narg(submitted_by), sqlc.narg(approved_by)
) RETURNING *;

-- name: GetRoadmapItemByID :one
SELECT * FROM roadmap_items WHERE id = @id;

-- name: GetRoadmapItemBySlug :one
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id
WHERE i.slug = @slug AND i.deleted_at IS NULL;

-- name: ListPublicRoadmapItems :many
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color,
       (v.user_id IS NOT NULL) AS has_voted
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id
LEFT JOIN roadmap_votes v ON v.item_id = i.id AND v.user_id = sqlc.narg(viewer_id)
WHERE i.status = ANY(@statuses::text[]) AND i.deleted_at IS NULL
  AND (sqlc.narg(category_slug)::text = '' OR c.slug = sqlc.narg(category_slug)::text)
  AND (i.status <> 'shipped' OR i.shipped_at > now() - make_interval(days => @shipped_window_days))
ORDER BY i.is_flagship DESC, i.sort_order ASC, i.vote_count DESC, i.created_at DESC;

-- name: ListRoadmapItemsByStatus :many
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id
WHERE i.status = @status AND i.deleted_at IS NULL
ORDER BY i.sort_order ASC, i.created_at DESC;

-- name: ListAdminRoadmapItems :many
SELECT i.*, c.name AS category_name
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id
WHERE (sqlc.narg(status)::text = '' OR i.status = sqlc.narg(status)::text)
  AND (sqlc.narg(category_id)::uuid IS NULL OR i.category_id = sqlc.narg(category_id)::uuid)
  AND (sqlc.narg(search)::text = '' OR i.title ILIKE '%' || sqlc.narg(search)::text || '%' OR i.summary ILIKE '%' || sqlc.narg(search)::text || '%')
  AND (@include_deleted::bool OR i.deleted_at IS NULL)
ORDER BY
  CASE WHEN sqlc.narg(sort)::text = 'votes' THEN i.vote_count END DESC NULLS LAST,
  CASE WHEN sqlc.narg(sort)::text = 'title' THEN i.title END ASC NULLS LAST,
  CASE WHEN sqlc.narg(sort)::text IS NULL OR sqlc.narg(sort)::text = '' OR sqlc.narg(sort)::text = 'recent' THEN i.created_at END DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAdminRoadmapItems :one
SELECT count(*) FROM roadmap_items i
WHERE (sqlc.narg(status)::text = '' OR i.status = sqlc.narg(status)::text)
  AND (sqlc.narg(category_id)::uuid IS NULL OR i.category_id = sqlc.narg(category_id)::uuid)
  AND (sqlc.narg(search)::text = '' OR i.title ILIKE '%' || sqlc.narg(search)::text || '%' OR i.summary ILIKE '%' || sqlc.narg(search)::text || '%')
  AND (@include_deleted::bool OR i.deleted_at IS NULL);

-- name: UpdateRoadmapItem :one
UPDATE roadmap_items SET
    title = @title, slug = @slug,
    summary = sqlc.narg(summary), body = sqlc.narg(body),
    category_id = sqlc.narg(category_id),
    sort_order = @sort_order, is_flagship = @is_flagship,
    target_window = sqlc.narg(target_window),
    feature_flag_key = sqlc.narg(feature_flag_key),
    approved_by = sqlc.narg(approved_by),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: UpdateRoadmapItemStatus :one
UPDATE roadmap_items SET
    status = @status,
    shipped_at = CASE WHEN @status = 'shipped' THEN now() ELSE shipped_at END,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetRoadmapItemContent :exec
UPDATE roadmap_items SET content_id = @content_id, updated_at = now() WHERE id = @id;

-- name: SoftDeleteRoadmapItem :exec
UPDATE roadmap_items SET deleted_at = now(), updated_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: HardDeleteRoadmapItem :exec
DELETE FROM roadmap_items WHERE id = @id AND deleted_at IS NOT NULL;

-- name: ReorderRoadmapItems :exec
UPDATE roadmap_items SET sort_order = v.ord
FROM (SELECT unnest(@ids::uuid[]) AS id, unnest(@orders::int[]) AS ord) v
WHERE roadmap_items.id = v.id;

-- name: ListRoadmapCategories :many
SELECT * FROM roadmap_categories
WHERE deleted_at IS NULL
ORDER BY sort_order ASC, name ASC;

-- name: ListAdminRoadmapCategories :many
SELECT * FROM roadmap_categories
ORDER BY sort_order ASC, name ASC;

-- name: GetRoadmapCategoryByID :one
SELECT * FROM roadmap_categories WHERE id = @id;

-- name: CreateRoadmapCategory :one
INSERT INTO roadmap_categories (slug, name, description, color, sort_order)
VALUES (@slug, @name, sqlc.narg(description), sqlc.narg(color), @sort_order)
RETURNING *;

-- name: UpdateRoadmapCategory :one
UPDATE roadmap_categories SET
    slug = @slug, name = @name,
    description = sqlc.narg(description), color = sqlc.narg(color),
    sort_order = @sort_order, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SoftDeleteRoadmapCategory :exec
UPDATE roadmap_categories SET deleted_at = now(), updated_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: TryCreateRoadmapVote :one
INSERT INTO roadmap_votes (item_id, user_id) VALUES (@item_id, @user_id)
ON CONFLICT (item_id, user_id) DO NOTHING
RETURNING item_id;

-- name: DeleteRoadmapVote :execrows
DELETE FROM roadmap_votes WHERE item_id = @item_id AND user_id = @user_id;

-- name: SyncRoadmapVoteCount :exec
UPDATE roadmap_items SET vote_count = (
    SELECT count(*) FROM roadmap_votes v WHERE v.item_id = @item_id
) WHERE id = @item_id;

-- name: ListRoadmapVoterIDs :many
SELECT user_id FROM roadmap_votes WHERE item_id = @item_id;

-- name: ListRoadmapSitemapItems :many
SELECT slug, updated_at FROM roadmap_items
WHERE status IN ('under_review','planned','in_progress','shipped') AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- name: CreateRoadmapShadowContent :one
INSERT INTO contents (
    author_id, content_type, title, slug, body, body_html,
    field_values, content_hash, seo_indexable, status, visibility,
    read_time_minutes, published_at
) VALUES (
    @author_id, 'roadmap', @title, @slug, @body, '',
    '{}'::jsonb, sqlc.narg(content_hash), false, 'published', 'public',
    1, now()
) RETURNING id;
```

- [ ] **Step 2: Regenerate**

```bash
make sqlc-gen
```

Expected: `internal/db/gen/roadmap.sql.go` generated; `go build ./internal/db/gen` passes.

- [ ] **Step 3: Commit**

```bash
git add internal/db/queries/roadmap.sql internal/db/gen/roadmap.sql.go
git commit -m "db: roadmap sqlc queries (§113)"
```

---

### Task 4: `internal/roadmap` — types + store

**Files:**
- Create: `internal/roadmap/types.go`
- Create: `internal/roadmap/store.go`
- Create: `internal/roadmap/types_test.go`

**Interfaces:**
- Produces consumed by Task 5 (service) and Task 6 (handlers):
  - `type ItemStatus string` + `StatusPending/UnderReview/Planned/InProgress/Shipped/Declined/Archived`
  - `func IsValidTransition(from, to ItemStatus) bool`
  - `var PublicStatuses = []string{"under_review","planned","in_progress","shipped"}`
  - `type BoardColumn struct{ Status, Label string; Items []BoardItem }`
  - `type BoardItem` (see below), `type Board struct{ Columns []BoardColumn; Categories []CategoryDTO }`
  - `type ItemDetail` (board fields + `Body`, `ContentID`, `CreatedAt`)
  - `type Store` + `NewStore(read, write *gen.Queries)` + methods mirroring gen queries
  - `ErrAlreadyVoted`, `ErrNotVoted` sentinels
  - `DefaultPageSize = 20`, `MaxPageSize = 100`

- [ ] **Step 1: Write `types.go`**

```go
// Package roadmap implements the public product roadmap (§113).
//
// Items move through a status lifecycle; community submissions land in
// pending and are triaged by moderators onto the public board. Votes are
// one-per-user; comments ride the §49 comments system via a hidden
// contents row per item.
//
// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md
package roadmap

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// ItemStatus is the lifecycle state of a roadmap item.
type ItemStatus string

const (
	StatusPending     ItemStatus = "pending"
	StatusUnderReview ItemStatus = "under_review"
	StatusPlanned     ItemStatus = "planned"
	StatusInProgress  ItemStatus = "in_progress"
	StatusShipped     ItemStatus = "shipped"
	StatusDeclined    ItemStatus = "declined"
	StatusArchived    ItemStatus = "archived"
)

// PublicStatuses are the statuses rendered on the public board.
var PublicStatuses = []string{
	string(StatusUnderReview), string(StatusPlanned),
	string(StatusInProgress), string(StatusShipped),
}

var validTransitions = map[ItemStatus][]ItemStatus{
	StatusPending:     {StatusUnderReview, StatusDeclined},
	StatusUnderReview: {StatusPlanned, StatusDeclined, StatusArchived},
	StatusPlanned:     {StatusInProgress, StatusUnderReview, StatusDeclined, StatusArchived},
	StatusInProgress:  {StatusShipped, StatusPlanned, StatusArchived},
	StatusShipped:     {StatusArchived},
	StatusDeclined:    {StatusUnderReview, StatusArchived},
	StatusArchived:    {},
}

// IsValidTransition reports whether a status transition is allowed.
func IsValidTransition(from, to ItemStatus) bool {
	if from == to {
		return true
	}
	for _, s := range validTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

var (
	ErrAlreadyVoted = errors.New("already_voted")
	ErrNotVoted     = errors.New("not_voted")
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// BoardItem is one card on the public board.
type BoardItem struct {
	ID             pgtype.UUID `json:"id"`
	Slug           string      `json:"slug"`
	Title          string      `json:"title"`
	Summary        *string     `json:"summary"`
	Status         string      `json:"status"`
	CategoryName   *string     `json:"category_name"`
	CategorySlug   *string     `json:"category_slug"`
	CategoryColor  *string     `json:"category_color"`
	VoteCount      int32       `json:"vote_count"`
	HasVoted       bool        `json:"has_voted"`
	IsFlagship     bool        `json:"is_flagship"`
	TargetWindow   *string     `json:"target_window"`
	FeatureFlagKey *string     `json:"feature_flag_key"`
	ContentID      pgtype.UUID `json:"content_id"`
	ShippedAt      *time.Time  `json:"shipped_at"`
	CreatedAt      time.Time   `json:"created_at"`
}

// ItemDetail adds the description body for the detail page.
type ItemDetail struct {
	BoardItem
	Body *string `json:"body"`
}

// BoardColumn groups items under one status on the public board.
type BoardColumn struct {
	Status string      `json:"status"`
	Label  string      `json:"label"`
	Items  []BoardItem `json:"items"`
}

// CategoryDTO is the public category shape.
type CategoryDTO struct {
	ID          pgtype.UUID `json:"id"`
	Slug        string      `json:"slug"`
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	Color       *string     `json:"color"`
	SortOrder   int32       `json:"sort_order"`
}

// Board is the GET /roadmap/board payload.
type Board struct {
	Columns    []BoardColumn `json:"columns"`
	Categories []CategoryDTO `json:"categories"`
}

// AdminListResponse is the paginated admin list payload.
type AdminListResponse struct {
	Items      []gen.ListAdminRoadmapItemsRow `json:"items"`
	Total      int64                          `json:"total"`
	Page       int32                          `json:"page"`
	TotalPages int32                          `json:"total_pages"`
}
```

- [ ] **Step 2: Write `store.go`** — mirror `internal/tickets/store.go`: `Store{read, write *gen.Queries}`, `NewStore(read, write)`, and one thin wrapped method per gen query (items CRUD, status, reorder, categories CRUD, votes, shadow content). Example pair:

```go
func (s *Store) createItem(ctx context.Context, p gen.CreateRoadmapItemParams) (gen.RoadmapItem, error) {
	row, err := s.write.CreateRoadmapItem(ctx, p)
	if err != nil {
		return gen.RoadmapItem{}, fmt.Errorf("creating roadmap item: %w", err)
	}
	return row, nil
}

func (s *Store) listPublicItems(ctx context.Context, viewerID pgtype.UUID, categorySlug *string, shippedWindowDays int32) ([]gen.ListPublicRoadmapItemsRow, error) {
	rows, err := s.read.ListPublicRoadmapItems(ctx, gen.ListPublicRoadmapItemsParams{
		ViewerID:          viewerID,
		CategorySlug:      categorySlug,
		Statuses:          PublicStatuses,
		ShippedWindowDays: shippedWindowDays,
	})
	if err != nil {
		return nil, fmt.Errorf("listing public roadmap items: %w", err)
	}
	return rows, nil
}
```

- [ ] **Step 3: Write `types_test.go`** — table test for `IsValidTransition` (pending→under_review true, pending→shipped false, shipped→archived true, archived→anything false).

- [ ] **Step 4: Run**

```bash
go test ./internal/roadmap/ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/roadmap/
git commit -m "roadmap: types and store (§113)"
```

---

### Task 5: `internal/roadmap` — service

**Files:**
- Create: `internal/roadmap/service.go`
- Create: `internal/roadmap/service_test.go`

**Interfaces:**
- Consumes: `Store` (Task 4), `gen` queries, `config.Settings`, `valkeycompat.Cmdable`, `worker.Enqueuer`-shaped interface.
- Produces consumed by Task 6 handlers and Task 7 wiring:

```go
type Enqueuer interface { Enqueue(taskType string, payload []byte, opts ...interface{}) error }
func NewService(database *db.DB) *Service
func NewServiceWithEnqueuer(database *db.DB, enq Enqueuer) *Service
func (s *Service) SetSettings(settings *config.Settings)
func (s *Service) SetValkey(vk valkeycompat.Cmdable)

func (s *Service) GetBoard(ctx context.Context, viewerID pgtype.UUID, categorySlug string) (*Board, error)
func (s *Service) GetItem(ctx context.Context, slug string, viewerID pgtype.UUID) (*ItemDetail, error)
func (s *Service) GetChangelog(ctx context.Context) ([]gen.ListAdminRoadmapItemsRow, error) // shipped, all time
func (s *Service) CastVote(ctx context.Context, itemID, userID pgtype.UUID) (int32, error)
func (s *Service) RemoveVote(ctx context.Context, itemID, userID pgtype.UUID) (int32, error)
func (s *Service) CreateSubmission(ctx context.Context, in SubmissionInput) (gen.RoadmapItem, error)
func (s *Service) AdminListItems(ctx context.Context, f AdminListFilter) (AdminListResponse, error)
func (s *Service) AdminCreateItem(ctx context.Context, in AdminItemInput) (gen.RoadmapItem, error)
func (s *Service) AdminUpdateItem(ctx context.Context, id pgtype.UUID, in AdminItemInput) (gen.RoadmapItem, error)
func (s *Service) AdminSetStatus(ctx context.Context, id pgtype.UUID, to ItemStatus, actorID pgtype.UUID) (gen.RoadmapItem, error)
func (s *Service) AdminDeleteItem(ctx context.Context, id pgtype.UUID, force bool) error
func (s *Service) AdminReorderItems(ctx context.Context, ids []pgtype.UUID, orders []int32) error
func (s *Service) ListCategories(ctx context.Context) ([]gen.RoadmapCategory, error)
func (s *Service) AdminListCategories(ctx context.Context) ([]gen.RoadmapCategory, error)
func (s *Service) CreateCategory(ctx context.Context, in CategoryInput) (gen.RoadmapCategory, error)
func (s *Service) UpdateCategory(ctx context.Context, id pgtype.UUID, in CategoryInput) (gen.RoadmapCategory, error)
func (s *Service) DeleteCategory(ctx context.Context, id pgtype.UUID) error
```

- [ ] **Step 1: Write the failing tests** in `service_test.go` — follow `tickets/service_test.go` conventions (they use a real test DB if available via `internal/testdb`; otherwise test pure helpers). Cover: `IsValidTransition` usage inside `AdminSetStatus` returns `errpkg.ErrInvalidStatusTransition` for `pending→shipped`; slug generation dedupes `title` → `title-2`; `CastVote` returns `ErrAlreadyVoted` on second call (if testdb available).

- [ ] **Step 2: Implement `service.go`**

Key logic blocks:

```go
// Service provides roadmap operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
	settings *config.Settings
	vk       valkeycompat.Cmdable
}

const boardCacheKey = "jc:roadmap:board"
```

- `GetBoard`: read `jc:roadmap:board` JSON from Valkey (skip when `categorySlug != ""` — filtered views aren't cached); on miss call `store.listPublicItems` with `PublicStatuses` and `roadmap.shipped_window_days` (default 90) via `s.settings.GetIntOr(ctx, ...)`, group into `BoardColumn`s (labels: Under Consideration / Planned / In Progress / Recently Shipped), fetch categories, marshal + `SET` with TTL `roadmap.board_cache_ttl_seconds` (default 60). Map rows to `BoardItem` (convert `pgtype.Timestamptz` → `*time.Time` where `Valid`).
- `GetItem`: `getItemBySlug`; reject `status='pending'` with `errpkg.ErrNotFound` unless caller has moderate (pass a `canModerate bool` param or let handler check status==pending → 404 — simplest: service returns pending only to moderators via `GetItemAdmin` distinction; public `GetItem` 404s on pending/declined/archived). Include `has_voted` via a `HasRoadmapVote`-style check — add query `HasRoadmapVote :one SELECT EXISTS(...)`. (Add to roadmap.sql in Task 3.)
- `CastVote`: ensure item exists & public status → `tryCreateVote`; on `pgx.ErrNoRows` (conflict do-nothing) return `ErrAlreadyVoted`; `syncVoteCount`; `invalidateBoard`; return new count.
- `RemoveVote`: `deleteVote` execrows → 0 rows ⇒ `ErrNotVoted`; `syncVoteCount`; invalidate.
- `CreateSubmission`: gate on `roadmap.submissions_enabled` (default true → `errpkg.ErrFeatureDisabled` when off); title required (`errpkg.ErrRequired`); slug = `content.Slugify(title)` deduped with `-2/-3` suffix loop; `status='pending'`, `submitted_by=user`; **no shadow content yet** (created on approval).
- `AdminCreateItem`: validate, `status` defaults `under_review`; create shadow content via `createShadowContent` then `setItemContent`.
- `AdminUpdateItem`: update fields; if title/summary changed, also update the shadow contents row title/body via a small `UPDATE contents` query (add `UpdateRoadmapShadowContent :exec UPDATE contents SET title=@title, body=@body, updated_at=now() WHERE id=@content_id` to roadmap.sql).
- `AdminSetStatus`: fetch current → `IsValidTransition` else `errpkg.ErrInvalidStatusTransition`; on `pending→under_review` create shadow content + set `approved_by`; on `→shipped` enqueue `roadmap:ship_notify` payload `{"item_id","title","slug"}` fire-and-forget (Rule 8).
- `AdminDeleteItem`: `soft` default; `force` requires already-deleted (else `errpkg.ErrNotDeleted`) → `HardDeleteRoadmapItem` (route gated by `rbac.RequireForceDelete`, not checked here).
- `AdminReorderItems`: pass `ids`+`orders` arrays to `reorderItems`.
- `createShadowContent(ctx, item, authorID)`: slug `roadmap-<item.slug>`, `content_hash` = sha256 hex of normalized body per Rule 12 (reuse the normalizer if exported — else replicate: lowercase, collapse whitespace, trim); `CreateRoadmapShadowContent`; on unique-violation retry with `-2` suffix.
- `invalidateBoard(ctx)`: `s.vk.Del(ctx, boardCacheKey)` guarded by `if s.vk != nil`.

- [ ] **Step 3: Run tests + build**

```bash
go test ./internal/roadmap/ && go build ./internal/roadmap/
```

- [ ] **Step 4: Commit**

```bash
git commit -am "roadmap: service — board cache, votes, lifecycle, shadow content (§113)"
```

---

### Task 6: `internal/roadmap` — handlers

**Files:**
- Create: `internal/roadmap/handlers.go`

**Interfaces:**
- Consumes: `Service` (Task 5), `auth.FromContext`, `errpkg`, `chi.URLParam`.
- Produces consumed by Task 7 wiring:

```go
func NewHandlers(service *Service) *Handlers
// public
GetBoard, GetChangelog, GetItem, CastVote, RemoveVote, CreateSubmission
// admin
AdminListItems, AdminGetItem, AdminCreateItem, AdminUpdateItem,
AdminSetStatus, AdminDeleteItem, AdminReorderItems,
AdminListCategories, AdminCreateCategory, AdminUpdateCategory, AdminDeleteCategory
```

- [ ] **Step 1: Write `handlers.go`** following `internal/tickets/handlers.go` structure — same helpers (`parseUUIDParam`, `requireUser`, `handleServiceError` → delegate to `errpkg.HandleServiceError` after checking `ErrAlreadyVoted`→409 `already_voted` / `ErrNotVoted`→409 `not_voted`), same response style:

```go
errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": board})
```

Public handlers:
- `GetBoard` — `viewerID` from `auth.FromContext` (nil for guests — OptionalAuth route); `category` query param; `errpkg.WriteJSON(w, 200, {"data": board})`.
- `GetItem` — slug param; service `ErrNotFound` → 404 via HandleServiceError.
- `GetChangelog` — items where `status='shipped'` ordered `shipped_at DESC` (add `ListChangelogItems` query or reuse `ListRoadmapItemsByStatus` filtered + grouped client-side — prefer a dedicated `ListRoadmapChangelog :many SELECT i.*, c.name AS category_name FROM roadmap_items i LEFT JOIN roadmap_categories c ON c.id=i.category_id WHERE i.status='shipped' AND i.deleted_at IS NULL ORDER BY i.shipped_at DESC` in roadmap.sql).
- `CastVote`/`RemoveVote` — require user; `{id}` param; return `{"data":{"vote_count": n}}`; map `ErrAlreadyVoted` → `errpkg.WriteError(w, 409, "already_voted", ...)`.
- `CreateSubmission` — require user; JSON body `{title, summary, body, category_id}`; 400 on missing title; 201 with item.

Admin handlers — JSON bodies mirror `AdminItemInput` `{title, slug?, summary, body, category_id?, status?, sort_order, is_flagship, target_window?, feature_flag_key?}`; `AdminSetStatus` body `{status}`; `AdminReorderItems` body `{items: [{id, sort_order}]}`; `AdminListItems` reads `page_size`, `page_offset`, `status`, `category_id`, `q` (search), `sort` (`recent|votes|title`), `include_deleted`.

- [ ] **Step 2: Build + commit**

```bash
go build ./internal/roadmap/
git commit -am "roadmap: HTTP handlers (§113)"
```

---

### Task 7: Feature flag + route wiring

**Files:**
- Modify: `internal/features/registry.go`
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: `roadmap.NewService`, `roadmap.NewHandlers` (Tasks 5–6), existing `featureChecker`, `kr`, `vkClient`, `rbacResolver`, `settingsService`, and the Asynq enqueuer used by tickets (`worker.NewEnqueuer(...)` — grep `tickets.NewService`/`NewServiceWithEnqueuer` in server.go to copy the exact wiring).

- [ ] **Step 1: Register the flag**

In `internal/features/registry.go` add near the community features:

```go
{Key: "roadmap.enabled", Label: "Public Product Roadmap", Default: true},
```

- [ ] **Step 2: Wire the service** — next to `ticketHandlers` construction (~line 4540):

```go
// §113 — Public Product Roadmap.
roadmapService := roadmap.NewServiceWithEnqueuer(database, enq) // match tickets' enqueuer wiring
roadmapService.SetSettings(settingsService)
roadmapService.SetValkey(vkClient.Redis())
roadmapHandlers := roadmap.NewHandlers(roadmapService)
```

- [ ] **Step 3: Register routes** — inside the same API group as tickets:

```go
// Public Product Roadmap (§113). Flag: roadmap.enabled → 404 when off.
r.With(features.Gate(featureChecker, "roadmap.enabled")).Group(func(r chi.Router) {
    r.With(auth.OptionalAuth(kr, vkClient.Redis())).
        Get("/roadmap/board", roadmapHandlers.GetBoard)
    r.With(auth.OptionalAuth(kr, vkClient.Redis())).
        Get("/roadmap/changelog", roadmapHandlers.GetChangelog)
    r.With(auth.OptionalAuth(kr, vkClient.Redis())).
        Get("/roadmap/items/{slug}", roadmapHandlers.GetItem)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.vote"), mw.RateLimit(vkClient.Redis(), "moderate")).
        Post("/roadmap/items/{id}/vote", roadmapHandlers.CastVote)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.vote"), mw.RateLimit(vkClient.Redis(), "moderate")).
        Delete("/roadmap/items/{id}/vote", roadmapHandlers.RemoveVote)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.submit"), mw.RateLimit(vkClient.Redis(), "form_submission")).
        Post("/roadmap/submissions", roadmapHandlers.CreateSubmission)

    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.view")).
        Get("/admin/roadmap/items", roadmapHandlers.AdminListItems)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.create")).
        Post("/admin/roadmap/items", roadmapHandlers.AdminCreateItem)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.view")).
        Get("/admin/roadmap/items/{id}", roadmapHandlers.AdminGetItem)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.edit")).
        Patch("/admin/roadmap/items/{id}", roadmapHandlers.AdminUpdateItem)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.delete")).
        Delete("/admin/roadmap/items/{id}", roadmapHandlers.AdminDeleteItem)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.delete"), rbac.RequireForceDelete(rbacResolver)).
        Delete("/admin/roadmap/items/{id}/permanent", roadmapHandlers.AdminHardDeleteItem)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.moderate")).
        Patch("/admin/roadmap/items/{id}/status", roadmapHandlers.AdminSetStatus)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.edit")).
        Post("/admin/roadmap/items/reorder", roadmapHandlers.AdminReorderItems)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.moderate")).
        Get("/admin/roadmap/submissions", roadmapHandlers.AdminListSubmissions)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.view")).
        Get("/admin/roadmap/categories", roadmapHandlers.AdminListCategories)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.edit")).
        Post("/admin/roadmap/categories", roadmapHandlers.AdminCreateCategory)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.edit")).
        Patch("/admin/roadmap/categories/{id}", roadmapHandlers.AdminUpdateCategory)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "roadmap.edit")).
        Delete("/admin/roadmap/categories/{id}", roadmapHandlers.AdminDeleteCategory)
})
```

Notes for the implementer: `AdminListSubmissions` = `AdminListItems` hardcoded to `status=pending`; `AdminHardDeleteItem` calls `AdminDeleteItem(id, force=true)` (verify `rbac.RequireForceDelete` exists — used at server.go:4201; if it requires `?force=true` in the URL instead, keep `?force` on the DELETE route and drop the separate `/permanent` path — match the JCE pattern).

- [ ] **Step 4: Build + curl smoke test**

```bash
go build ./... 
# with the API running:
curl -s localhost:8080/api/v1/roadmap/board | jq
```

Expected: `{"data":{"columns":[...],"categories":[...]}}` (empty items).

- [ ] **Step 5: Commit**

```bash
git commit -am "roadmap: feature flag + route wiring (§113)"
```

---

### Task 8: Worker task `roadmap:ship_notify`

**Files:**
- Modify: `internal/worker/tasks.go` (const + registration)
- Modify: `internal/worker/handlers.go` (handler)
- Modify: `internal/worker/enqueuer.go` (task catalog entry)
- Modify: `internal/db/queries/roadmap.sql` (voter query — added in Task 3: `ListRoadmapVoterIDs`)

**Interfaces:**
- Consumes: `gen.ListRoadmapVoterIDs`, `gen.InsertNotification`, `worker` helpers `requireDB()`/`parsePayloadOrSkip`/`scanUUID`, package var `dbPool`.

- [ ] **Step 1: Register the task type** in `internal/worker/tasks.go` with the other `Type...` consts and handler-registration slice:

```go
TypeRoadmapShipNotify = "roadmap:ship_notify"
```

```go
{TypeRoadmapShipNotify, HandleRoadmapShipNotify},
```

- [ ] **Step 2: Add the catalog entry** in `internal/worker/enqueuer.go`'s task table (near `{TypeTicketSendStatusUpdate, "default", "Send ticket status change email"}`):

```go
{TypeRoadmapShipNotify, "default", "Notify roadmap voters/submitter that an item shipped"},
```

- [ ] **Step 3: Write the handler** in `internal/worker/handlers.go`, modeled on `HandleTicketSendStatusUpdate` + the `InsertNotification` block at ~line 3688:

```go
// HandleRoadmapShipNotify notifies every voter and the submitter when a
// roadmap item transitions to shipped (§113).
func HandleRoadmapShipNotify(ctx context.Context, t *asynq.Task) error {
	slog.Info("task:roadmap:ship_notify")

	queries, err := requireDB()
	if err != nil {
		slog.Warn("task:roadmap:ship_notify: no database, skipping", "error", err)
		return nil
	}

	var payload struct {
		ItemID string `json:"item_id"`
		Title  string `json:"title"`
		Slug   string `json:"slug"`
	}
	if err := parsePayloadOrSkip(t, &payload); err != nil {
		return err
	}
	itemID, err := scanUUID(payload.ItemID)
	if err != nil {
		return fmt.Errorf("task:roadmap:ship_notify: %w", err)
	}

	voters, err := queries.ListRoadmapVoterIDs(ctx, itemID)
	if err != nil {
		slog.Error("task:roadmap:ship_notify: list voters failed", "error", err)
		return err
	}

	item, err := queries.GetRoadmapItemByID(ctx, itemID)
	if err != nil {
		slog.Warn("task:roadmap:ship_notify: item gone", "error", err)
		return nil
	}

	recipients := map[string]pgtype.UUID{}
	for _, v := range voters {
		recipients[v.String()] = v
	}
	if item.SubmittedBy != nil {
		recipients[item.SubmittedBy.String()] = *item.SubmittedBy
	}

	actorType, subjectType := "system", "roadmap_item"
	title := "Shipped: " + payload.Title
	bodyStr := "A roadmap item you voted for has shipped."
	actionURL := "/roadmap/" + payload.Slug

	writer := gen.New(dbPool.Writer)
	for _, rid := range recipients {
		if _, nErr := writer.InsertNotification(ctx, gen.InsertNotificationParams{
			RecipientID:      rid,
			NotificationType: "roadmap_shipped",
			ActorType:        &actorType,
			SubjectType:      &subjectType,
			SubjectID:        itemID,
			Title:            title,
			Body:             &bodyStr,
			ActionUrl:        &actionURL,
		}); nErr != nil {
			slog.Error("task:roadmap:ship_notify: notification failed", "user_id", rid.String(), "error", nErr)
		}
	}
	return nil
}
```

- [ ] **Step 4: Verify §11 catalog entry exists** (Task 1 Step 9) — Rule 7.

- [ ] **Step 5: Build + commit**

```bash
go build ./internal/worker/
git commit -am "worker: roadmap:ship_notify task (§113)"
```

---

### Task 9: `apps/main` — types + board page

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (add `RoadmapCategory`, `RoadmapBoardItem`, `RoadmapBoardColumn`, `RoadmapBoard`, `RoadmapItemDetail`)
- Create: `apps/main/src/lib/components/roadmap/VoteButton.svelte`
- Create: `apps/main/src/lib/components/roadmap/RoadmapCard.svelte`
- Create: `apps/main/src/lib/components/roadmap/StatusBadge.svelte`
- Create: `apps/main/src/routes/(app)/roadmap/+page.server.ts`
- Create: `apps/main/src/routes/(app)/roadmap/+page.svelte`

**Interfaces:**
- Consumes: `apiGet`/`apiContext` (`$lib/server/api`), `api` client (`$lib/api/client`), `t` (`$lib/i18n`), `EmptyState` (`$lib/components/ui`), `SessionUser.permissions`.
- Produces consumed by Tasks 10–11: `VoteButton` props `{item: RoadmapBoardItem, authed: boolean}`, `RoadmapCard` props `{item, authed, canModerate}`.

- [ ] **Step 1: Add types** to `types/index.ts`:

```ts
export interface RoadmapCategory {
	id: string;
	slug: string;
	name: string;
	description: string | null;
	color: string | null;
	sort_order: number;
}

export interface RoadmapBoardItem {
	id: string;
	slug: string;
	title: string;
	summary: string | null;
	status: 'under_review' | 'planned' | 'in_progress' | 'shipped' | 'pending' | 'declined' | 'archived';
	category_name: string | null;
	category_slug: string | null;
	category_color: string | null;
	vote_count: number;
	has_voted: boolean;
	is_flagship: boolean;
	target_window: string | null;
	feature_flag_key: string | null;
	content_id: string | null;
	shipped_at: string | null;
	created_at: string;
}

export interface RoadmapBoardColumn {
	status: string;
	label: string;
	items: RoadmapBoardItem[];
}

export interface RoadmapBoard {
	columns: RoadmapBoardColumn[];
	categories: RoadmapCategory[];
}

export interface RoadmapItemDetail extends RoadmapBoardItem {
	body: string | null;
}
```

- [ ] **Step 2: `VoteButton.svelte`** — pill button showing ▲ count; pressed state when `has_voted`; `api.post/delete('/roadmap/items/'+item.id+'/vote')` toggle; guests → `goto('/auth/login?next=/roadmap')`; optimistic count update rolled back on error.

- [ ] **Step 3: `StatusBadge.svelte` + `RoadmapCard.svelte`** — badge maps status → label/color via i18n keys `roadmap.status.<status>`; card renders title, summary, category chip (colored dot), target window, `is_flagship` pin, vote button, link to `/roadmap/{slug}`; when `canModerate`, a small status `<select>` that PATCHes `/admin/roadmap/items/{id}/status` via `api.patch` then `invalidateAll()`.

- [ ] **Step 4: `+page.server.ts`** —

```ts
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { RoadmapBoard } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const category = event.url.searchParams.get('category') ?? '';
	let board: RoadmapBoard | null = null;
	let loadError = false;
	try {
		board = await apiGet<RoadmapBoard>(ctx, `/roadmap/board${category ? `?category=${category}` : ''}`);
	} catch {
		loadError = true;
	}
	return { board, loadError, category };
};
```

- [ ] **Step 5: `+page.svelte`** — page header (title + subtitle + "Submit a request" button → `/roadmap/submit`), flagship item pinned banner (first `is_flagship` item across columns), category filter chips (`?category=` links), 4-column grid (horizontal scroll on mobile) of `RoadmapCard`s per column, `EmptyState` when `board == null`, i18n keys `roadmap.*`. Moderator affordance: `canModerate = data.user?.permissions?.includes('roadmap.moderate')`.

- [ ] **Step 6: Check + commit**

```bash
cd apps/main && bun run check
git commit -am "main: /roadmap public board page (§113)"
```

---

### Task 10: `apps/main` — item detail page with comments

**Files:**
- Create: `apps/main/src/routes/(app)/roadmap/[slug]/+page.server.ts`
- Create: `apps/main/src/routes/(app)/roadmap/[slug]/+page.svelte`

**Interfaces:**
- Consumes: `GET /roadmap/items/{slug}` → `RoadmapItemDetail`; `GET /content/{id}/comments` → `Comment[]` (envelope); `CommentThread` props `{comments, totalCount, contentId, contentType, authed, currentUserId, canModerate, loadError, deletedUserIds}`; `tiptapBodyToHtml` (`$lib/tiptap/serialize`) for `body` rendering.

- [ ] **Step 1: `+page.server.ts`** — fetch item by slug; on miss `error(404)`. If `item.content_id`, SSR-seed comments exactly like `blog/[slug]/+page.server.ts`:

```ts
const envelope = await apiGetEnvelope<Comment[]>(ctx, `/content/${item.content_id}/comments?page_size=50`);
comments = envelope.data ?? [];
```

Return `{ item, comments, commentsLoadError, deletedUserIds }` — merge deleted-user IDs from comments the same way the blog page does (copy that block verbatim).

- [ ] **Step 2: `+page.svelte`** — layout: status stepper (Under Review → Planned → In Progress → Shipped with current highlighted), `VoteButton`, category chip, `target_window`, feature-flag badge (`feature_flag_key` present → "rolling out" badge), `{@html tiptapBodyToHtml(item.body)}` description, submitter credit + dates, `<CommentThread comments={data.comments} contentId={item.content_id} contentType="roadmap" authed={!!data.user} currentUserId={data.user?.id ?? null} canModerate={canModerate} loadError={data.commentsLoadError} />`. Moderators get the same inline status select as the card.

- [ ] **Step 3: Check + commit**

```bash
cd apps/main && bun run check
git commit -am "main: /roadmap/[slug] detail with §49 comments (§113)"
```

---

### Task 11: `apps/main` — changelog + submit page

**Files:**
- Create: `apps/main/src/routes/(app)/roadmap/changelog/+page.server.ts` + `+page.svelte`
- Create: `apps/main/src/routes/(app)/roadmap/submit/+page.server.ts` + `+page.svelte`

**Interfaces:**
- Consumes: `GET /roadmap/changelog` → shipped items grouped by date; `POST /roadmap/submissions` `{title, summary?, body?, category_id?}`; categories from `GET /roadmap/board` (categories array).

- [ ] **Step 1: Changelog page** — server load fetches `/roadmap/changelog`; page groups items by `shipped_at` month ("October 2026") and renders a release-notes list: date, title link, summary, category chip.

- [ ] **Step 2: Submit page** — `+page.server.ts` redirects unauthenticated users to `/auth/login?next=/roadmap/submit`; loads categories via `/roadmap/board`. `+page.svelte` form: title input, category `AutocompleteSelect` (same component as `/feedback`), summary textarea, body textarea (plain text — comments accept plain body; reuse `CommentEditor` if it accepts a `placeholder` prop, else `<textarea>`), submit → `api.post('/roadmap/submissions', {...})`; on success show "submitted for review" state. Follow the `/feedback` page's structure and error handling.

- [ ] **Step 3: Check + commit**

```bash
cd apps/main && bun run check
git commit -am "main: roadmap changelog + submission pages (§113)"
```

---

### Task 12: `apps/main` — `roadmap.japaconnect.com` host detection

**Files:**
- Modify: `apps/main/src/hooks.server.ts`
- Modify: `apps/main/src/app.d.ts` (`Locals` + `PageData`)
- Modify: `apps/main/src/routes/+layout.server.ts` (pass `isRoadmapHost` into PageData)
- Modify: `apps/main/src/routes/(app)/+layout.svelte` or the app-shell component that branches on `isUniversityHost` (find where it renders — grep showed only layout.server.ts files reference it; the branch likely lives in `+layout.svelte` or `app-shell` components — locate and mirror it)
- Modify: `apps/main/.env.example` (`JC_ROADMAP_HOSTS`)

**Interfaces:**
- Consumes: `isUniversitySurface()` + `event.locals.isUniversityHost` (hooks.server.ts ~lines 58–67, 265).
- Produces: `event.locals.isRoadmapHost`, `PageData.isRoadmapHost`; when true the app shell renders a minimal roadmap-branded header/footer instead of the full community chrome; the subdomain root `/` redirects to `/roadmap`.

- [ ] **Step 1: Host detector** in `hooks.server.ts` beside `isUniversitySurface`:

```ts
/**
 * ADR-127: detect whether the request host is the public roadmap surface.
 * The canonical host is roadmap.japaconnect.com; additional custom domains
 * via the JC_ROADMAP_HOSTS env var (comma-separated).
 */
function isRoadmapSurface(host: string): boolean {
	const canonical = 'roadmap.japaconnect.com';
	const configured = env.JC_ROADMAP_HOSTS?.split(',').map((h) => h.trim().toLowerCase()).filter(Boolean) ?? [];
	return [canonical, ...configured].includes(host.toLowerCase());
}
```

Set `event.locals.isRoadmapHost = isRoadmapSurface(event.url.hostname)` beside the `isUniversityHost` assignment.

- [ ] **Step 2: Types** — add `isRoadmapHost: boolean` to `Locals` and `PageData` in `app.d.ts`.

- [ ] **Step 3: PageData** — in `apps/main/src/routes/+layout.server.ts`, include `isRoadmapHost: event.locals.isRoadmapHost` in the returned data (mirroring `isUniversityHost`).

- [ ] **Step 4: Shell branch** — where `isUniversityHost` switches shells (locate the component), add the roadmap branch: when `data.isRoadmapHost`, render a minimal shell — logo + "Roadmap" + "Back to JapaConnect" link — wrapping `<slot />`. Also redirect `/` → `/roadmap` on the roadmap host: in `apps/main/src/routes/(app)/+page.server.ts` (or wherever the root load lives) add `if (event.locals.isRoadmapHost) redirect(302, '/roadmap')` — alternatively a dedicated `+page.server.ts` guard; match how the university host handles `/`.

- [ ] **Step 5: `.env.example`** — add `JC_ROADMAP_HOSTS=`.

- [ ] **Step 6: Check + commit**

```bash
cd apps/main && bun run check
git commit -am "main: roadmap.japaconnect.com host detection (ADR-127)"
```

---

### Task 13: `apps/admin` — API module + Roadmap section

**Files:**
- Modify: `apps/admin/src/lib/api.ts` (append `roadmapAdminApi` block)
- Create: `apps/admin/src/routes/roadmap/+page.svelte` (items EntityPage)
- Create: `apps/admin/src/routes/roadmap/submissions/+page.svelte` (triage)
- Create: `apps/admin/src/routes/roadmap/categories/+page.svelte` (categories EntityPage)
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (nav entries)
- Modify: admin i18n locale files for `nav.roadmap*` + `roadmap.*` keys

**Interfaces:**
- Consumes: `api<T>(path, opts)` → `ApiResult<T>`; `EntityPage`/`EntityPageConfig`/`FieldDef` (`$lib/components/entity`); column/filter/action types; `t` (`$lib/stores/i18n.svelte`).
- Produces: admin routes `/admin/roadmap`, `/admin/roadmap/submissions`, `/admin/roadmap/categories` gated by `roadmap.*` nodes.

- [ ] **Step 1: API module** — append to `apps/admin/src/lib/api.ts`:

```ts
// ── Public Product Roadmap (§113) ───────────────────────────────────────

export const roadmapAdminApi = {
	list: (params?: string) => api<unknown>(`/admin/roadmap/items${params ? `?${params}` : ''}`),
	get: (id: string) => api<unknown>(`/admin/roadmap/items/${id}`),
	create: (body: Record<string, unknown>) =>
		api('/admin/roadmap/items', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api(`/admin/roadmap/items/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/roadmap/items/${id}`, { method: 'DELETE' }),
	hardDelete: (id: string) =>
		api(`/admin/roadmap/items/${id}?force=true`, { method: 'DELETE' }),
	setStatus: (id: string, status: string) =>
		api(`/admin/roadmap/items/${id}/status`, { method: 'PATCH', body: { status } }),
	reorder: (items: { id: string; sort_order: number }[]) =>
		api('/admin/roadmap/items/reorder', { method: 'POST', body: { items } }),
	submissions: (params?: string) =>
		api<unknown>(`/admin/roadmap/submissions${params ? `?${params}` : ''}`),
};

export const roadmapCategoriesAdminApi = {
	list: (params?: string) =>
		api<unknown>(`/admin/roadmap/categories${params ? `?${params}` : ''}`),
	create: (body: Record<string, unknown>) =>
		api('/admin/roadmap/categories', { method: 'POST', body }),
	update: (id: string, body: Record<string, unknown>) =>
		api(`/admin/roadmap/categories/${id}`, { method: 'PATCH', body }),
	delete: (id: string) =>
		api(`/admin/roadmap/categories/${id}`, { method: 'DELETE' }),
};
```

- [ ] **Step 2: Items page** `routes/roadmap/+page.svelte` — `EntityPage` scaffold per `share-platforms` template. Columns: title (link-styled), `status` (badge render via column `render`), `category_name`, `vote_count` (sortable), `target_window`, `is_flagship`, `created_at`. Filters: status `select` (all 7 statuses), category `select` (options loaded from `roadmapCategoriesAdminApi.list` on mount — follow `/users` dynamic-filter pattern). `fields` (EntityForm): title (text, required), slug (text, optional placeholder "auto"), summary (textarea), body (textarea), category_id (select, options from categories API), status (select), sort_order (number), is_flagship (checkbox), target_window (text), feature_flag_key (text). Permissions: `createPermission: 'roadmap.create'`, `editPermission: 'roadmap.edit'`, `deletePermission: 'roadmap.delete'`, `hardDeletePermission: 'admin.hard_delete'`; provide `remove`, `hardRemove` (needs `isDeleted: (row) => row.deleted_at != null` — include `include_deleted=true` in load params). Custom `actions`: default set + "Change Status" row action opening a page-local `<Modal>` with a status select → `roadmapAdminApi.setStatus` → `entityController?.reload()`. `exportable: true`.

- [ ] **Step 3: Submissions page** `routes/roadmap/submissions/+page.svelte` — `EntityPage` readonly-ish list over `roadmapAdminApi.submissions` (status=pending rows). Custom `actions`: **Approve** (`show: row => row.status === 'pending'` → `setStatus(id,'under_review')` → reload) and **Decline** (`variant:'destructive'` → confirm `Modal` → `setStatus(id,'declined')` → reload). Include View action (opens `viewSnippet` showing title/summary/body/submitter). `hideCreate: true`, no delete.

- [ ] **Step 4: Categories page** `routes/roadmap/categories/+page.svelte` — simple `EntityPage` like share-platforms: columns slug/name/color (componentSnippet swatch)/sort_order; fields slug/name/description/color (`type:'color'`)/sort_order; permissions `roadmap.edit`; bulk delete on.

- [ ] **Step 5: Sidebar nav** — in `sidebar.svelte`, add under the Content section (after share-platforms):

```ts
{ path: "/roadmap", label: t("nav.roadmap", "Roadmap"), icon: MapIcon, permission: "roadmap.view" },
{ path: "/roadmap/submissions", label: t("nav.roadmap_submissions", "Roadmap Submissions"), icon: Inbox, permission: "roadmap.moderate" },
{ path: "/roadmap/categories", label: t("nav.roadmap_categories", "Roadmap Categories"), icon: Tags, permission: "roadmap.edit" },
```

Import `Map as MapIcon, Inbox, Tags` from lucide-svelte (check existing imports).

- [ ] **Step 6: Check + commit**

```bash
cd apps/admin && bun run check
git commit -am "admin: roadmap management section (§113)"
```

---

### Task 14: Sitemap + i18n + SEO

**Files:**
- Modify: `internal/seo/sitemap.go` (add `ServeRoadmapSitemap` following `ServeDealsSitemap` ~line 470, + index entry `{Loc: base + "/sitemap-roadmap.xml", ...}` in the sitemap index ~line 217)
- Modify: `internal/server/server.go` (`GET /sitemap-roadmap.xml` root route beside the other sitemap routes)
- Modify: `apps/main/src/lib/i18n/en.json` + `fr.json` (all `roadmap.*` keys used in Tasks 9–11)
- Modify: `apps/main/src/routes/robots.txt/+server.ts` only if the roadmap host needs separate rules (check current impl — likely host-agnostic already)

**Interfaces:**
- Consumes: `gen.ListRoadmapSitemapItems` (Task 3).

- [ ] **Step 1: `ServeRoadmapSitemap`** — mirror `ServeDealsSitemap`: read `jc:cache:sitemap:roadmap` cache, query `ListRoadmapSitemapItems`, emit `/roadmap/{slug}` urlset with `<lastmod>` from `updated_at`, plus static `/roadmap` and `/roadmap/changelog` entries; add to the sitemap index; register the route.

- [ ] **Step 2: i18n keys** — in `en.json` under a `"roadmap"` object: `title`, `subtitle`, `board.*` (column labels: `under_review`→"Under Consideration", `planned`→"Planned", `in_progress`→"In Progress", `shipped`→"Recently Shipped"), `submit*`, `vote*` (`vote`, `voted`, `votes`), `status.*`, `changelog.*`, `detail.*`, `errors.*`, `submissions.*`. French translations in `fr.json`.

- [ ] **Step 3: Build + check + commit**

```bash
go build ./... && cd apps/main && bun run check
git commit -am "seo+i18n: roadmap sitemap and translations (§113)"
```

---

### Task 15: Seed flagship content + end-to-end verification

**Files:**
- Optionally create: `migrations/000795_roadmap.up.sql` amendment is NOT needed — seed the flagship item via the admin UI post-deploy, or include a seed row in the migration for the Spaces item (decision: include it — the flagship is a product decision made in this spec):

In the up migration (amend Task 2's file before it ships — fine since it hasn't run anywhere yet), append:

```sql
-- Flagship: the first wave — Spaces & Content Creation (in_progress).
INSERT INTO roadmap_items (title, slug, summary, body, category_id, status, sort_order, is_flagship, target_window, feature_flag_key)
SELECT 'Spaces & Content Creation', 'spaces-content-creation',
       'The core of JapaConnect: geo-first spaces and rich content creation — posts, articles, media, and moderation tools.',
       'The core of JapaConnect: geo-first spaces (Country → State → City → University) and rich content creation — posts, articles, media, drafts, and moderation tools. This is the first feature wave going live.',
       c.id, 'in_progress', 0, true, 'Q4 2026', NULL
FROM roadmap_categories c WHERE c.slug = 'spaces-content';
```

- [ ] **Step 1: Amend migration + re-run on a fresh DB** (down + up) to confirm idempotency.

- [ ] **Step 2: Backend regression checks**

```bash
go build ./...
go test ./internal/roadmap/ ./internal/worker/ -count=1
# gosec on new code (Rule 15 context): gosec ./internal/roadmap/
```

- [ ] **Step 3: Frontend checks**

```bash
cd apps/main && bun run check && cd ../admin && bun run check
```

- [ ] **Step 4: Manual smoke script** — with the stack running:

```bash
curl -s localhost:8080/api/v1/roadmap/board | jq '.data.columns[].status'   # all 4 public statuses
curl -s localhost:8080/api/v1/roadmap/items/spaces-content-creation | jq '.data.title'
curl -s -X POST localhost:8080/api/v1/roadmap/items/{id}/vote -H "Authorization: Bearer $TOK"  # 200 {vote_count:1}
curl -s -X POST localhost:8080/api/v1/roadmap/items/{id}/vote -H "Authorization: Bearer $TOK"  # 409 already_voted
curl -s -X POST localhost:8080/api/v1/roadmap/items/{id}/vote -X DELETE -H "Authorization: Bearer $TOK"  # vote_count:0
curl -s -X POST localhost:8080/api/v1/roadmap/submissions -H "Authorization: Bearer $TOK" -d '{"title":"Test request"}'  # 201 pending
# admin: PATCH status pending→under_review → shadow content created; →shipped → roadmap:ship_notify enqueued
```

- [ ] **Step 5: Feed/search leak check** — verify `content_type='roadmap'` shadow rows do not appear in `/api/v1/feed`, search, or sitemap-content (those queries filter by type — confirm with a quick query/`grep 'content_type' internal/db/queries/feeds.sql`).

- [ ] **Step 6: Update `.jeos/CHANGELOG.md`** — add the §113 implementation entry matching the file's format.

- [ ] **Step 7: Final commit**

```bash
git commit -am "roadmap: flagship seed + docs (§113 complete)"
```

---

## Self-review notes

- **Spec coverage:** board/detail/changelog/submit pages, votes, comments via shadow contents, admin CRUD + triage + reorder + categories, inline edit, host detection, feature flag, notifications, sitemap, i18n, GDPR cascade — all mapped to tasks.
- **Query inventory:** Task 3's `roadmap.sql` must also include `HasRoadmapVote`, `UpdateRoadmapShadowContent`, and `ListRoadmapChangelog` (referenced by Tasks 5–6 — add them in Step 1 alongside the listed queries).
- **Known follow-ups (not in scope):** anonymous voting, per-tenant roadmaps, email digests of roadmap changes, auto-sync of `feature_flag_key` badge with real flag state.
