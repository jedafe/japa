# Phase 2 Content Patterns

**Date:** 2026-06-25
**Phase:** 2
**Purpose:** Implementation patterns learned during Section 14 (Content Type System), reusable in future sections.

## Pattern: Content Type Discriminator (Single Table, JSONB field_values)

All content on JapaConnect — blog posts, discussions, pages, comparisons — lives
in a single `contents` table discriminated by a `content_type` column. Type-specific
fields are stored as JSONB in `field_values`, pulling from the universal custom
field architecture (Section 06). This avoids a table-per-type explosion while
keeping type safety at the application layer via constants:

```go
const (
    TypeBlogPost   = "blog_post"
    TypeDiscussion = "discussion"
    TypePage       = "page"
    TypeComparison = "comparison"
)
```

The discriminator is a plain `text` column (not an enum) so admin-created content
types can be added without a migration. Query filters use the constant values:

```sql
-- name: ListContentsByType :many
SELECT id, title, content_type, field_values, created_at
FROM contents
WHERE tenant_id = @tenant_id AND content_type = @content_type
  AND status = 'published'
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;
```

Type-specific field validation happens at the handler boundary by resolving the
content type's field group from Section 06 — not in the database.

## Pattern: Revision-on-Save (Every Update Creates a Revision Row)

Every successful `UpdateContent` call creates a `content_revisions` row capturing
the complete `field_values` and `body` snapshot before the update is applied. This
gives full rollback capability and a complete audit trail without a separate event
sourcing system:

```go
func (s *Service) UpdateContent(ctx context.Context, params UpdateContentParams) (Content, error) {
    // 1. Fetch the existing content
    existing, err := s.read.GetContentByID(ctx, GetContentByIDParams{...})
    // 2. Insert a revision capturing the current state
    s.write.CreateContentRevision(ctx, CreateContentRevisionParams{
        ContentID: existing.ID,
        Body:      existing.Body,
        FieldValues: existing.FieldValues,
        // ...
    })
    // 3. Apply the update
    return s.write.UpdateContent(ctx, params)
}
```

Revisions are append-only — no update or delete queries exist for them. The
`ListRevisions` handler is gated by `content.revisions.view` permission so only
authorized users see edit history. Revision rows carry `tenant_id` for tenant
isolation and cascade-delete with the parent content.

## Pattern: Slug History and 301 Redirects

When a content's slug changes, the old slug is recorded in `slug_history` with
the new slug. The `ResolveSlugRedirect` function looks up an old slug and returns
the current one, enabling 301 redirects that preserve SEO equity across renames:

```go
func (s *Service) UpdateContent(ctx context.Context, params UpdateContentParams) (Content, error) {
    existing, _ := s.read.GetContentByID(ctx, ...)
    // If slug changed, record the old → new mapping
    if params.Slug != existing.Slug {
        s.write.RecordSlugChange(ctx, SlugHistoryParams{
            EntityType: "content",
            EntityID:   existing.ID,
            OldSlug:    existing.Slug,
            NewSlug:    params.Slug,
        })
    }
    return s.write.UpdateContent(ctx, params)
}
```

The `slug_history` table is polymorphic (`entity_type`/`entity_id`) so the same
mechanism can serve other slug-bearing entities (spaces, users, organizations)
without schema changes. Redirects are time-ordered — the most recent
`old_slug → new_slug` entry wins. SEO crawlers follow the chain and the link
equity transfers.

## Pattern: Polymorphic Reactions (entity_type/entity_id)

Reactions are not tied to the `contents` table — they are polymorphic via
`entity_type`/`entity_id`, allowing the same `reactions` table to serve blog
posts, discussions, comments, and any future reactable entity. Composite primary
key `(entity_type, entity_id, user_id, reaction_key)` ensures one reaction per
user per entity per type:

```sql
-- name: AddReaction :exec
INSERT INTO reactions (entity_type, entity_id, user_id, reaction_key, tenant_id)
VALUES (@entity_type, @entity_id, @user_id, @reaction_key, @tenant_id)
ON CONFLICT (entity_type, entity_id, user_id, reaction_key) DO NOTHING;
```

`reaction_types` are admin-configurable (not hardcoded). Six defaults are seeded
(like, support, celebrate, insightful, grateful, funny) but tenants can create
custom reaction sets. The `is_system` flag protects seeded types from API
deletion. Reaction type keys are tenant-scoped via `UNIQUE(tenant_id, key)` so
two tenants can define a reaction with the same key independently.

## Pattern: Content Scores with pgvector (Activity Feed Relevance)

The `content_scores` table backs the Activity Feed (Section 32) ranking. It
carries a `relevance_embedding vector(1536)` column for semantic similarity and
five scalar score columns (`vote_score`, `comment_score`, `view_score`,
`bookmark_score`, `velocity_score`) for engagement-based ranking. The embedding
column is created in migration 000010 so the schema is stable, but it is not
populated in this section — the AI embedding pipeline in Section 32 writes to it:

```sql
-- name: UpsertContentScore :one
INSERT INTO content_scores (content_id, tenant_id, relevance_embedding,
    vote_score, comment_score, view_score, bookmark_score, velocity_score)
VALUES (@content_id, @tenant_id, @relevance_embedding,
    @vote_score, @comment_score, @view_score, @bookmark_score, @velocity_score)
ON CONFLICT (content_id, tenant_id) DO UPDATE SET
    relevance_embedding = EXCLUDED.relevance_embedding,
    vote_score = EXCLUDED.vote_score, ...
RETURNING *;
```

The `github.com/pgvector/pgvector-go` dependency is required by the generated
`ContentScore.RelevanceEmbedding` field type. Without it, `sqlc generate` fails
and the `gen` package will not compile. Add the dependency before running
`sqlc generate` when a query file references `vector` types.

## Pattern: Per-Tenant JSON-LD Templates

Structured data (Schema.org JSON-LD) is per-tenant and per-content-type. The
`content_type_jsonld_templates` table stores a template string that is rendered
with content-specific values at request time. Templates are upserted via
`ON CONFLICT (tenant_id, content_type) DO UPDATE`:

```sql
-- name: UpsertJSONLDTemplate :one
INSERT INTO content_type_jsonld_templates (tenant_id, content_type, template)
VALUES (@tenant_id, @content_type, @template)
ON CONFLICT (tenant_id, content_type) DO UPDATE SET template = EXCLUDED.template
RETURNING *;
```

This lets each tenant customise how their content appears in search results and
social previews without code changes. The template is stored as a string, not
JSONB, because the structure varies by content type (BlogPosting vs. Discussion
vs. Product) and tenants may include custom Schema.org properties.

## Lesson: Always Verify Generated Signatures Before Writing Wrappers

sqlc's return-type inference depends on the query shape. A query that selects a
single boolean-expressed column returns `(bool, error)` directly; a query that
selects multiple columns returns a struct. The generated `IsContentPinned`
function returns `(bool, error)` — not a `gen.IsContentPinnedRow` struct with an
`Exists` field:

```go
// Generated — bare return, not a struct
func (q *Queries) IsContentPinned(ctx context.Context, arg IsContentPinnedParams) (bool, error) {
    row := q.db.QueryRow(ctx, isContentPinned, arg.SpaceID, arg.ContentID)
    var exists bool
    err := row.Scan(&exists)
    return exists, err
}
```

Before writing a service-layer wrapper around a generated query, read the
generated signature in `internal/db/gen/*.sql.go`. Do not assume the return shape
from the SQL alone — a `SELECT EXISTS(...)` query produces a bare bool, but a
`SELECT EXISTS(...) AS exists, count(*) AS total` query produces a struct. The
generated code is the source of truth, not the SQL file.

This applies to all sqlc-generated code: param struct field types, return types,
and whether a nullable column produces a pointer or a `pgtype` wrapper all depend
on the query annotation (`sqlc.narg`, `sqlc.arg`, `sqlc.nil`) and the column's
nullability. When in doubt, read the generated file.