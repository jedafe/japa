# Public Product Roadmap — Design Spec

**Date:** 2026-10-05
**Status:** Approved design, pre-implementation
**JEOS:** new section `sections/113-public-product-roadmap.md` + `adr/ADR-127-public-product-roadmap.md` (to be authored as part of implementation, before code per AGENTS.md)
**Migration:** `000796_roadmap.{up,down}.sql`

## 1. Purpose

JapaConnect is approaching production and will release features incrementally, with
**Spaces & Content Creation** leading the priority table. This spec defines a public,
feature-rich product roadmap surface — `japa.to/roadmap` AND
`roadmap.japa.to` — that:

- communicates what is under consideration, planned, in progress, and shipped;
- lets logged-in users vote on items and submit feature requests;
- carries public discussion on each item via the existing §49 comments system;
- gives moderators inline editing on the public surface plus a full DataTable
  management section in `apps/admin`;
- notifies voters and submitters when an item ships.

## 2. Approved decisions

| Decision | Outcome |
| :--- | :--- |
| Interaction model | Voting + public submissions + comments (full Canny-style) |
| Hosting | `/roadmap` in `apps/main` **and** `roadmap.japa.to` via host detection (ADR-063 precedent, `isUniversityHost` pattern) |
| Management | Admin panel DataTable **and** privileged inline editing on the public page |
| Voting auth | Logged-in users only; one user, one vote per item |
| Backend shape | Dedicated `internal/roadmap` module (Handlers → Service → Store) |
| Comments | Shadow `contents` row per item (`content_type='roadmap'`, `seo_indexable=false`), reusing §49 comments with threading, mentions, moderation |

## 3. Architecture & data flow

New layered module `internal/roadmap/` registered in `internal/server/server.go`.
Public board reads are cached in Valkey (`jc:roadmap:board`, ~60s TTL) and
invalidated on any mutation. A `roadmap.enabled` feature flag gates every route —
routes return 404 when disabled, matching the `internal/features` contract.

```
Browser ──► apps/main /roadmap (SSR) ──► GET /api/v1/roadmap/board ──► roadmap.Service ──► Postgres (+Valkey)
        └── roadmap.japa.to ──► hooks.server.ts host detect ──► same pages, minimal shell
Admin   ──► apps/admin DataTable ──► /api/v1/admin/roadmap/* (RBAC-gated)
Worker  ──► asynq task roadmap:ship_notify ──► notifications to voters + submitter
```

For each published item the service maintains a hidden `contents` row:
`content_type='roadmap'`, `seo_indexable=false`, `status='published'`,
`visibility='public'`. Its `id` is stored on `roadmap_items.content_id`, so the
existing `/api/v1/comments` endpoints serve the discussion thread with no new
comment code. Feed/search/sitemap queries already filter by `content_type`, so
`roadmap` rows cannot leak into them. `content_hash` is computed on the shadow
row per Rule 12.

## 4. Data model — migration `000796_roadmap`

### `roadmap_items`

| Column | Type | Notes |
| :--- | :--- | :--- |
| `id` | uuid PK | |
| `title` | text | required |
| `slug` | text | unique, indexed |
| `summary` | text | card preview |
| `body` | text | markdown source; frontend renders it (same pipeline as comments) |
| `category_id` | uuid FK → roadmap_categories | nullable |
| `status` | text | see lifecycle below |
| `sort_order` | int | manual ordering within a status column |
| `is_flagship` | bool | pinned "now building" highlight |
| `target_window` | text | free label, e.g. `Q4 2026` |
| `feature_flag_key` | text | nullable; links to a flags-registry key (e.g. `spaces.enabled`) shown as a rollout badge |
| `content_id` | uuid FK → contents | shadow comments row |
| `vote_count` | int | denormalized; self-healing recount per mutation (ADR-127) |
| `submitted_by` | uuid FK → users | nullable (admin-created items) |
| `approved_by` | uuid FK → users | nullable |
| `shipped_at` | timestamptz | set on `→ shipped` |
| `created_at` / `updated_at` / `deleted_at` | timestamptz | soft delete per Rule 11 |

### `roadmap_categories`

`id, slug (unique), name, description, color, sort_order, created_at, updated_at, deleted_at`

Seeded defaults: `spaces-content` (Spaces & Content Creation), `community`,
`tools`, `mobile`, `platform`.

### `roadmap_votes`

`item_id FK, user_id FK, created_at` — `PRIMARY KEY (item_id, user_id)`,
`ON DELETE CASCADE` on `user_id` so GDPR account erasure removes votes.

### Status lifecycle

```
pending (community submission, NOT public)
   │ approve (admin triage)
   ▼
under_review (public, votable) ──► planned ──► in_progress ──► shipped (sets shipped_at)
   │                                                    │
   └──► declined (hidden from default board)      archived
```

Public board shows: `under_review`, `planned`, `in_progress`, `shipped` (recent
window). `pending`, `declined`, `archived` never render publicly.

## 5. API surface

### Public (`/api/v1/roadmap/*`)

| Method | Route | Auth | Notes |
| :--- | :--- | :--- | :--- |
| GET | `/roadmap/board` | — | items grouped by status + categories + counts; Valkey-cached |
| GET | `/roadmap/items/{slug}` | — | item detail incl. `content_id` for comments |
| GET | `/roadmap/changelog` | — | shipped items grouped by ship date |
| POST | `/roadmap/items/{id}/vote` | required | toggle-on; 409 `already_voted` on repeat |
| DELETE | `/roadmap/items/{id}/vote` | required | removes vote |
| POST | `/roadmap/submissions` | required | `form_submission` rate tier; creates `pending` item |
| GET/POST/etc | `/comments?content_id=…` | existing | unchanged §49 endpoints serve item threads |

### Admin (`/api/v1/admin/roadmap/*`, RBAC-gated)

| Method | Route | Node |
| :--- | :--- | :--- |
| GET | `/admin/roadmap/items` (DataTable: sort/search/filter/paginate) | `roadmap.view` |
| POST | `/admin/roadmap/items` | `roadmap.create` |
| GET/PATCH | `/admin/roadmap/items/{id}` | `roadmap.view` / `roadmap.edit` |
| DELETE | `/admin/roadmap/items/{id}` | `roadmap.delete` (soft) |
| PATCH | `/admin/roadmap/items/{id}/status` | `roadmap.moderate` — drives lifecycle, fires ship_notify |
| POST | `/admin/roadmap/items/reorder` | `roadmap.edit` |
| GET | `/admin/roadmap/submissions` (`status=pending` queue) | `roadmap.moderate` |
| CRUD | `/admin/roadmap/categories[/{id}]` | `roadmap.edit` |

RBAC nodes: `roadmap.vote`, `roadmap.submit` (member-tier roles),
`roadmap.view`, `roadmap.create`, `roadmap.edit`, `roadmap.delete`,
`roadmap.moderate` — seeded to `super_admin` and `admin` groups; added to the
permission-key-naming convention doc.

## 6. Public frontend — `apps/main`

All data from the API (Rule 16); i18n keys in `en.json` + `fr.json`.

- **`/roadmap`** — SSR board: status columns (Under Consideration → Planned →
  In Progress → Recently Shipped), flagship item pinned at top ("Now building:
  Spaces & Content Creation"), category filter chips, sort by votes/recency,
  vote buttons (anonymous → login prompt), "Submit a request" CTA.
- **`/roadmap/[slug]`** — item detail: full body (markdown rendered like
  comments), vote button, status stepper, `target_window`, feature-flag
  rollout badge, comments thread (existing comment components; comment
  counts come from the comments API), related items.
- **`/roadmap/changelog`** — shipped items grouped by `shipped_at`, release-notes
  style.
- **`/roadmap/submit`** (or dialog) — auth-required form: title, description,
  category select.
- **Subdomain** — `hooks.server.ts` detects `roadmap.japa.to` (+
  `roadmap.custom_domain` setting support) → `event.locals.isRoadmapHost`;
  root layout serves a minimal roadmap-branded shell (no app chrome); all
  routes on that host render the roadmap surface.
- SEO: sitemap entries, OG meta, `robots.txt` allowance on the roadmap host.

## 7. Admin & inline editing

- **`apps/admin` → Roadmap section**
  - Items DataTable (full spec: sort/search/filter/paginate/row actions/bulk
    actions/create form)
  - Submissions triage queue: approve → `under_review`, decline → `declined`
  - Categories management page
- **Inline edit (public page)**: users carrying `roadmap.moderate` see a status
  dropdown and edit affordances on roadmap cards/detail pages; mutations call
  the admin endpoints and invalidate the board cache.

## 8. Notifications & background work

- Asynq task `roadmap:ship_notify` (registered in §11 catalog, `default` queue):
  on `→ shipped`, notify all voters + `submitted_by` via the notifications
  system. Enqueued fire-and-forget from the status handler (Rule 8).
- Cache invalidation: any item/category/vote mutation deletes `jc:roadmap:board:*`.

## 9. Edge cases & error handling

- Repeat vote → `409 already_voted`; unauthenticated vote/submit → `401`;
  flag disabled → `404` on every roadmap route.
- Submissions pass the spam middleware + `form_submission` rate tier; `pending`
  items are never exposed via public routes (service filters by status).
- Admin hard-delete path requires `admin.hard_delete` + `?force=true` on
  already-soft-deleted items (Rule 11).
- GDPR: `roadmap_votes` cascade-deletes on user erasure; comments/shadow
  content inherit existing erasure flows.
- Duplicate slugs: submissions get `title-slug + shortid` suffix.
- `content_hash` on the shadow contents row is computed per Rule 12; the hash
  derives from item body text.

## 10. Testing

- Go: service tests for lifecycle transitions, vote idempotency, board
  grouping/caching; handler tests following existing `*_test.go` patterns.
- Frontend: `bun run check` / `svelte-check`; Playwright smoke for board,
  voting, submission flow, subdomain host mode.
- Regression checklist: feed, search, sitemap, trending verified to exclude
  `content_type='roadmap'` rows.

## 11. JEOS updates required (before code — AGENTS.md ordering)

1. `sections/113-public-product-roadmap.md` — full feature section
2. `adr/ADR-127-public-product-roadmap.md` — shadow-content comments decision,
   ADR-063 host-detection reuse, dedicated-module rationale
3. Catalog entries: `api-routes-catalog.md`, `valkey-keys-catalog.md`
   (`jc:roadmap:*`), `system-settings-catalog.md`
   (`roadmap.submissions_enabled`, `roadmap.custom_domain`,
   `roadmap.board_cache_ttl_seconds`, `roadmap.shipped_window_days`),
   `permission-key-naming.md`, `MIGRATION-ORDER.md`, §11 Asynq task
   (`roadmap:ship_notify`), `internal/features` registry (`roadmap.enabled`,
   default `true`)

## 12. Out of scope

- Anonymous voting (logged-in only)
- Per-tenant/private roadmaps
- Automatic status sync from feature-flag state (the `feature_flag_key` badge
  is informational only)
- Email digests of roadmap changes (notifications are in-app only)
