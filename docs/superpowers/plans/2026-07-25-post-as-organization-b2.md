# Post-as-Organization + Org Content Tabs (Phase B, Sub-project B2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give organizations real authored content via a dual-attribution `contents.organization_id` link ("post as organization"), expose the org profile's Posts/Media/Spaces tabs (guest-readable), and badge org-attributed posts on the author's own profile (dual-surface).

**Architecture:** Layered Handler(Chi)→Service→Store(sqlc/pgx)→PostgreSQL. A new nullable `contents.organization_id` (FK organizations) sits alongside the always-present `author_id` (the human, for moderation/accountability). Posting/editing *as* an org is gated by `organization_members.role ∈ {owner,admin,editor}`, checked in the content service via the already-generated `GetOrganizationMemberRole` query (no cross-package service dependency). The three org content tabs mirror the Phase A user tabs (`content`/`media`/`spaces` packages each own their tab handler), wired under `/organizations/{id}/*` with the **B1 `requireActiveOrgByID` gate** and `OptionalAuth`. Lean DTOs reuse Phase A shapes (`PostCard`, `MediaResponse`, `SpaceCard`). Integration tests run against real Postgres via `internal/testdb`.

**Tech Stack:** Go, Chi, sqlc + pgx/v5, PostgreSQL 18, golang-migrate, `internal/testdb` harness (build tag `integration`).

## Global Constraints

- Multi-tenancy (ADR-004): every query filters `tenant_id`. Every new query MUST be tenant-scoped.
- Authorship model (ADR-062, this plan): dual attribution — new nullable `contents.organization_id` + always-present `author_id NOT NULL`. `author_profile_id` stub is left untouched (orgs are NOT merged with `professional_profiles`).
- Post/edit-as-org permission: caller's `organization_members.role` for the target org must be `owner|admin|editor`; `member`/non-member → 403 (`org_post_forbidden`).
- Org profile tabs = Posts, Media, About, Members, Spaces. Likes/Replies do NOT apply to org authorship (they are user actions).
- Org Spaces tab = DISTINCT public+active spaces the org has published-public content in (`contents.space_id` where `organization_id = org`). "Where the org is active."
- Dual-surface: an org-attributed post appears on BOTH `/org/[slug]` (org Posts tab) AND `/u/[username]` (author's Posts tab, badged). The Phase A user query stays keyed by `author_id`; the user `PostCard` gains org-attribution fields.
- Response contract: success `{"data": ...}` / lists `{"data": [...], "meta": {...}}`; errors `{"error":{"code","message"}}`; IDs as strings, timestamps RFC3339. A lean DTO that is a subset of the sqlc row with the same field types is marshal-identical, only fewer fields.
- Public surfaces: `OptionalAuth` (guest-readable), lean DTOs (NO internal columns), 404 for non-active/deleted orgs via `requireActiveOrgByID`.
- Work-on-main-in-place: `git add` ONLY the exact files each task lists — never `git add .`. `sqlc generate` is global; stage ONLY the gen file(s) this plan owns, by exact path. A cron auto-committer may bundle uncommitted files into its own commit — commit each task immediately; if `git status` shows a task's files already clean, verify they are in HEAD's tree and proceed.
- gopls diagnostics in this repo are frequently STALE — trust `go build ./...` (exit 0), not the editor's inline errors. If `go build` reports `could not import <stdlib>` / `-d: no such file or directory`, the build cache was cleaned mid-run: `go clean -cache` then rebuild.
- **Deferred to a follow-on plan (B2 cross-cutting), NOT in scope here:** feed-item org byline (LEFT JOIN organizations across ~7 feed queries), and Meilisearch content-index `organization_id`/org-name enrichment. These are independent subsystems (writing-plans Scope Check) and are additive to the shippable core below.

## File Structure

- `.jeos/adr/ADR-062-organization-content-authorship.md` — CREATE (submodule): the authorship ADR.
- `migrations/000162_contents_organization_id.up.sql` / `.down.sql` — CREATE: add column + partial index.
- `internal/db/queries/contents.sql` — MODIFY: add `organization_id` to CreateContent (INSERT+RETURNING), UpdateContent (SET+RETURNING), GetContentByID (SELECT), ListPublishedContentByAuthor (SELECT + LEFT JOIN); add new `ListPublishedContentByOrganization`.
- `internal/db/queries/media.sql` — MODIFY: add `ListMediaFromOrgPublicPosts` + `CountMediaFromOrgPublicPosts`.
- `internal/db/queries/spaces.sql` — MODIFY: add `ListSpacesByOrgPosts`.
- `internal/db/gen/{contents,media,spaces}.sql.go` — REGENERATED (stage only these three from the gen dir).
- `internal/testdb/seed.go` — MODIFY: `ContentOpts.OrganizationID` + INSERT column.
- `internal/content/content.go` — MODIFY: `ErrOrgPostForbidden` sentinel + `isOrgPostRole` helper.
- `internal/content/store.go` — MODIFY: `CreateContentInput.OrganizationID` / `UpdateContentInput.OrganizationID` + service membership checks + pass-through.
- `internal/content/handlers.go` — MODIFY: parse `organization_id` in create/update; map `ErrOrgPostForbidden` → 403.
- `internal/content/profile_posts.go` — MODIFY: `PostCard` gains org-attribution fields + `OrgRef`; `toPostCards` fills badge; add `ListPublishedByOrganization` service + `ListByOrganization` handler + `toOrgPostCards`.
- `internal/content/post_card_test.go` — MODIFY: add `TestToPostCards_OrgBadge` (reuses the package-level `mustUUID` from `content_test.go`).
- `internal/media/store.go`, `internal/media/service.go`, `internal/media/handlers.go` — MODIFY: org media store/service/handler.
- `internal/spaces/store.go`, `internal/spaces/handlers.go` — MODIFY: org spaces service/handler.
- `internal/server/server.go` — MODIFY: wire 3 new `/organizations/{id}/{content,media,spaces}` routes (OptionalAuth + requireActiveOrgByID).
- `internal/content/org_authorship_integration_test.go`, `internal/content/org_posts_integration_test.go` — CREATE.
- `internal/media/org_media_integration_test.go`, `internal/spaces/org_spaces_integration_test.go` — CREATE.
- `internal/content/post_card_test.go` — MODIFY: `TestToPostCards_OrgBadge` mapper unit test (org tab reuses the same `PostCard` type).
- `.jeos/conventions/api-routes-catalog.md` — MODIFY (submodule): mark org content tabs.

---

### Task 1: ADR-062 — Organization Content Authorship

**Files:**
- Create: `.jeos/adr/ADR-062-organization-content-authorship.md`

**Interfaces:** none (docs). Captures the architectural decision before the migration lands (Gate 1: architectural changes without an ADR are spec drift).

- [ ] **Step 1: Write the ADR**

Create `.jeos/adr/ADR-062-organization-content-authorship.md`:

```markdown
# ADR-062 — Organization Content Authorship (Dual Attribution)

**Status:** Accepted
**Date:** 2026-07-25
**Drivers:** JEOS Section 07 defines `GET /organizations/:id/content` ("org's published content") and the org public profile wants Posts/Media/Spaces tabs, but the schema had no `content → organizations` link: `contents.author_id → users`, `contents.author_profile_id → professional_profiles` (Section 33, a separate parallel system whose write path is an unpopulated stub). Organizations could not author content. Phase A's read-only-reuse approach does not extend to org content without a schema change.
**References:** Design spec `docs/superpowers/specs/2026-07-25-post-as-organization-and-org-public-profile-design.md`; ADR-004 (multi-tenancy); ADR-061 (integration test harness); Section 07 (Organization Profiles); Section 33 (professional profiles — untouched).

---

## Context

Two structurally near-identical profile systems exist: `organizations` (Section 07) and `professional_profiles` (Section 33). Content attaches to `professional_profiles` via a stubbed `author_profile_id` (one read query, no write path). Merging the two systems, or building out the professional-profile authorship stub, would be a large, risky refactor unrelated to the immediate need: letting an organization publish posts that appear on its public profile.

Accountability matters: every post must trace to a human for moderation, even when it speaks for an organization.

## Decision

1. **Dual attribution.** Add a new nullable `contents.organization_id uuid REFERENCES organizations(id)`. `author_id` remains `NOT NULL` (the human who wrote it). `organization_id`, when set, is the org the post is published on behalf of. The `author_profile_id` stub is left as-is; organizations are NOT merged into `professional_profiles`.
2. **Authority.** Setting `organization_id` on create/update requires the caller's `organization_members.role` for that org to be `owner`, `admin`, or `editor`. `member` and non-members receive 403 (`org_post_forbidden`). No new global RBAC node — authority is the org membership role.
3. **Visibility (dual-surface).** An org-attributed post is shown on the org profile Posts tab AND on the authoring user's profile Posts tab (badged "on behalf of <Org>"). The user Posts query stays keyed by `author_id`; the `PostCard` DTO carries optional org-attribution fields.
4. **Scope.** Org profile content = Posts, Media, Spaces (+ About, Members). Likes and Replies do not apply — they are user actions (`reactions.user_id`, `comments.author_user_id`).

## Consequences

- **Positive:** orgs get a first-class content identity with a minimal, reversible schema change; the human author is always known (moderation/accountability preserved); no premature merge of the two profile systems; the org content tabs reuse the Phase A read patterns.
- **Negative / trade-offs:** a second nullable attribution column on `contents` (queries that care about org authorship must remember to filter it); the `author_profile_id` stub remains dead until Section 33 is built out; dual-surface means one post is returned by two different profile queries (intended).
- **Follow-on (separate plan):** feed-item byline resolution and Meilisearch indexing gain `organization_id` so org posts are attributed in feeds and findable/indexable by org.
```

- [ ] **Step 2: Commit (submodule)**

```bash
git -C .jeos add adr/ADR-062-organization-content-authorship.md
git -C .jeos commit -m "docs(adr): ADR-062 organization content authorship (dual attribution)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
(The outer gitlink is left modified for the cron bundler.)

---

### Task 2: Migration — `contents.organization_id` + partial index

**Files:**
- Create: `migrations/000162_contents_organization_id.up.sql`, `migrations/000162_contents_organization_id.down.sql`

**Interfaces:**
- Produces: column `contents.organization_id uuid NULL REFERENCES organizations(id)`; index `idx_contents_org`.

REQUIRED SUB-SKILL for this task: `jc-backend-migrate` (up/down pairing, zero-downtime, post-migration sqlc regen). This migration is additive (nullable column + index) → zero-downtime safe.

- [ ] **Step 1: Write the up migration `migrations/000162_contents_organization_id.up.sql`**

```sql
-- Post-as-organization (ADR-062): dual attribution. author_id stays NOT NULL
-- (the human author); organization_id is the optional org the post speaks for.
ALTER TABLE contents ADD COLUMN organization_id uuid REFERENCES organizations(id);

-- Powers the org Posts tab and org-attributed lookups: org's published-public
-- posts, newest first. Partial (only org-attributed, live rows) to stay small.
CREATE INDEX idx_contents_org ON contents (tenant_id, organization_id, status, published_at DESC)
  WHERE organization_id IS NOT NULL AND deleted_at IS NULL;
```

- [ ] **Step 2: Write the down migration `migrations/000162_contents_organization_id.down.sql`**

```sql
DROP INDEX IF EXISTS idx_contents_org;
ALTER TABLE contents DROP COLUMN IF EXISTS organization_id;
```

- [ ] **Step 3: Apply the migration**

Run: `make migrate-up` (or the repo's forward-apply target; confirm via `Makefile`).
Expected: applies `000162`, exit 0. Verify: `psql "$JC_DATABASE_URL" -c '\d contents' | grep organization_id` shows the column.

- [ ] **Step 4: Commit**

```bash
git add migrations/000162_contents_organization_id.up.sql migrations/000162_contents_organization_id.down.sql
git commit -m "feat(db): contents.organization_id + partial index — post-as-org (Phase B2, ADR-062)"
```

---

### Task 3: sqlc plumbing — thread `organization_id` through content queries + seeder

**Files:**
- Modify: `internal/db/queries/contents.sql`, `internal/testdb/seed.go`
- Regenerate: `internal/db/gen/contents.sql.go`

**Interfaces:**
- Produces: `gen.CreateContentParams.OrganizationID pgtype.UUID`; `gen.UpdateContentParams.OrganizationID pgtype.UUID`; `organization_id` present on `gen.CreateContentRow`, `gen.UpdateContentRow`, `gen.GetContentByIDRow`; `testdb.ContentOpts.OrganizationID pgtype.UUID`.
- Consumes: Task 2's column.

- [ ] **Step 1: Add `organization_id` to `CreateContent` in `internal/db/queries/contents.sql`**

Replace the `CreateContent` query (lines ~7-21) with:

```sql
-- name: CreateContent :one
INSERT INTO contents (
    tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
    field_values, content_hash, geo_space_id, canonical_url, status, visibility,
    read_time_minutes, publish_at, published_at
) VALUES (
    @tenant_id, @author_id, sqlc.narg(organization_id), sqlc.narg(space_id), @content_type, @title, @slug, @body, @body_html,
    @field_values, sqlc.narg(content_hash), sqlc.narg(geo_space_id), sqlc.narg(canonical_url),
    @status, @visibility, @read_time_minutes, sqlc.narg(publish_at), sqlc.narg(published_at)
)
RETURNING id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at;
```

- [ ] **Step 2: Add `organization_id` to `GetContentByID` (SELECT) in `internal/db/queries/contents.sql`**

In the `GetContentByID` query (lines ~24-30), add `organization_id` to the projection right after `author_id`:

```sql
-- name: GetContentByID :one
SELECT id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at
FROM contents
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;
```

- [ ] **Step 3: Add `organization_id` to `UpdateContent` (SET + RETURNING) in `internal/db/queries/contents.sql`**

In `UpdateContent` (lines ~84-97): add `organization_id = sqlc.narg(organization_id)` to the SET list, and `organization_id` to the RETURNING projection (after `author_id`):

```sql
-- name: UpdateContent :one
UPDATE contents
SET title = @title, slug = @slug, body = @body, body_html = @body_html,
    field_values = @field_values, content_hash = sqlc.narg(content_hash),
    geo_space_id = sqlc.narg(geo_space_id), canonical_url = sqlc.narg(canonical_url),
    status = @status, visibility = @visibility, read_time_minutes = @read_time_minutes,
    publish_at = sqlc.narg(publish_at), published_at = sqlc.narg(published_at),
    is_locked = @is_locked, is_pinned = @is_pinned,
    organization_id = sqlc.narg(organization_id), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at;
```

- [ ] **Step 4: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0. Verify: `grep -n 'OrganizationID' internal/db/gen/contents.sql.go` shows the field on `CreateContentParams`, `UpdateContentParams`, `CreateContentRow`, `UpdateContentRow`, `GetContentByIDRow`.

- [ ] **Step 5: Extend the content seeder `internal/testdb/seed.go`**

Add `OrganizationID` to `ContentOpts` and include it in the INSERT. Replace the `ContentOpts` struct and the INSERT in `SeedContent`:

```go
type ContentOpts struct {
	Title           string
	Status          string
	Visibility      string
	FeaturedImageID pgtype.UUID
	OrganizationID  pgtype.UUID
	SpaceID         pgtype.UUID
}
```

In `SeedContent`, replace the `INSERT INTO contents (...)` exec with:

```go
	_, err := d.Writer.Exec(ctx,
		`INSERT INTO contents (id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html, status, visibility, featured_image_id)
		 VALUES ($1, $2, $3, $4, $5, 'discussion', $6, $7, 'Seeded test body.', '<p>Seeded test body.</p>', $8, $9, $10)`,
		contentID, tenantID, authorID, o.OrganizationID, o.SpaceID, title, slug, status, visibility, o.FeaturedImageID)
```

(`o.OrganizationID` / `o.SpaceID` zero-value `pgtype.UUID{}` inserts SQL NULL — backward-compatible with existing callers.)

- [ ] **Step 6: Build check**

Run: `go build ./... && go vet -tags integration ./internal/testdb/`
Expected: exit 0. (If `could not import <stdlib>` appears: `go clean -cache`, retry.)

- [ ] **Step 7: Commit**

```bash
git add internal/db/queries/contents.sql internal/db/gen/contents.sql.go internal/testdb/seed.go
git commit -m "feat(content): thread organization_id through content queries + seeder (Phase B2)"
```

---

### Task 4: Write-path — post as organization (CreateContent permission)

**Files:**
- Modify: `internal/content/content.go`, `internal/content/store.go`, `internal/content/handlers.go`
- Test: `internal/content/org_authorship_integration_test.go` (create)

**Interfaces:**
- Produces: `content.ErrOrgPostForbidden` (sentinel error); `content.CreateContentInput.OrganizationID pgtype.UUID`; create handler accepts `"organization_id"` and returns 403 `org_post_forbidden` when the caller lacks `owner|admin|editor`.
- Consumes: `gen.GetOrganizationMemberRole` (already generated), Task 3's `gen.CreateContentParams.OrganizationID`.

- [ ] **Step 1: Add the sentinel error + role helper to `internal/content/content.go`**

Add near the top of the file (after imports; ensure `errors` is imported):

```go
// ErrOrgPostForbidden is returned when a caller tries to attribute content to
// an organization they are not an owner/admin/editor of (ADR-062).
var ErrOrgPostForbidden = errors.New("not authorized to post as this organization")

// isOrgPostRole reports whether an org membership role may post/edit as the org.
func isOrgPostRole(role string) bool {
	return role == "owner" || role == "admin" || role == "editor"
}
```

- [ ] **Step 2: Add `OrganizationID` to `CreateContentInput` in `internal/content/store.go`**

Add the field to the `CreateContentInput` struct (after `AuthorID`):

```go
	AuthorID       pgtype.UUID
	OrganizationID pgtype.UUID
	SpaceID        pgtype.UUID
```

- [ ] **Step 3: Enforce membership + pass through in `CreateContent` (`internal/content/store.go`)**

In `CreateContent`, immediately before the `s.write.CreateContent(...)` call, insert the membership check; and add `OrganizationID: input.OrganizationID` to `gen.CreateContentParams`. Ensure `errors` and `github.com/jackc/pgx/v5` are imported.

```go
	if input.OrganizationID.Valid {
		role, err := s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
			OrganizationID: input.OrganizationID,
			UserID:         input.AuthorID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.CreateContentRow{}, ErrOrgPostForbidden
			}
			return gen.CreateContentRow{}, fmt.Errorf("checking org membership: %w", err)
		}
		if !isOrgPostRole(role) {
			return gen.CreateContentRow{}, ErrOrgPostForbidden
		}
	}

	content, err := s.write.CreateContent(ctx, gen.CreateContentParams{
		TenantID:        tenantID,
		AuthorID:        input.AuthorID,
		OrganizationID:  input.OrganizationID,
		SpaceID:         input.SpaceID,
		// ...unchanged fields...
```

(Keep every other `gen.CreateContentParams` field exactly as-is; only the `OrganizationID` line is new.)

- [ ] **Step 4: Parse `organization_id` + map 403 in the create handler (`internal/content/handlers.go`)**

In `CreateContent` (handlers.go:278): add `OrganizationID *string json:"organization_id"` to the request `input` struct; parse it into `createInput.OrganizationID` (mirror the `SpaceID` block); map the sentinel to 403 where the service is called.

Add to the anonymous `input` struct:
```go
		SpaceID        *string `json:"space_id"`
		GeoSpaceID     *string `json:"geo_space_id"`
		OrganizationID *string `json:"organization_id"`
```

Add after the `SpaceID` parse block:
```go
	if input.OrganizationID != nil && *input.OrganizationID != "" {
		if err := createInput.OrganizationID.Scan(*input.OrganizationID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
			return
		}
	}
```

Find where `h.Service.CreateContent(...)` is called and its error handled; add the 403 mapping before the generic error branch:
```go
	if errors.Is(err, ErrOrgPostForbidden) {
		writeError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to post as this organization")
		return
	}
```
(Ensure `errors` is imported in `handlers.go`.)

- [ ] **Step 5: Write the create-permission integration test `internal/content/org_authorship_integration_test.go`**

```go
//go:build integration

package content

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/testdb"
)

func TestCreateContent_OrgAttribution_Permission(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)

	owner := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-owner"})
	editor := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-editor"})
	plain := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-plain"})
	outsider := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "org-outsider"})

	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: owner})
	testdb.SeedOrgMembership(t, database, orgID, owner, "owner")
	testdb.SeedOrgMembership(t, database, orgID, editor, "editor")
	testdb.SeedOrgMembership(t, database, orgID, plain, "member")

	newInput := func(author pgtype.UUID) CreateContentInput {
		return CreateContentInput{
			AuthorID:       author,
			OrganizationID: orgID,
			ContentType:    "discussion",
			Title:          "Hello from Acme",
			Body:           "body",
			BodyHtml:       "<p>body</p>",
			Status:         "published",
			Visibility:     "public",
		}
	}

	// editor may post as org
	row, err := svc.CreateContent(context.Background(), tenantID, newInput(editor))
	if err != nil {
		t.Fatalf("editor CreateContent: %v", err)
	}
	if !row.OrganizationID.Valid || row.OrganizationID != orgID {
		t.Fatalf("org post organization_id not recorded: %+v", row.OrganizationID)
	}
	if row.AuthorID != editor {
		t.Fatalf("author_id must be the human editor, got %+v", row.AuthorID)
	}

	// plain member → forbidden
	if _, err := svc.CreateContent(context.Background(), tenantID, newInput(plain)); !errors.Is(err, ErrOrgPostForbidden) {
		t.Fatalf("plain member: err=%v want ErrOrgPostForbidden", err)
	}
	// non-member → forbidden
	if _, err := svc.CreateContent(context.Background(), tenantID, newInput(outsider)); !errors.Is(err, ErrOrgPostForbidden) {
		t.Fatalf("outsider: err=%v want ErrOrgPostForbidden", err)
	}
}
```

- [ ] **Step 6: Run + build**

Run: `go build ./... && go test -tags integration ./internal/content/ -run TestCreateContent_OrgAttribution_Permission -v`
Expected: build exit 0; PASS (editor 200 w/ org_id + human author; member/outsider → ErrOrgPostForbidden).

- [ ] **Step 7: Commit**

```bash
git add internal/content/content.go internal/content/store.go internal/content/handlers.go \
        internal/content/org_authorship_integration_test.go
git commit -m "feat(content): post-as-org write path — membership-gated organization_id on create (Phase B2)"
```

---

### Task 5: Write-path — edit as organization (UpdateContent permission)

**Files:**
- Modify: `internal/content/store.go`, `internal/content/handlers.go`
- Test: `internal/content/org_authorship_integration_test.go` (add a test)

**Interfaces:**
- Produces: `content.UpdateContentInput.OrganizationID pgtype.UUID`; update handler accepts `"organization_id"` and returns 403 `org_post_forbidden` when the caller lacks `owner|admin|editor` for the target org.
- Consumes: Task 4's `ErrOrgPostForbidden`/`isOrgPostRole`, Task 3's `gen.UpdateContentParams.OrganizationID`.

- [ ] **Step 1: Add `OrganizationID` to `UpdateContentInput` (`internal/content/store.go`)**

Add to the struct (after `IsPinned`):
```go
	IsLocked       bool
	IsPinned       bool
	OrganizationID pgtype.UUID
```

- [ ] **Step 2: Enforce membership + pass through in `UpdateContent` (`internal/content/store.go`)**

In `UpdateContent`, after `existing` is fetched and before `s.write.UpdateContent(...)`, add the membership check; add `OrganizationID: input.OrganizationID` to `gen.UpdateContentParams`.

```go
	if input.OrganizationID.Valid {
		role, err := s.read.GetOrganizationMemberRole(ctx, gen.GetOrganizationMemberRoleParams{
			OrganizationID: input.OrganizationID,
			UserID:         editedBy,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.UpdateContentRow{}, ErrOrgPostForbidden
			}
			return gen.UpdateContentRow{}, fmt.Errorf("checking org membership on update: %w", err)
		}
		if !isOrgPostRole(role) {
			return gen.UpdateContentRow{}, ErrOrgPostForbidden
		}
	}

	content, err := s.write.UpdateContent(ctx, gen.UpdateContentParams{
		// ...unchanged fields...
		OrganizationID: input.OrganizationID,
		ID:             id,
		TenantID:       tenantID,
	})
```

- [ ] **Step 3: Parse `organization_id` + map 403 in the update handler (`internal/content/handlers.go`)**

In `UpdateContent` (handlers.go:494): add `OrganizationID *string json:"organization_id"` to the request `input`; parse into `updateInput.OrganizationID` (mirror the `GeoSpaceID` block); add the same 403 mapping where `h.Service.UpdateContent(...)`'s error is handled.

```go
	if input.OrganizationID != nil && *input.OrganizationID != "" {
		if err := updateInput.OrganizationID.Scan(*input.OrganizationID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_organization_id", "Invalid organization_id format")
			return
		}
	}
```
And before the generic error branch of the update call:
```go
	if errors.Is(err, ErrOrgPostForbidden) {
		writeError(w, http.StatusForbidden, "org_post_forbidden", "You are not authorized to edit as this organization")
		return
	}
```

- [ ] **Step 4: Add the update-permission test to `internal/content/org_authorship_integration_test.go`**

```go
func TestUpdateContent_OrgAttribution_Permission(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	svc := NewService(database)

	admin := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "up-admin"})
	member := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "up-member"})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "beta", Status: "active", OwnerUserID: admin})
	testdb.SeedOrgMembership(t, database, orgID, admin, "admin")
	testdb.SeedOrgMembership(t, database, orgID, member, "member")

	// seed a plain (non-org) post authored by admin
	postID := testdb.SeedContent(t, database, tenantID, admin, testdb.ContentOpts{Title: "draft", Status: "draft", Visibility: "public"})

	base := UpdateContentInput{
		Title: "attributed to org", Body: "b", BodyHtml: "<p>b</p>",
		Status: "published", Visibility: "public", OrganizationID: orgID,
	}

	// admin may attribute the post to the org
	if _, err := svc.UpdateContent(context.Background(), postID, tenantID, admin, base); err != nil {
		t.Fatalf("admin UpdateContent: %v", err)
	}
	// member may not
	if _, err := svc.UpdateContent(context.Background(), postID, tenantID, member, base); !errors.Is(err, ErrOrgPostForbidden) {
		t.Fatalf("member update: err=%v want ErrOrgPostForbidden", err)
	}
}
```

- [ ] **Step 5: Run + build**

Run: `go build ./... && go test -tags integration ./internal/content/ -run 'TestCreateContent_OrgAttribution_Permission|TestUpdateContent_OrgAttribution_Permission' -v`
Expected: build exit 0; both PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/content/store.go internal/content/handlers.go internal/content/org_authorship_integration_test.go
git commit -m "feat(content): edit-as-org write path — membership-gated organization_id on update (Phase B2)"
```

---

### Task 6: Dual-surface — user PostCard org-attribution badge

**Files:**
- Modify: `internal/db/queries/contents.sql` (ListPublishedContentByAuthor), `internal/content/profile_posts.go`, `internal/content/post_card_test.go`
- Regenerate: `internal/db/gen/contents.sql.go`

**Interfaces:**
- Produces: `content.OrgRef` (`{id, name, slug, logo_media_id}`); `PostCard.OrganizationID` + `PostCard.Organization *OrgRef` (omitempty); `toPostCards` fills the badge from the new LEFT JOIN columns; `gen.ListPublishedContentByAuthorRow` gains `OrganizationID`, `OrgName`, `OrgSlug`, `OrgLogoMediaID`.
- Consumes: Task 2's column.

- [ ] **Step 1: LEFT JOIN organizations in `ListPublishedContentByAuthor` (`internal/db/queries/contents.sql`)**

Replace the `ListPublishedContentByAuthor` query (lines ~322-339) with:

```sql
-- name: ListPublishedContentByAuthor :many
-- Public profile Posts tab (GET /users/{id}/content): a user's PUBLISHED PUBLIC
-- content, newest first, paginated. author_id is the user id (FK users.id).
-- LEFT JOINs organizations so dual-surface posts carry the "on behalf of" badge.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id AND o.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0. Verify: `grep -n 'OrgName\|OrgSlug\|OrgLogoMediaID' internal/db/gen/contents.sql.go` shows the fields on `ListPublishedContentByAuthorRow`.

- [ ] **Step 3: Add the failing org-badge mapper test to `internal/content/post_card_test.go`**

This file already exists (it holds `TestToPostCards_LeanAndCorrect`) and already imports `encoding/json`, `strings`, `testing`, `time`, `pgtype`, and `gen`. The package-level `mustUUID(t, s)` helper lives in `content_test.go` — reuse it (do NOT add another UUID helper; a duplicate func in the same package won't compile). Append this function:

```go
func TestToPostCards_OrgBadge(t *testing.T) {
	orgName := "Acme"
	orgSlug := "acme"
	rows := []gen.ListPublishedContentByAuthorRow{
		{ // org-attributed → badge present
			ID: mustUUID(t, "11111111-1111-1111-1111-111111111111"), AuthorID: mustUUID(t, "22222222-2222-2222-2222-222222222222"),
			OrganizationID: mustUUID(t, "33333333-3333-3333-3333-333333333333"), Title: "org post", Slug: "op",
			ContentType: "discussion", OrgName: &orgName, OrgSlug: &orgSlug,
		},
		{ // personal → no badge
			ID: mustUUID(t, "44444444-4444-4444-4444-444444444444"), AuthorID: mustUUID(t, "22222222-2222-2222-2222-222222222222"),
			Title: "personal", Slug: "p", ContentType: "discussion",
		},
	}

	cards := toPostCards(rows)
	if cards[0].Organization == nil || cards[0].Organization.Name != "Acme" {
		t.Fatalf("org post missing badge: %+v", cards[0].Organization)
	}
	if cards[1].Organization != nil {
		t.Fatalf("personal post should have nil Organization: %+v", cards[1].Organization)
	}

	blob, _ := json.Marshal(cards[1])
	if strings.Contains(string(blob), "\"organization\":") && !strings.Contains(string(blob), "\"organization\":null") {
		// omitempty: personal post should not carry a populated organization object
		t.Errorf("personal card leaks org object: %s", blob)
	}
}
```

- [ ] **Step 4: Run to verify it fails**

Run: `go test ./internal/content/ -run TestToPostCards_OrgBadge -v`
Expected: FAIL — `cards[0].Organization undefined` (field not yet on PostCard).

- [ ] **Step 5: Add `OrgRef` + org fields to `PostCard` and fill them in `toPostCards` (`internal/content/profile_posts.go`)**

Add the `OrgRef` type and two fields on `PostCard` (after `AuthorID`), and update `toPostCards` to consume the new row columns:

```go
// OrgRef is the lean org reference embedded in an org-attributed PostCard so
// the "on behalf of <Org>" badge renders. Nil for personal posts.
type OrgRef struct {
	ID          pgtype.UUID `json:"id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	LogoMediaID pgtype.UUID `json:"logo_media_id"`
}
```

Add to `PostCard` (after `AuthorID`):
```go
	AuthorID       pgtype.UUID `json:"author_id"`
	OrganizationID pgtype.UUID `json:"organization_id"`
	Organization   *OrgRef     `json:"organization,omitempty"`
```

In `toPostCards`, inside the loop, after building the base card, add:
```go
		if c.OrganizationID.Valid {
			ref := &OrgRef{ID: c.OrganizationID, LogoMediaID: c.OrgLogoMediaID}
			if c.OrgName != nil {
				ref.Name = *c.OrgName
			}
			if c.OrgSlug != nil {
				ref.Slug = *c.OrgSlug
			}
			cards[i].OrganizationID = c.OrganizationID
			cards[i].Organization = ref
		}
```

- [ ] **Step 6: Run to verify it passes**

Run: `go test ./internal/content/ -run TestToPostCards_OrgBadge -v`
Expected: PASS.

- [ ] **Step 7: Build + commit**

Run: `go build ./...` → exit 0.
```bash
git add internal/db/queries/contents.sql internal/db/gen/contents.sql.go internal/content/profile_posts.go internal/content/post_card_test.go
git commit -m "feat(content): dual-surface org badge on user PostCard (Phase B2)"
```

---

### Task 7: Org Posts tab — `GET /organizations/{id}/content`

**Files:**
- Modify: `internal/db/queries/contents.sql`, `internal/content/profile_posts.go`, `internal/server/server.go`
- Regenerate: `internal/db/gen/contents.sql.go`
- Test: `internal/content/org_posts_integration_test.go` (create)

**Interfaces:**
- Produces: query `ListPublishedContentByOrganization`; `Service.ListPublishedByOrganization(ctx, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByOrganizationRow, error)`; handler `Handlers.ListByOrganization`; mapper `toOrgPostCards`.
- Consumes: B1 `requireActiveOrgByID(orgService)` (server pkg), Task 6 `PostCard`/`OrgRef`.

- [ ] **Step 1: Add `ListPublishedContentByOrganization` to `internal/db/queries/contents.sql`**

Append after `ListPublishedContentByAuthor`:

```sql
-- name: ListPublishedContentByOrganization :many
-- Org profile Posts tab (GET /organizations/{id}/content): the org's PUBLISHED
-- PUBLIC content, newest first, paginated. Keyed by organization_id (ADR-062).
-- author_id (the human) is projected for accountability; org badge fields via join.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id AND o.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.organization_id = @organization_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate`
Expected: exit 0. Verify `gen.ListPublishedContentByOrganizationRow` exists with `OrgName`/`OrgSlug`/`OrgLogoMediaID`.

- [ ] **Step 3: Add service + handler + mapper to `internal/content/profile_posts.go`**

```go
// ListPublishedByOrganization returns an org's published+public content, newest
// first, paginated. Powers the org profile Posts tab (GET /organizations/{id}/content).
func (s *Service) ListPublishedByOrganization(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.ListPublishedContentByOrganizationRow, error) {
	rows, err := s.read.ListPublishedContentByOrganization(ctx, gen.ListPublishedContentByOrganizationParams{
		TenantID:       tenantID,
		OrganizationID: orgID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing published content by organization: %w", err)
	}
	return rows, nil
}

// ListByOrganization handles GET /organizations/{id}/content — the org profile
// Posts tab. Non-active orgs are already 404'd by the requireActiveOrgByID gate.
func (h *Handlers) ListByOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
		return
	}

	limit := parsePageSize(r)
	var offset int32
	if o := r.URL.Query().Get("page_offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n > 0 {
			offset = int32(n)
		}
	}

	items, err := h.Service.ListPublishedByOrganization(r.Context(), tenantID, orgID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", "Failed to list content")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toOrgPostCards(items)})
}

func toOrgPostCards(items []gen.ListPublishedContentByOrganizationRow) []PostCard {
	cards := make([]PostCard, len(items))
	for i := range items {
		c := &items[i]
		cards[i] = PostCard{
			ID:              c.ID,
			AuthorID:        c.AuthorID,
			SpaceID:         c.SpaceID,
			ContentType:     c.ContentType,
			Title:           c.Title,
			Slug:            c.Slug,
			BodyHtml:        c.BodyHtml,
			HeaderImageID:   c.HeaderImageID,
			FeaturedImageID: c.FeaturedImageID,
			ReadTimeMinutes: c.ReadTimeMinutes,
			ViewCount:       c.ViewCount,
			CommentCount:    c.CommentCount,
			ShareCount:      c.ShareCount,
			IsPinned:        c.IsPinned,
			PublishedAt:     c.PublishedAt,
			CreatedAt:       c.CreatedAt,
		}
		if c.OrganizationID.Valid {
			ref := &OrgRef{ID: c.OrganizationID, LogoMediaID: c.OrgLogoMediaID}
			if c.OrgName != nil {
				ref.Name = *c.OrgName
			}
			if c.OrgSlug != nil {
				ref.Slug = *c.OrgSlug
			}
			cards[i].OrganizationID = c.OrganizationID
			cards[i].Organization = ref
		}
	}
	return cards
}
```

- [ ] **Step 4: Wire the route in `internal/server/server.go`**

Add immediately after the public members route (`/organizations/{id}/members`, ~line 1557):

```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/content", contentHandlers.ListByOrganization)
```
(`contentHandlers` is declared at server.go:278, `orgService` at :377 — both in scope. If a nested block puts them out of scope here, place this route in the public `/users/{id}/*` tabs block near ~1950 instead, where `contentHandlers`/`mediaHandlers`/`spaceHandlers` are already wired together; the org gate only needs `orgService`.)

- [ ] **Step 5: Write the org-Posts integration test `internal/content/org_posts_integration_test.go`**

```go
//go:build integration

package content

import (
	"context"
	"net/http"
	"net/http/httptest"
	"encoding/json"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

func TestListByOrganization_OnlyOrgPublishedPublic(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "auth"})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: author})
	otherOrg := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "other", Status: "active"})

	// org published-public (included) ; org draft (excluded) ; other org (excluded) ; personal (excluded)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "orgpub", Status: "published", Visibility: "public", OrganizationID: orgID})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "orgdraft", Status: "draft", Visibility: "public", OrganizationID: orgID})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "otherorg", Status: "published", Visibility: "public", OrganizationID: otherOrg})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "personal", Status: "published", Visibility: "public"})

	h := NewHandlers(NewService(database))
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/content", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", orgID.String())
	rec := httptest.NewRecorder()
	h.ListByOrganization(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	var env struct {
		Data []struct {
			Title        string `json:"title"`
			Organization *struct {
				Slug string `json:"slug"`
			} `json:"organization"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].Title != "orgpub" {
		t.Fatalf("want 1 org published-public post 'orgpub', got %+v", env.Data)
	}
	if env.Data[0].Organization == nil || env.Data[0].Organization.Slug != "acme" {
		t.Fatalf("org badge missing/wrong: %+v", env.Data[0].Organization)
	}
}
```
(`NewHandlers(*Service)` — confirm the content `NewHandlers` signature at content.go; it takes the service. If it needs more args, mirror the existing content handler construction.)

- [ ] **Step 6: Run + build**

Run: `go build ./... && go test -tags integration ./internal/content/ -run TestListByOrganization_OnlyOrgPublishedPublic -v`
Expected: build exit 0; PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/db/queries/contents.sql internal/db/gen/contents.sql.go \
        internal/content/profile_posts.go internal/content/org_posts_integration_test.go \
        internal/server/server.go
git commit -m "feat(org): public Posts tab — GET /organizations/{id}/content, gated + lean (Phase B2)"
```

---

### Task 8: Org Media tab — `GET /organizations/{id}/media`

**Files:**
- Modify: `internal/db/queries/media.sql`, `internal/media/store.go`, `internal/media/service.go`, `internal/media/handlers.go`, `internal/server/server.go`
- Regenerate: `internal/db/gen/media.sql.go`
- Test: `internal/media/org_media_integration_test.go` (create)

**Interfaces:**
- Produces: queries `ListMediaFromOrgPublicPosts` + `CountMediaFromOrgPublicPosts`; `Store.ListMediaFromOrgPublicPosts`/`CountMediaFromOrgPublicPosts`; `Service.ListByOrganization(ctx, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Medium, int32, error)`; handler `Handlers.ListByOrganization`.
- Consumes: B1 `requireActiveOrgByID(orgService)`, existing `toMediaResponses`.

- [ ] **Step 1: Add the org media queries to `internal/db/queries/media.sql`**

Append after `CountMediaFromPublicPostsByUser`:

```sql
-- name: ListMediaFromOrgPublicPosts :many
-- Org profile Media tab: media referenced (featured/header image) by the ORG's
-- PUBLISHED PUBLIC posts. Excludes unconfirmed uploads and group-restricted media.
SELECT m.* FROM media m
WHERE m.tenant_id = @tenant_id
  AND m.deleted_at IS NULL
  AND m.confirmed = true
  AND m.user_group_id IS NULL
  AND EXISTS (
    SELECT 1 FROM contents c
    WHERE c.tenant_id = @tenant_id
      AND c.organization_id = @organization_id
      AND c.status = 'published'
      AND c.visibility = 'public'
      AND (c.featured_image_id = m.id OR c.header_image_id = m.id)
  )
ORDER BY m.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMediaFromOrgPublicPosts :one
SELECT COUNT(*)::int AS count FROM media m
WHERE m.tenant_id = @tenant_id
  AND m.deleted_at IS NULL
  AND m.confirmed = true
  AND m.user_group_id IS NULL
  AND EXISTS (
    SELECT 1 FROM contents c
    WHERE c.tenant_id = @tenant_id
      AND c.organization_id = @organization_id
      AND c.status = 'published'
      AND c.visibility = 'public'
      AND (c.featured_image_id = m.id OR c.header_image_id = m.id)
  );
```

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate` → exit 0. Verify `gen.ListMediaFromOrgPublicPostsParams`/`gen.CountMediaFromOrgPublicPostsParams` exist.

- [ ] **Step 3: Add store methods to `internal/media/store.go`**

```go
// ListMediaFromOrgPublicPosts returns media used as featured/header images by
// the org's published-public posts, paginated. Powers the org Media tab.
func (s *Store) ListMediaFromOrgPublicPosts(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Medium, error) {
	items, err := s.read.ListMediaFromOrgPublicPosts(ctx, gen.ListMediaFromOrgPublicPostsParams{
		TenantID:       tenantID,
		OrganizationID: orgID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing media from org public posts: %w", err)
	}
	return items, nil
}

func (s *Store) CountMediaFromOrgPublicPosts(ctx context.Context, tenantID, orgID pgtype.UUID) (int32, error) {
	count, err := s.read.CountMediaFromOrgPublicPosts(ctx, gen.CountMediaFromOrgPublicPostsParams{
		TenantID:       tenantID,
		OrganizationID: orgID,
	})
	if err != nil {
		return 0, fmt.Errorf("counting media from org public posts: %w", err)
	}
	return count, nil
}
```
(Mirror the exact param field names from the ListMediaFromPublicPostsByUser store method — if the generated param field for the limit is `PageLimit`/`PageOffset` vs `Limit`/`Offset`, match what sqlc produced; check `internal/media/store.go:186`.)

- [ ] **Step 4: Add the service method to `internal/media/service.go`**

```go
// ListByOrganization returns media from the org's public posts, with a total
// count. Powers the org profile Media tab.
func (s *Service) ListByOrganization(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Medium, int32, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	items, err := s.store.ListMediaFromOrgPublicPosts(ctx, tenantID, orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing media from org public posts: %w", err)
	}
	total, err := s.store.CountMediaFromOrgPublicPosts(ctx, tenantID, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("counting media from org public posts: %w", err)
	}
	return items, total, nil
}
```

- [ ] **Step 5: Add the handler to `internal/media/handlers.go`**

```go
// ListByOrganization handles GET /organizations/{id}/media — the org profile
// Media tab. Non-active orgs are already 404'd by the requireActiveOrgByID gate.
func (h *Handlers) ListByOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)

	items, total, err := h.service.ListByOrganization(r.Context(), tenantID, orgID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": toMediaResponses(items, h.service),
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset},
	})
}
```

- [ ] **Step 6: Wire the route in `internal/server/server.go`**

Add after the org content route (Task 7):
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/media", mediaHandlers.ListByOrganization)
```

- [ ] **Step 7: Write the org-Media integration test `internal/media/org_media_integration_test.go`**

```go
//go:build integration

package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

func TestListByOrganization_OrgPublicPostMedia(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "mauth"})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: author})

	mediaID := testdb.SeedMedia(t, database, tenantID, author) // confirmed, universal
	// org published-public post featuring the media → media included
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Title: "orgpub", Status: "published", Visibility: "public",
		OrganizationID: orgID, FeaturedImageID: mediaID,
	})
	// org DRAFT featuring media → excluded (not published)
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{
		Title: "orgdraft", Status: "draft", Visibility: "public",
		OrganizationID: orgID, FeaturedImageID: testdb.SeedMedia(t, database, tenantID, author),
	})

	h := NewHandlers(NewService(database))
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/media", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", orgID.String())
	rec := httptest.NewRecorder()
	h.ListByOrganization(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	var env struct {
		Data []struct{ ID string `json:"id"` } `json:"data"`
		Meta struct{ Total int `json:"total"` } `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Meta.Total != 1 || len(env.Data) != 1 || env.Data[0].ID != mediaID.String() {
		t.Fatalf("want exactly the org public post media, got total=%d data=%+v", env.Meta.Total, env.Data)
	}
}
```
(Confirm `testdb.SeedMedia`'s signature — the harness has it per ADR-061; match its actual arg list. If it returns `(mediaID)`, use as shown; if it needs `MediaOpts`, pass a zero value.)

- [ ] **Step 8: Run + build**

Run: `go build ./... && go test -tags integration ./internal/media/ -run TestListByOrganization_OrgPublicPostMedia -v`
Expected: build exit 0; PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/db/queries/media.sql internal/db/gen/media.sql.go \
        internal/media/store.go internal/media/service.go internal/media/handlers.go \
        internal/media/org_media_integration_test.go internal/server/server.go
git commit -m "feat(org): public Media tab — GET /organizations/{id}/media, gated + lean (Phase B2)"
```

---

### Task 9: Org Spaces tab — `GET /organizations/{id}/spaces`

**Files:**
- Modify: `internal/db/queries/spaces.sql`, `internal/spaces/store.go`, `internal/spaces/handlers.go`, `internal/server/server.go`
- Regenerate: `internal/db/gen/spaces.sql.go`
- Test: `internal/spaces/org_spaces_integration_test.go` (create)

**Interfaces:**
- Produces: query `ListSpacesByOrgPosts`; `Service.ListPublicByOrgPosts(ctx, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Space, error)`; handler `Handlers.ListByOrganization`.
- Consumes: B1 `requireActiveOrgByID(orgService)`, existing `toSpaceCards`.

- [ ] **Step 1: Add `ListSpacesByOrgPosts` to `internal/db/queries/spaces.sql`**

Append after `ListPublicSpacesByMemberUser`:

```sql
-- name: ListSpacesByOrgPosts :many
-- Org profile Spaces tab: DISTINCT public+active spaces the org has published
-- public content in ("where the org is active"). No membership exposure.
SELECT DISTINCT s.id, s.tenant_id, s.parent_id, s.space_type_id, s.name, s.slug, s.description,
       s.wiki_content, s.logo_media_id, s.header_image_id, s.geo_code, s.level,
       s.member_count, s.post_count, s.follower_count, s.visibility, s.posting_perm,
       s.replying_perm, s.commenting_perm, s.joining_perm, s.status, s.created_at,
       s.updated_at
FROM spaces s
JOIN contents c ON c.space_id = s.id AND c.tenant_id = s.tenant_id
WHERE s.tenant_id = @tenant_id
  AND c.organization_id = @organization_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND s.visibility = 'public'
  AND s.status = 'active'
ORDER BY s.created_at DESC, s.id DESC
LIMIT @page_limit OFFSET @page_offset;
```
(ORDER BY columns `s.created_at`, `s.id` are in the SELECT list — required with DISTINCT.)

- [ ] **Step 2: Regenerate sqlc**

Run: `sqlc generate` → exit 0. Confirm `ListSpacesByOrgPosts` returns `[]gen.Space` (projection matches the model, same as `ListPublicSpacesByMemberUser`). If sqlc emits a distinct `ListSpacesByOrgPostsRow`, adjust the store return type in Step 3 accordingly and add a `toSpaceCards`-compatible mapper.

- [ ] **Step 3: Add the service method to `internal/spaces/store.go`**

(Spaces `Service` methods live in `store.go` in this package.)
```go
// ListPublicByOrgPosts returns the DISTINCT public+active spaces the org has
// published public content in, paginated. Powers the org profile Spaces tab.
func (s *Service) ListPublicByOrgPosts(ctx context.Context, tenantID, orgID pgtype.UUID, limit, offset int32) ([]gen.Space, error) {
	spaces, err := s.read.ListSpacesByOrgPosts(ctx, gen.ListSpacesByOrgPostsParams{
		OrganizationID: orgID,
		TenantID:       tenantID,
		PageLimit:      limit,
		PageOffset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing public spaces by org posts: %w", err)
	}
	return spaces, nil
}
```

- [ ] **Step 4: Add the handler to `internal/spaces/handlers.go`**

```go
// ListByOrganization handles GET /organizations/{id}/spaces — the org profile
// Spaces tab. Non-active orgs are already 404'd by the requireActiveOrgByID gate.
func (h *Handlers) ListByOrganization(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var orgID pgtype.UUID
	if err := orgID.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_org_id", "Invalid organization ID format")
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	spaces, err := h.Service.ListPublicByOrgPosts(r.Context(), tenantID, orgID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": toSpaceCards(spaces)})
}
```
(Uses `parsePageSizeOffset` — the spaces package's offset helper, per `ListByMemberUser`. Match the exact helper name in this package.)

- [ ] **Step 5: Wire the route in `internal/server/server.go`**

Add after the org media route (Task 8):
```go
			r.With(auth.OptionalAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Get("/organizations/{id}/spaces", spaceHandlers.ListByOrganization)
```

- [ ] **Step 6: Write the org-Spaces integration test `internal/spaces/org_spaces_integration_test.go`**

```go
//go:build integration

package spaces

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/testdb"
)

func TestListByOrganization_DistinctPublicSpaces(t *testing.T) {
	t.Parallel()

	database := testdb.Connect(t)
	tenantID := testdb.NewTenant(t, database)
	author := testdb.SeedUser(t, database, tenantID, testdb.UserOpts{Username: "sauth"})
	orgID := testdb.SeedOrganization(t, database, tenantID, testdb.OrgOpts{Slug: "acme", Status: "active", OwnerUserID: author})

	// SeedSpaceMembership creates a space (+ space_type + a membership row that
	// ListSpacesByOrgPosts ignores) and returns the space id. Slug is auto-
	// generated, so assert on the returned id, not a fixed slug.
	publicSpace := testdb.SeedSpaceMembership(t, database, tenantID, author, testdb.SpaceOpts{Visibility: "public", Status: "active"})
	privateSpace := testdb.SeedSpaceMembership(t, database, tenantID, author, testdb.SpaceOpts{Visibility: "private", Status: "active"})

	// two org public posts in the same public space → DISTINCT collapses to 1
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "a", Status: "published", Visibility: "public", OrganizationID: orgID, SpaceID: publicSpace})
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "b", Status: "published", Visibility: "public", OrganizationID: orgID, SpaceID: publicSpace})
	// org post in a private space → excluded
	testdb.SeedContent(t, database, tenantID, author, testdb.ContentOpts{Title: "c", Status: "published", Visibility: "public", OrganizationID: orgID, SpaceID: privateSpace})

	// spaces.NewHandlers(service, unread, contentService) — ListByOrganization
	// uses only h.Service + h.requireTenant, so the unread/content deps are nil-safe here.
	h := NewHandlers(NewService(database), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/spaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), tenant.TenantContextKey, &tenant.Tenant{ID: tenantID}))
	req = testdb.WithChiURLParam(req, "id", orgID.String())
	rec := httptest.NewRecorder()
	h.ListByOrganization(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", rec.Code, rec.Body)
	}
	var env struct {
		Data []struct{ ID string `json:"id"` } `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].ID != publicSpace.String() {
		t.Fatalf("want 1 distinct public space (%s), got %+v", publicSpace.String(), env.Data)
	}
}
```
(Uses the existing `SeedSpaceMembership` — no new seeder needed. `_ = privateSpace` is consumed by the third SeedContent call.)

- [ ] **Step 7: Run + build**

Run: `go build ./... && go test -tags integration ./internal/spaces/ -run TestListByOrganization_DistinctPublicSpaces -v`
Expected: build exit 0; PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/db/queries/spaces.sql internal/db/gen/spaces.sql.go \
        internal/spaces/store.go internal/spaces/handlers.go \
        internal/spaces/org_spaces_integration_test.go internal/server/server.go
git commit -m "feat(org): public Spaces tab — GET /organizations/{id}/spaces, distinct+gated (Phase B2)"
```

---

### Task 10: Catalog + Section 07 reconciliation (submodule)

**Files:**
- Modify (submodule): `.jeos/conventions/api-routes-catalog.md`

- [ ] **Step 1: Annotate the org content-tab routes in `.jeos/conventions/api-routes-catalog.md`**

Under the Organizations (Section 07) block, add the three new routes with `[public]` + gate notes:
```
GET    /organizations/{id}/content         [public] org's published-public posts (Posts tab), gated by requireActiveOrgByID
GET    /organizations/{id}/media           [public] media from org's public posts (Media tab), gated
GET    /organizations/{id}/spaces          [public] distinct public spaces the org posts in (Spaces tab), gated
```
Also add a note near `POST /content` / `PATCH /content/{id}`: "`organization_id` (optional) attributes the post to an org; requires caller `owner|admin|editor` of that org (ADR-062), else 403 `org_post_forbidden`."

- [ ] **Step 2: Commit (submodule)**

```bash
git -C .jeos add conventions/api-routes-catalog.md
git -C .jeos commit -m "docs(api-catalog): org content tabs + post-as-org attribution (Phase B2, ADR-062)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Final verification (after all tasks)

- [ ] `go build ./...` → exit 0
- [ ] `go vet -tags integration ./internal/content/ ./internal/media/ ./internal/spaces/ ./internal/organizations/ ./internal/server/ ./internal/testdb/` → exit 0
- [ ] `go test -tags integration -p 1 ./internal/content/ ./internal/media/ ./internal/spaces/ ./internal/testdb/` → all PASS (`-p 1` avoids cross-package contention on the shared `japa_test` DB).
- [ ] Confirm the three org content-tab routes are wired with `OptionalAuth` + `requireActiveOrgByID` (grep `internal/server/server.go` for `/organizations/{id}/content|media|spaces`).
- [ ] Confirm `contents.organization_id` migration applied (`\d contents`).
- [ ] Pre-existing `TestNoDuplicateRoutes`/`TestNoOverlappingParamRoutes` in `internal/server` remain red from a prior cron bundle — NOT introduced by this plan; confirm no NEW collision (the new `/organizations/{id}/{content,media,spaces}` paths are unique).

## Notes for the executor (work-on-main hazards)

- A cron auto-committer sweeps the whole tree and may bundle your uncommitted files into its own commit. Commit each task immediately. If `git status` shows a task's files already clean, verify they are in HEAD's tree and proceed.
- `sqlc generate` is global: after each regen, `git add` ONLY the specific gen file(s) the task lists (`contents.sql.go` / `media.sql.go` / `spaces.sql.go`) — never the whole gen dir.
- Do NOT `git add .`. Stage the exact paths each task lists.
- Submodule (`.jeos`) commits (ADR, catalog) are made inside the submodule; the outer gitlink is left modified for the cron bundler. ⚠ The submodule commits must reach the `.jeos` remote before the outer `main` gitlink advances to them, or the pointer is broken — confirm the project's push flow handles submodules, or push `.jeos` explicitly at finish.

## Deferred to a follow-on plan (B2 cross-cutting)

These are called out in the design spec but are independent subsystems, deliberately out of scope here to keep this plan shippable:
1. **Feed byline:** LEFT JOIN `organizations` across the feed content queries (`GetExploreFeed`, `GetPrimaryFeed`, `GetSpaceFeed`, `GetProfileFeed`, `GetTopFeed`, `GetRisingFeed`, `GetForYouFeed`), add org fields to `FeedItem`/`PrimaryFeedItem`, populate in the feed mappers (`service.go:301/329/357`, `foryou.go:111`).
2. **Search enrichment:** add `organization_id` (filterable) + org name to the Meilisearch `contentDocument` (`internal/search/indexer.go`), register the filterable attribute in the index settings, and resolve org name via `GetOrganizationByID` when `organization_id` is set.
```
