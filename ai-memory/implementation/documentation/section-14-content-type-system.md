# Section 14 — Content Type System: Implementation Complete

**Date:** 2026-06-25
**Phase:** 2
**Section:** 14
**Status:** Complete

## Overview

Section 14 implements the content type system that powers every editorial and
community post on JapaConnect. A single polymorphic `contents` table discriminated
by a `content_type` column (blog_post, discussion, page, comparison) stores all
content bodies, with type-specific fields stored as JSONB in `field_values` — the
same universal custom field architecture from Section 06. Every save creates a
full revision row for rollback and audit. Slug changes are recorded in
`slug_history` for 301 redirects that preserve SEO equity across renames.

The section also introduced admin-configurable polymorphic reactions
(entity_type/entity_id/user_id/reaction_key), per-tenant JSON-LD templates for
structured data output per content type, a pgvector-backed `content_scores` table
for the Activity Feed (Section 32) with a 1536-dimension relevance embedding
column, and the `space_pinned_posts` many-to-many table deferred from Section 05.

## Components Implemented

### Migration 000010 — Content Type System

10 migration files now in `migrations/` (000001-000010); 8 tables created in this
migration:

1. **contents** — Core content table, tenant-scoped, unique on
   `(tenant_id, slug)`. Discriminator column `content_type`
   (blog_post/discussion/page/comparison). `field_values` JSONB pulls type-specific
   fields from Section 06 custom field groups. `content_hash` (SHA-256) enforces
   uniqueness per tenant. `read_time_minutes` auto-computed (200 wpm).
   `canonical_url` for SEO, `publish_at` for scheduled posts, `geo_space_id` FK to
   `spaces` (Section 05) for spatial scoping. `header_image_id` / `featured_image_id`
   as plain `uuid` (media table is Section 34). Engagement counters:
   `view_count`, `comment_count`, `share_count`, `reshare_count`. Status
   (draft/published/scheduled/archived), visibility (public/members/private)
2. **content_revisions** — One row per save, full history for rollback and audit.
   Stores the complete `field_values` and `body` snapshot per revision
3. **slug_history** — `entity_type`/`entity_id`/`old_slug`/`new_slug` for 301
   redirects (SEO equity preservation across renames)
4. **reaction_types** — Admin-configurable emoji set, tenant-scoped, unique on
   `(tenant_id, key)`. 6 seeded defaults: like, support, celebrate, insightful,
   grateful, funny
5. **reactions** — Polymorphic (`entity_type`/`entity_id`/`user_id`/`reaction_key`),
   composite PK ensures one reaction per user per entity per type
6. **content_scores** — `relevance_embedding vector(1536)` (pgvector) for semantic
   similarity, plus `vote_score`, `comment_score`, `view_score`, `bookmark_score`,
   `velocity_score` columns for Activity Feed ranking (Section 32)
7. **content_type_jsonld_templates** — Per-tenant JSON-LD template per
   `content_type` for structured data enrichment (Schema.org output)
8. **space_pinned_posts** — Many-to-many `space_id` × `content_id`, deferred from
   Section 05 because the `contents` table did not exist yet

Seed data:

- 6 system reaction types with deterministic UUIDs (like, support, celebrate,
  insightful, grateful, funny)

Conventions: `tenant_id` FK with `ON DELETE CASCADE` on tenant-scoped tables,
`gen_random_uuid()` for primary keys, `UNIQUE` constraints for natural keys,
`idx_` prefixed indexes, deterministic UUIDs for system seeds, `is_system` flag
protecting seeded reaction types from API deletion.

Design decisions:

- **`header_image_id` / `featured_image_id` are plain `uuid` without FK
  constraints** — the media table (Section 34) does not exist yet; FK
  constraints will be added in a later migration when the media system is built.
  Storing the columns now avoids a schema migration later.
- **`content_scores.relevance_embedding` populated in Section 32** — the
  pgvector column is created here so the schema is stable, but the AI embedding
  pipeline that writes 1536-dimension vectors is part of the Activity Feed.
- **Full-text search deferred to Meilisearch (Section 20)** — PostgreSQL
  `tsvector` is not used for content search; Meilisearch will index `contents`
  rows for typo-tolerant, faceted search.
- **Scheduled publish task wiring deferred to Phase 6 (Asynq)** — the
  `scheduled_publish` task type is defined but the real Asynq producer is
  stubbed via `noopEnqueuer`, same pattern as Section 05.

### sqlc Queries

30 queries across 2 SQL files (268 lines total):

- `internal/db/queries/contents.sql` (207 lines, 20 queries):
  content CRUD (CreateContent, GetContentByID, GetContentBySlug,
  ListContentsByTenant, ListContentsByType, ListContentsBySpace, UpdateContent,
  UpdateContentStatus, IncrementContentView, DeleteContent), cursor pagination
  by tenant/type/space, content revisions, slug history, pinned posts, JSON-LD
  templates, content scores
- `internal/db/queries/reactions.sql` (61 lines, 10 queries): reaction types
  CRUD (list, get, create, update, delete), polymorphic reactions (add, remove,
  list by entity, count by entity, get user reactions)

Generated code:

- `internal/db/gen/contents.sql.go` (1,049 lines)
- `internal/db/gen/reactions.sql.go` (325 lines)
- 8 new model structs in `models.go`: `Content`, `ContentRevision`,
  `SlugHistory`, `ReactionType`, `Reaction`, `ContentScore`,
  `ContentTypeJsonldTemplate`, `SpacePinnedPost`
- New dependency: `github.com/pgvector/pgvector-go` — required by
  `ContentScore.RelevanceEmbedding`

### internal/content Package (6 files)

**content.go** (74 lines)

- Status constants: `StatusDraft`, `StatusPublished`, `StatusScheduled`,
  `StatusArchived`
- Type constants: `TypeBlogPost`, `TypeDiscussion`, `TypePage`,
  `TypeComparison`
- Visibility constants: `VisibilityPublic`, `VisibilityMembers`,
  `VisibilityPrivate`
- Entity constants for polymorphic reactions: `EntityContent`,
  `EntityDiscussion`, `EntityPage`, `EntityComparison`
- Page size constants: `DefaultPageSize`, `MaxPageSize`
- `Service` struct with reader/writer `*gen.Queries` pool split
- `NewService` constructor

**slug.go** (107 lines)

- `Slugify` — normalises a title to a URL-safe slug
- `EnsureUniqueSlug` — appends `-2`, `-3`, ... suffixes up to
  `MaxSlugAttempts = 100` when a slug already exists for the tenant
- `RecordSlugChange` — writes to `slug_history` when a content's slug changes
- `ResolveSlugRedirect` — looks up the current slug for a 301 redirect
- `ComputeReadTime` — estimates `read_time_minutes` from body word count at
  200 words per minute

**tasks.go** (46 lines)

- `TaskTypeScheduledPublish = "scheduled_publish"` and
  `TaskTypeComputeScores = "compute_scores"` constants
- `TaskEnqueuer` interface with `EnqueueScheduledPublish` and
  `EnqueueComputeScores` methods
- `noopEnqueuer` implementation — logs via `slog.Info` and returns nil
  (real Asynq wiring deferred to Phase 6)

**store.go** (570 lines)

- 27 `Service` methods wrapping sqlc-generated queries across 7 entity groups:
  - **Contents (10):** `CreateContent` (auto-slug, revision, slug-history
    tracking), `GetContent`, `GetContentBySlug`, `ListContents`,
    `ListContentsByType`, `ListContentsBySpace`, `UpdateContent` (creates
    revision + records slug change if slug changed), `UpdateContentStatus`,
    `IncrementContentView`, `DeleteContent`
  - **Revisions (2):** `ListRevisions`, `GetRevision`
  - **Pinned posts (3):** `PinContent`, `UnpinContent`, `IsContentPinned`
  - **Reactions (5):** `AddReaction`, `RemoveReaction`, `ListReactions`,
    `CountReactions`, `GetUserReactions`
  - **Reaction types (4):** `ListReactionTypes`, `CreateReactionType`,
    `UpdateReactionType`, `DeleteReactionType`
  - **JSON-LD templates (2):** `ListJSONLDTemplates`, `UpsertJSONLDTemplate`
  - **Content scores (1):** `UpsertContentScore`
- Read methods use `s.read`; write methods use `s.write`
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context

**handlers.go** (562 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 13 HTTP handlers:
  - **Content CRUD (5):** `CreateContent`, `GetContent`, `ListContents`,
    `UpdateContent`, `DeleteContent`
  - **Revisions (1):** `ListRevisions`
  - **Pins (2):** `PinContent`, `UnpinContent`
  - **Reactions (3):** `AddReaction`, `RemoveReaction`, `ListReactions`
  - **JSON-LD templates (2):** `ListJSONLDTemplates`, `UpsertJSONLDTemplate`
- Helpers: `parseUUIDParam`, `requireTenant`, `requireUser`, `handleServiceError`
  (maps `pgx.ErrNoRows` to 404, duplicate errors to 409, other to 500),
  `writeJSON`, `writeError`, `parsePageSize`
- Uses `auth.FromContext`, `tenant.FromContext`, and `pgtype.UUID` — no hard-coded
  role checks, all access controlled via RBAC permission keys

**reactions.go** (104 lines)

- 4 admin reaction-type handlers on the same `Handlers` struct:
  `ListReactionTypes`, `CreateReactionType`, `UpdateReactionType`,
  `DeleteReactionType` — all gated by `content.reaction_types.manage` permission

**content_test.go** (229 lines)

- 10 unit tests: `Slugify`, `ComputeReadTime`, constants integrity, `writeJSON`/
  `writeError` envelope correctness, `parsePageSize` bounds enforcement,
  `NoopEnqueuer` behavior

### Server Wiring

- `content.NewService(database)` and `content.NewHandlers(contentService)` created
  in `NewServer`
- 19 API routes registered under `/api/v1`, all within the tenant-scoped route
  group
- 11 new permission keys bootstrapped at startup via the existing
  `bootstrapPermissions` function (added to the `permissions` slice alongside
  the 20 from Sections 05 and 06)
- All 11 new permissions assigned to the `super_admin_role`, granted to the
  `super_admin` system group (idempotent)

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/config | 4 | PASS |
| internal/server | 9 | PASS |
| internal/tenant | 3 | PASS |
| internal/auth | 22 | PASS |
| internal/rbac | 4 | PASS |
| internal/features | 5 | PASS |
| internal/geoip | 7 | PASS |
| internal/valkey | 6 | PASS |
| internal/customfields | 37 | PASS |
| internal/spaces | 13 | PASS |
| internal/content | 10 | PASS |
| **Total** | **119** | **ALL PASS** |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok — all 11 packages pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

19 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/contents` | Required | `content.create` | Create content (auto-slug, revision) |
| 2 | GET | `/contents` | Required | — | List/filter contents (by type, space, status) |
| 3 | GET | `/contents/{id}` | Required | — | Get content by ID |
| 4 | GET | `/contents/{slug}` | Required | — | Get content by slug |
| 5 | PATCH | `/contents/{id}` | Required | `content.edit` | Update content (revision + slug history) |
| 6 | DELETE | `/contents/{id}` | Required | `content.delete` | Delete content |
| 7 | GET | `/contents/{id}/revisions` | Required | `content.revisions.view` | List revisions |
| 8 | POST | `/contents/{id}/pin` | Required | `content.pin` | Pin content to a space |
| 9 | DELETE | `/contents/{id}/pin` | Required | `content.pin` | Unpin content |
| 10 | POST | `/contents/{id}/reactions` | Required | `content.reactions.add` | Add reaction |
| 11 | DELETE | `/contents/{id}/reactions` | Required | `content.reactions.remove` | Remove reaction |
| 12 | GET | `/contents/{id}/reactions` | Required | — | List reactions for content |
| 13 | GET | `/reaction-types` | Required | — | List reaction types |
| 14 | POST | `/reaction-types` | Required | `content.reaction_types.manage` | Create reaction type |
| 15 | PATCH | `/reaction-types/{id}` | Required | `content.reaction_types.manage` | Update reaction type |
| 16 | DELETE | `/reaction-types/{id}` | Required | `content.reaction_types.manage` | Delete reaction type |
| 17 | GET | `/jsonld-templates` | Required | — | List JSON-LD templates |
| 18 | PUT | `/jsonld-templates` | Required | `content.jsonld.manage` | Upsert JSON-LD template |
| 19 | GET | `/contents/{id}/scores` | Required | `content.scores.view` | Get content scores |

## Permission Keys Registered

11 new permission keys bootstrapped at server startup (added to the existing
`bootstrapPermissions` slice alongside the 20 from Sections 05 and 06):

| Key | Description |
|-----|-------------|
| `content.create` | Create content |
| `content.edit` | Edit content |
| `content.delete` | Delete content |
| `content.revisions.view` | View content revision history |
| `content.pin` | Pin/unpin content to spaces |
| `content.reaction_types.manage` | Manage reaction types (admin) |
| `content.reactions.add` | Add reactions to content |
| `content.reactions.remove` | Remove own reactions |
| `content.scores.view` | View content engagement scores |
| `content.jsonld.manage` | Manage JSON-LD templates |
| `content.scheduled.view` | View scheduled content |

All 31 permissions (11 Section 06 + 9 Section 05 + 11 Section 14) are assigned to
the `super_admin_role`, which is granted to the `super_admin` system group. Every
operation is idempotent.

## Files Created

| # | File | Purpose |
|---|------|---------|
| 1 | `migrations/000010_content_type_system.sql` | 8 tables, 6 reaction type seeds, indexes |
| 2 | `internal/db/queries/contents.sql` | 20 sqlc queries for content, revisions, slugs, pins, JSON-LD, scores |
| 3 | `internal/db/queries/reactions.sql` | 10 sqlc queries for reaction types and polymorphic reactions |
| 4 | `internal/db/gen/contents.sql.go` | Generated content query methods (1,049 lines) |
| 5 | `internal/db/gen/reactions.sql.go` | Generated reaction query methods (325 lines) |
| 6 | `internal/content/content.go` | Status/type/visibility/entity constants, Service struct |
| 7 | `internal/content/slug.go` | Slugify, EnsureUniqueSlug, slug history, ComputeReadTime |
| 8 | `internal/content/tasks.go` | TaskEnqueuer interface with noopEnqueuer stub |
| 9 | `internal/content/store.go` | 27 Service methods wrapping sqlc queries |
| 10 | `internal/content/handlers.go` | 13 HTTP handlers with helpers |
| 11 | `internal/content/reactions.go` | 4 admin reaction-type handlers |
| 12 | `internal/content/content_test.go` | 10 unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `content` import; created content service and handlers; registered 19 routes with auth/RBAC middleware; added 11 content permission keys to `bootstrapPermissions` (366 lines total) |
| 2 | `internal/db/gen/models.go` | Regenerated by `sqlc generate` to include structs for the 8 new tables |
| 3 | `internal/db/gen/db.go` | Regenerated by `sqlc generate` to register the new query methods |
| 4 | `go.mod` | Added `github.com/pgvector/pgvector-go` dependency |
| 5 | `go.sum` | Updated checksums for pgvector-go dependency |

## Key Decisions

- **`IsContentPinned` returns `(bool, error)` directly, not a row struct** — the
  generated `gen.IsContentPinned` function returns a bare `(bool, error)` pair
  rather than a `gen.IsContentPinnedRow` struct with an `Exists` field. Always
  check generated signatures in `internal/db/gen/*.sql.go` before writing
  service-layer wrappers — sqlc's return-type inference depends on the query
  shape (single boolean column → bare return vs. multiple columns → struct).
- **`header_image_id` / `featured_image_id` as plain `uuid`** — the media table
  (Section 34) does not exist yet, so FK constraints are deferred. Same pattern
  as `logo_media_id` / `header_image_id` on `spaces` in Section 05.
- **`space_pinned_posts` deferred from Section 05, now complete** — Section 05
  noted this table as a known issue because it needs a FK to `contents`. Both
  tables now exist; the schema gap is closed.
- **`content_scores.relevance_embedding` column created now, populated later** —
  the pgvector column is in the schema so it is stable, but the AI embedding
  pipeline that writes 1536-dimension vectors is part of Section 32 (Activity
  Feed). No migration backfill needed later.
- **Full-text search deferred to Meilisearch (Section 20)** — PostgreSQL
  `tsvector` is intentionally not added. Meilisearch will index `contents` rows
  for typo-tolerant, faceted search.

## Known Issues and Deferrals

- **Asynq scheduled publish wiring deferred to Phase 6** — the
  `scheduled_publish` task type is defined and the `noopEnqueuer` stub logs the
  enqueue request, but no real Asynq producer or consumer exists yet. Same
  pattern as `space:auto_populate` in Section 05.
- **AI embedding pipeline deferred to Section 32** — the
  `content_scores.relevance_embedding` column is in the schema but never written
  to in this section. The Activity Feed will populate it via the AI Gateway.
- **Meilisearch indexing deferred to Section 20** — content rows are stored in
  PostgreSQL only; the search index sync is not yet wired.
- **Media FK constraints deferred to Section 34** — `header_image_id` and
  `featured_image_id` are plain `uuid`. The media system will backfill FK
  constraints in a later migration.

## Next Steps

Section 15 — Block Editor (TipTap). This section depends on:

- Section 14 content rows (the editor writes to `contents.body` and
  `contents.field_values`)
- Section 06 custom fields (for type-specific editor schemas)
- The Valkey client wired in Section 06 (for editor cache and edit locks)