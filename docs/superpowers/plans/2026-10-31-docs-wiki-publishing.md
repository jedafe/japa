# Docs Wiki-Style Publishing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give doc pages wiki-style publishing: a per-page feature image (hero + cards + og:image) and a `doc_page_revisions` audit trail with view/restore/per-revision delete/clear-all, plus edit-summary capture — all inside `apps/docs`, editors (`docs.manage`) only.

**Architecture:** Snapshot-on-write versioning — a revision row is inserted in the same transaction as every content-changing page write; restore copies a revision forward as a new revision. Feature image is `doc_pages.feature_image_id → media.id`, resolved to `feature_image_url` via `media.MediaURL` like blog's `cover_image_url`.

**Tech Stack:** Go 1.x, chi, pgx + sqlc (`internal/db/gen`), Valkey cache, SvelteKit 2 + Svelte 5 runes, `@japaconnect/ui` RichTextEditor/MediaPickerModal, bun.

## Global Constraints

- Rule 2: never edit `internal/db/gen/*` — edit `internal/db/queries/docs.sql`, run `make sqlc-gen`.
- Rule 16: no hardcoded data in frontend — everything via the Go API.
- Rule 17: no AI attribution anywhere (comments, commits, docs).
- Rule 18: `bun`/`bunx` only, never npm/npx.
- All new routes are `docs.manage`-gated and must call `checkPageAudienceGate` for the page's audience.
- Write-path semantics (from spec): a revision is written on create, and on update/restore **only when `title`, `content`, or `feature_image_id` changed**. The **latest revision cannot be deleted** (`409`). Clear-all deletes every revision for the page.
- Reuse `docContentEqual` (service.go) for semantic TipTap comparison — re-sent unchanged docs must NOT version.
- Spec: `docs/superpowers/specs/2026-10-31-docs-wiki-publishing-design.md`.
- JEOS §44 must be amended BEFORE backend code (`.jeos/sections/44-documentation-system.md`, submodule repo — commit inside `.jeos`, then bump the pointer in the parent repo commit).

---

### Task 1: JEOS amendment + migration + sqlc queries + regen

**Files:**
- Modify: `.jeos/sections/44-documentation-system.md` (append amendment)
- Modify: `.jeos/conventions/api-routes-catalog.md` (register new routes)
- Create: `migrations/000987_doc_page_revisions_feature_image.up.sql`
- Create: `migrations/000987_doc_page_revisions_feature_image.down.sql`
- Modify: `internal/db/queries/docs.sql`
- Regenerate: `internal/db/gen/*` via `make sqlc-gen`

- [ ] **Step 1: JEOS §44 amendment (in the `.jeos` submodule)**

Append to `sections/44-documentation-system.md` (after amendment 4):

```markdown
### Feature images & revision history (2026-10-31 amendment 5)

- `doc_pages.feature_image_id uuid REFERENCES media(id)` — per-page hero
  image, media-library-backed like blog `cover_image_id`. Exposed as
  `feature_image_id` + computed `feature_image_url` (imgproxy URL via
  `media.MediaURL`) on `DocPage`, `DocPageSummary`, and `DocNavItem`.
  Create/PATCH accept `feature_image_id` (uuid, `""` → NULL) and an
  optional `summary` string recorded on the revision.
- `doc_page_revisions` snapshots every content-changing write:
  `(doc_page_id, version seq, title, content, feature_image_id, summary,
  created_by, created_at)`, `UNIQUE (doc_page_id, version)`, written in the
  same transaction as the page write. Organizational-only changes (slug,
  category, subcategory, parent, position, audience) never version.
- Editor endpoints (docs.manage, audience-gated):
  `GET /admin/docs/pages/{id}/revisions?page=&limit=` (paginated, excerpt),
  `GET /admin/docs/revisions/{revId}` (full content),
  `POST /admin/docs/pages/{id}/revisions/{revId}/restore` (copies
  title+content+feature_image forward as a new revision; approved →
  in_review per amendment 3), `DELETE /admin/docs/revisions/{revId}`
  (latest → 409 revision_is_latest), `DELETE /admin/docs/pages/{id}/revisions`
  (clear all).
- Docs frontend: hero image + og:image on the page, thumbnails on
  audience/category cards, "History" link next to "Edit this page",
  `/docs/{audience}/{slug}/history` editor page, feature-image picker +
  edit-summary field in both authoring forms.
```

Add to `.jeos/conventions/api-routes-catalog.md` under the docs admin section:

```
GET    /api/v1/admin/docs/pages/{id}/revisions                    docs.manage — revision list (paginated)
GET    /api/v1/admin/docs/revisions/{revId}                       docs.manage — single revision w/ content
POST   /api/v1/admin/docs/pages/{id}/revisions/{revId}/restore    docs.manage — restore revision
DELETE /api/v1/admin/docs/revisions/{revId}                       docs.manage — delete one revision (not latest)
DELETE /api/v1/admin/docs/pages/{id}/revisions                    docs.manage — clear all revisions
```

Commit inside `.jeos`: `cd .jeos && git add -A && git commit -m "§44 amendment 5: doc feature images + revision history"`.

- [ ] **Step 2: Migration**

`migrations/000987_doc_page_revisions_feature_image.up.sql`:

```sql
-- §44 amendment 5: per-page feature image (media library) + revision
-- snapshots written on every content-changing page write.
ALTER TABLE doc_pages
    ADD COLUMN feature_image_id uuid REFERENCES media(id);

CREATE TABLE doc_page_revisions (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    doc_page_id      uuid NOT NULL REFERENCES doc_pages(id) ON DELETE CASCADE,
    version          int NOT NULL,
    title            text NOT NULL,
    content          jsonb NOT NULL,
    feature_image_id uuid REFERENCES media(id),
    summary          text,
    created_by       uuid REFERENCES users(id),
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT doc_page_revisions_page_version_key UNIQUE (doc_page_id, version)
);
CREATE INDEX idx_doc_page_revisions_page
    ON doc_page_revisions (doc_page_id, version DESC);
```

`migrations/000987_doc_page_revisions_feature_image.down.sql`:

```sql
DROP TABLE IF EXISTS doc_page_revisions;
ALTER TABLE doc_pages DROP COLUMN IF EXISTS feature_image_id;
```

- [ ] **Step 3: sqlc queries in `internal/db/queries/docs.sql`**

3a. In every query that feeds `DocPage`/`DocPageSummary`/`DocNavItem`, add the
image join. Change `FROM doc_pages` to `FROM doc_pages p LEFT JOIN media m ON m.id = p.feature_image_id`
and prefix page columns with `p.` in: `GetDocPageBySlug`,
`GetDocPageBySlugAnyStatus`, `GetDocPageByID`, `ListDocPagesByAudience`,
`ListAllDocPagesByAudience`, `ListAllDocPages`, `ListPublishedDocPages`.
Add to each SELECT list: `p.feature_image_id, m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path`.
Example for `GetDocPageBySlug`:

```sql
-- name: GetDocPageBySlug :one
SELECT p.id, p.title, p.slug, p.audience, p.content, p.ai_generated, p.review_status,
 p.last_reviewed_by, p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
 p.helpful_yes, p.helpful_no, p.feature_image_id,
 m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
 p.created_at, p.updated_at
FROM doc_pages p LEFT JOIN media m ON m.id = p.feature_image_id
WHERE p.audience = @audience AND p.slug = @slug AND p.review_status = 'published';
```

3b. `CreateDocPage` — add `feature_image_id` to the column list and params:
`feature_image_id` column + `sqlc.narg(feature_image_id)::uuid` value.

3c. `UpdateDocPage` — add `feature_image_id = @feature_image_id` to the SET
list (the service always passes the effective value — the row's stored image
when the request omits the field).

3d. Append the revision queries:

```sql
-- name: InsertDocPageRevision :one
-- §44 amendment 5: snapshot of title/content/feature_image after a
-- content-changing write; version is the per-page sequence (MAX+1) computed
-- in the same statement so concurrent writers can't pick the same number.
INSERT INTO doc_page_revisions (doc_page_id, version, title, content, feature_image_id, summary, created_by)
SELECT @doc_page_id,
       COALESCE(MAX(version), 0) + 1,
       @title, @content, sqlc.narg(feature_image_id)::uuid, sqlc.narg(summary)::text, sqlc.narg(created_by)::uuid
FROM doc_page_revisions
WHERE doc_page_id = @doc_page_id
RETURNING id, version;

-- name: ListDocPageRevisions :many
-- content is selected so the service derives the plain-text excerpt — same
-- convention as ListDocPagesByAudience.
SELECT id, doc_page_id, version, title, content, feature_image_id, summary, created_by, created_at
FROM doc_page_revisions
WHERE doc_page_id = @doc_page_id
ORDER BY version DESC
LIMIT NULLIF(@result_limit::int, 0) OFFSET COALESCE(@result_offset::int, 0);

-- name: CountDocPageRevisions :one
SELECT COUNT(*) FROM doc_page_revisions WHERE doc_page_id = @doc_page_id;

-- name: GetDocPageRevision :one
SELECT id, doc_page_id, version, title, content, feature_image_id, summary, created_by, created_at
FROM doc_page_revisions
WHERE id = @id;

-- name: GetDocPageMaxRevisionVersion :one
SELECT COALESCE(MAX(version), 0) FROM doc_page_revisions WHERE doc_page_id = @doc_page_id;

-- name: DeleteDocPageRevision :execrows
DELETE FROM doc_page_revisions WHERE id = @id;

-- name: DeleteDocPageRevisions :exec
DELETE FROM doc_page_revisions WHERE doc_page_id = @doc_page_id;
```

- [ ] **Step 4: Regenerate + verify compile**

Run: `make sqlc-gen`
Then `go build ./internal/docs/ ./internal/db/gen/` — expect compile errors in
`internal/docs/store.go`/`service.go` where the changed row types are mapped
manually (they're fixed in Task 2). Confirm the new `gen` methods exist:
`InsertDocPageRevision`, `ListDocPageRevisions`, `CountDocPageRevisions`,
`GetDocPageRevision`, `GetDocPageMaxRevisionVersion`, `DeleteDocPageRevision`,
`DeleteDocPageRevisions`.

- [ ] **Step 5: Commit**

```bash
git add migrations/000987_doc_page_revisions_feature_image.* internal/db/queries/docs.sql internal/db/gen .jeos
git commit -m "feat(docs): doc_page_revisions + feature_image_id schema and queries"
```

---

### Task 2: Backend types + feature-image plumbing

**Files:**
- Modify: `internal/docs/types.go`
- Modify: `internal/docs/service.go` (toDocPage, summary/image handling, media validation, Store wiring)
- Modify: `internal/docs/store.go` (row→gen mappings for new columns)
- Test: `internal/docs/docs_test.go`

**Interfaces:**
- Produces: `DocPage.FeatureImageID *string`, `DocPage.FeatureImageURL string`,
  `DocPageSummary.FeatureImageURL string`, `DocNavItem.FeatureImageURL string`,
  `CreateDocPageRequest{FeatureImageID *string, Summary *string}`,
  `UpdateDocPageRequest{FeatureImageID *string, Summary *string}` —
  consumed by Tasks 3–5 and the frontend.

- [ ] **Step 1: Failing tests**

Append to `internal/docs/docs_test.go`:

```go
func TestToDocPageFeatureImage(t *testing.T) {
	imgID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	p := gen.DocPage{
		ID:              pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
		Title:           "T", Slug: "t", Audience: "users",
		FeatureImageID:  imgID,
		FeatureImageBucket: "media",
		FeatureImagePath:   "docs/hero.webp",
	}
	got := toDocPage(p)
	if got.FeatureImageID == nil || *got.FeatureImageID != imgID.String() {
		t.Fatalf("feature_image_id = %v", got.FeatureImageID)
	}
	if got.FeatureImageURL == "" {
		t.Fatal("feature_image_url empty")
	}
}

func TestUpdateDocPageRequestFeatureImageJSON(t *testing.T) {
	body := `{"title":"T","feature_image_id":"","summary":"typo fix"}`
	var req UpdateDocPageRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	if req.FeatureImageID == nil || *req.FeatureImageID != "" {
		t.Fatalf("feature_image_id = %v", req.FeatureImageID)
	}
	if req.Summary == nil || *req.Summary != "typo fix" {
		t.Fatalf("summary = %v", req.Summary)
	}
}
```

Run: `go test ./internal/docs/ -run 'FeatureImage' -v` → FAIL (fields don't exist).

- [ ] **Step 2: Types**

In `types.go`, add to `DocPage`, `DocPageSummary`, `DocNavItem`:

```go
FeatureImageID  *string `json:"feature_image_id,omitempty"`
FeatureImageURL string  `json:"feature_image_url"`
```

Add to `CreateDocPageRequest` and `UpdateDocPageRequest` (doc comment: `""` clears, nil preserves on update):

```go
FeatureImageID *string `json:"feature_image_id,omitempty"`
Summary        *string `json:"summary,omitempty"`
```

Add the revision DTO:

```go
// DocRevision is one snapshot in a page's content history (§44 amendment 5).
// Content is populated only by the single-revision endpoint.
type DocRevision struct {
	ID             string          `json:"id"`
	DocPageID      string          `json:"doc_page_id"`
	Version        int32           `json:"version"`
	Title          string          `json:"title"`
	Summary        *string         `json:"summary,omitempty"`
	FeatureImageID *string         `json:"feature_image_id,omitempty"`
	CreatedBy      *string         `json:"created_by,omitempty"`
	CreatedAt      string          `json:"created_at"`
	Excerpt        string          `json:"excerpt"`
	IsLatest       bool            `json:"is_latest"`
	Content        json.RawMessage `json:"content,omitempty"`
}
```

- [ ] **Step 3: store.go row mappings**

The `gen.*Row` types now carry `FeatureImageID`, `FeatureImageBucket`,
`FeatureImagePath`. Add them to each manual `gen.DocPage{...}` literal in
`getBySlug`, `getBySlugAnyStatus`, `getByID` (copy `FeatureImageID`,
`FeatureImageBucket`, `FeatureImagePath` from the row). For `listByAudience`/
`listAllByAudience`/`listAll`/`listPublished` the row types are used directly —
no mapping changes needed.

- [ ] **Step 4: service.go — URL resolution + request handling**

4a. Import `"github.com/jedafe/japa/internal/media"`. Add helper:

```go
// docMediaURL builds the served image URL for a media row's storage coords —
// same mediaURL helper pattern as blog's cover_image_url (§44 amendment 5).
func docMediaURL(bucket, path *string) string {
	if bucket == nil || path == nil || *bucket == "" || *path == "" {
		return ""
	}
	return media.MediaURL(*bucket, *path, media.ImageProxyOpts{})
}
```

Check `media.MediaURL` signature: `media.MediaURL(storageBucket, storagePath string, opts media.ImageProxyOpts) string` (from `internal/blog/handlers.go:409`).

4b. `toDocPage`: populate `FeatureImageID: uuidPtr(p.FeatureImageID)`,
`FeatureImageURL: docMediaURL(p.FeatureImageBucket, p.FeatureImagePath)`.
The gen types emit `*string` for the join columns (LEFT JOIN nullability) —
verify with `go doc` after regen; if they're plain `string`, wrap as
`&row.FeatureImageBucket`.

4c. `ListDocPagesByAudience` and `navItemFromRow`: set `FeatureImageURL`
from the row's bucket/path join columns.

4d. `resolveFeatureImage` helper on Service — validate + return `pgtype.UUID`:

```go
// resolveFeatureImage validates a request's feature_image_id: empty string
// clears (NULL), a uuid must reference a live media row (§44 amendment 5).
func (s *Service) resolveFeatureImage(ctx context.Context, raw *string, fallback pgtype.UUID) (pgtype.UUID, error) {
	if raw == nil {
		return fallback, nil
	}
	if *raw == "" {
		return pgtype.UUID{}, nil
	}
	var id pgtype.UUID
	if err := id.Scan(*raw); err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: feature_image_id", errpkg.ErrInvalid)
	}
	if _, err := s.read.GetMediaByID(ctx, id); err != nil {
		return pgtype.UUID{}, fmt.Errorf("%w: feature_image_id", errpkg.ErrInvalid)
	}
	return id, nil
}
```

(`s.read` is `*gen.Queries`; `GetMediaByID` already exists.)
Note: `s.read` lives on `Store`, so make it a `Store` method or call
`s.store.read` — use `s.store.read.GetMediaByID`.

4e. `CreateDocPage`: after the parent validation block, resolve
`featureImage, err := s.resolveFeatureImage(ctx, req.FeatureImageID, pgtype.UUID{})`,
pass `FeatureImageID: featureImage` into `gen.CreateDocPageParams`.

4f. `UpdateDocPage`: resolve `featureImage, err := s.resolveFeatureImage(ctx, req.FeatureImageID, existing.FeatureImageID)` near the taxonomy block; pass into `gen.UpdateDocPageParams`.

- [ ] **Step 5: Run tests + build**

`go test ./internal/docs/ && go build ./internal/docs/` → PASS.

- [ ] **Step 6: Commit** — `feat(docs): feature_image_id DTOs, media validation, URL resolution`

---

### Task 3: Revision writes on create/update + restore

**Files:**
- Modify: `internal/docs/store.go` (pool + BeginTx + revision store fns)
- Modify: `internal/docs/service.go` (revisionRequired, tx writes, RestoreDocPageRevision)
- Modify: `internal/server/server.go:1330` (Store now needs `pool`) — check `NewService` call sites; `NewService` signature is unchanged (it builds the Store itself — just add `pool: database` inside it).
- Test: `internal/docs/docs_test.go`

**Interfaces:**
- Produces: `Service.RestoreDocPageRevision(ctx, pageID, revID pgtype.UUID, summary string, userID pgtype.UUID) error`;
  `revisionRequired(existing gen.DocPage, title string, content []byte, featureImage pgtype.UUID) bool` (pure, exported? keep unexported but tested in-package);
  `Store.BeginTx(ctx) (pgx.Tx, error)`.

- [ ] **Step 1: Failing tests for the predicate**

Append to `docs_test.go`:

```go
func TestRevisionRequired(t *testing.T) {
	base := gen.DocPage{
		Title:   "T",
		Content: []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`),
	}
	same := []byte(`{"content":[{"type":"paragraph"}],"type":"doc"}`) // key order differs
	if revisionRequired(base, "T", same, pgtype.UUID{}) {
		t.Fatal("unchanged title/content/image must not version")
	}
	if !revisionRequired(base, "T2", same, pgtype.UUID{}) {
		t.Fatal("title change must version")
	}
	if !revisionRequired(base, "T", []byte(`{"type":"doc","content":[]}`), pgtype.UUID{}) {
		t.Fatal("content change must version")
	}
	if !revisionRequired(base, "T", same, pgtype.UUID{Bytes: [16]byte{9}, Valid: true}) {
		t.Fatal("feature_image change must version")
	}
}
```

Run → FAIL (function undefined).

- [ ] **Step 2: Store — pool + tx + revision helpers**

`store.go`: add `pool *db.DB` field (import `internal/db`), set it in
`NewService` (`store: &Store{read: gen.New(database.Reader), write: gen.New(database.Writer), pool: database}`),
add:

```go
// BeginTx opens a writer-pool transaction for page write + revision insert —
// the snapshot must commit or roll back with its page write (§44 amendment 5).
func (s *Store) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Writer.Begin(ctx)
}

// insertRevisionTx writes a revision snapshot inside tx (version = MAX+1).
func (s *Store) insertRevisionTx(ctx context.Context, tx pgx.Tx, p gen.InsertDocPageRevisionParams) error {
	_, err := s.write.WithTx(tx).InsertDocPageRevision(ctx, p)
	return err
}
```

Add `pgx` import. `NewStore(read, write)` keeps working — `pool` stays nil for
unit tests; service code must fall back to sequential writes when `BeginTx`
fails only in tests? **Simpler contract:** revision insert uses the tx when the
pool exists, otherwise `s.write` directly:

```go
func (s *Service) withWriteTx(ctx context.Context, fn func(q *gen.Queries) error) error {
	if s.store.pool == nil {
		return fn(s.store.write)
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(s.store.write.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

Then in `CreateDocPage` wrap the create+revision insert, and in
`UpdateDocPage` the update+revision insert. `gen.CreateDocPage`/`UpdateDocPage`
must be called through the tx `q` — refactor `s.store.create`/`s.store.update`
to accept `q *gen.Queries` (or call `q.CreateDocPage`/`q.UpdateDocPage`
directly in service and keep the row-count check inline — choose the latter:
move the `mapWriteErr` logic by calling `s.write` through `fn(q)`).

- [ ] **Step 3: `revisionRequired` + wire into Create/Update/Restore**

```go
// revisionRequired reports whether a write changed the fields history
// captures — title, content, or feature image (§44 amendment 5).
// Organizational moves (slug/category/parent/position/audience) never
// version; content compares semantically (amendment 3).
func revisionRequired(existing gen.DocPage, title string, content []byte, featureImage pgtype.UUID) bool {
	if title != existing.Title {
		return true
	}
	if !docContentEqual(content, existing.Content) {
		return true
	}
	return featureImage != existing.FeatureImageID
}
```

In `CreateDocPage`, after the successful create:

```go
created := auth.FromContext(ctx)
var createdBy pgtype.UUID
if created != nil { createdBy = created.UserID }
// v1 snapshot — same tx as the page insert.
```

(The create path already computes `createdBy` — reuse that variable.)
Wrap in `withWriteTx`: `q.CreateDocPage(...)` → `q.InsertDocPageRevision({DocPageID: id, Title: title, Content: contentBytes, FeatureImageID: featureImage, Summary: req.Summary, CreatedBy: createdBy})`.

`Summary` param type is `*string` (sqlc.narg) — pass `req.Summary` directly;
`CreatedBy` is `pgtype.UUID` (nullable via narg) — pass `createdBy`.

In `UpdateDocPage`, after computing all effective values but before
`s.store.update`, compute:

```go
writeRevision := revisionRequired(existing, title, contentBytes, featureImage)
```

Wrap the update in `withWriteTx`; inside `fn`, run `q.UpdateDocPage`, check
rows, then when `writeRevision` run `q.InsertDocPageRevision` with the new
effective values + `req.Summary`.

- [ ] **Step 4: `RestoreDocPageRevision`**

```go
// RestoreDocPageRevision copies a revision's title/content/feature_image
// back onto the page and snapshots the restore itself as a new revision
// (§44 amendment 5). The amendment-3 demotion applies: restoring over an
// approved page voids the approval.
func (s *Service) RestoreDocPageRevision(ctx context.Context, pageID, revID pgtype.UUID, summary string, userID pgtype.UUID) error {
	existing, err := s.store.getByID(ctx, pageID)
	if err != nil {
		return fmt.Errorf("fetching existing page: %w", err)
	}
	rev, err := s.store.read.GetDocPageRevision(ctx, revID)
	if err != nil || rev.DocPageID != pageID {
		return errpkg.ErrNotFound
	}
	if summary == "" {
		summary = fmt.Sprintf("Restored from v%d", rev.Version)
	}
	clearReview := false
	status := existing.ReviewStatus
	if existing.ReviewStatus == "approved" &&
		(rev.Title != existing.Title || !docContentEqual(rev.Content, existing.Content)) {
		status = "in_review"
		clearReview = true
	}
	return s.withWriteTx(ctx, func(q *gen.Queries) error {
		rows, err := q.UpdateDocPage(ctx, gen.UpdateDocPageParams{
			ID:            pageID,
			Title:         rev.Title,
			Content:       rev.Content,
			ReviewStatus:  status,
			Slug:          existing.Slug,
			ParentID:      existing.ParentID,
			Position:      existing.Position,
			Audience:      existing.Audience,
			Category:      existing.Category,
			Subcategory:   existing.Subcategory,
			FeatureImageID: rev.FeatureImageID,
			ClearReview:   &clearReview,
		})
		if err != nil {
			return mapWriteErr(err)
		}
		if rows == 0 {
			return errpkg.ErrStaleWrite
		}
		_, err = q.InsertDocPageRevision(ctx, gen.InsertDocPageRevisionParams{
			DocPageID:      pageID,
			Title:          rev.Title,
			Content:        rev.Content,
			FeatureImageID: rev.FeatureImageID,
			Summary:        &summary,
			CreatedBy:      userID,
		})
		return err
	})
	// (cache invalidation after the tx, same as UpdateDocPage)
}
```

Note: `UpdateDocPage` params type after Task 1 includes `FeatureImageID`.
Invalidate caches after success: `s.invalidateCache(ctx, existing.Audience, existing.Slug)`.

- [ ] **Step 5: Test + build + commit**

`go test ./internal/docs/ && go build ./internal/...` → PASS.
Commit: `feat(docs): revision snapshots on write + restore path`.

---

### Task 4: Revision endpoints + route registration

**Files:**
- Modify: `internal/docs/handlers.go` (5 new handlers)
- Modify: `internal/docs/service.go` (List/Get/Delete/Clear service fns)
- Modify: `internal/server/server.go` (~line 6609, inside the existing `docs.manage` route group)

- [ ] **Step 1: Service functions**

```go
// ListDocPageRevisions returns a page's history newest-first with excerpt
// + is_latest on v-max (§44 amendment 5).
func (s *Service) ListDocPageRevisions(ctx context.Context, pageID pgtype.UUID, page, limit int32) ([]DocRevision, int64, error) {
	if limit <= 0 { limit = 20 }
	total, err := s.store.read.CountDocPageRevisions(ctx, pageID)
	if err != nil { return nil, 0, err }
	rows, err := s.store.read.ListDocPageRevisions(ctx, gen.ListDocPageRevisionsParams{
		DocPageID: pageID, ResultLimit: limit, ResultOffset: (page - 1) * limit,
	})
	if err != nil { return nil, 0, err }
	maxV, _ := s.store.read.GetDocPageMaxRevisionVersion(ctx, pageID)
	items := make([]DocRevision, len(rows))
	for i, r := range rows {
		items[i] = DocRevision{
			ID: r.ID.String(), DocPageID: r.DocPageID.String(), Version: r.Version,
			Title: r.Title, Summary: r.Summary, FeatureImageID: uuidPtr(r.FeatureImageID),
			CreatedBy: uuidPtr(r.CreatedBy),
			CreatedAt: r.CreatedAt.Time.Format(time.RFC3339Nano),
			Excerpt: extractExcerpt(r.Content, excerptMaxLen),
			IsLatest: r.Version == maxV,
		}
	}
	return items, total, nil
}

func (s *Service) GetDocPageRevision(ctx context.Context, revID pgtype.UUID) (*DocRevision, error) {
	r, err := s.store.read.GetDocPageRevision(ctx, revID)
	if err != nil { return nil, errpkg.ErrNotFound }
	return &DocRevision{
		ID: r.ID.String(), DocPageID: r.DocPageID.String(), Version: r.Version,
		Title: r.Title, Summary: r.Summary, FeatureImageID: uuidPtr(r.FeatureImageID),
		CreatedBy: uuidPtr(r.CreatedBy), CreatedAt: r.CreatedAt.Time.Format(time.RFC3339Nano),
		Excerpt: extractExcerpt(r.Content, excerptMaxLen), IsLatest: false, Content: r.Content,
	}, nil
}

// DeleteDocPageRevision removes a non-latest revision (§44 amendment 5 — the
// latest anchors history to the live page and is undeletable).
func (s *Service) DeleteDocPageRevision(ctx context.Context, revID pgtype.UUID) error {
	rev, err := s.store.read.GetDocPageRevision(ctx, revID)
	if err != nil { return errpkg.ErrNotFound }
	maxV, err := s.store.read.GetDocPageMaxRevisionVersion(ctx, rev.DocPageID)
	if err != nil { return err }
	if rev.Version >= maxV {
		return fmt.Errorf("%w: latest revision cannot be deleted", errpkg.ErrConflict)
	}
	rows, err := s.store.write.DeleteDocPageRevision(ctx, revID)
	if err != nil { return err }
	if rows == 0 { return errpkg.ErrNotFound }
	return nil
}

// ClearDocPageRevisions wipes a page's entire history (§44 amendment 5).
func (s *Service) ClearDocPageRevisions(ctx context.Context, pageID pgtype.UUID) error {
	return s.store.write.DeleteDocPageRevisions(ctx, pageID)
}
```

Check `errpkg.ErrConflict` exists — `grep ErrConflict internal/errors/`; if the
sentinel for 409 is different (e.g. `ErrDuplicate`/`ErrStaleWrite`), use it.
`ErrStaleWrite` maps to 409 — reuse it if no `ErrConflict`.

- [ ] **Step 2: Handlers (append to handlers.go admin section)**

Each: parse UUID param, `checkPageAudienceGate` on the owning page (for
`revId` handlers, resolve `rev.DocPageID` via `GetDocPageRevision` first),
decode body where needed.

```go
// ListDocPageRevisions handles GET /api/v1/admin/docs/pages/{id}/revisions.
func (h *Handlers) ListDocPageRevisions(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}
	if !h.checkPageAudienceGate(w, r, id) {
		return
	}
	var page, limit int32 = 1, 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 { page = int32(n) }
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 { limit = int32(n) }
	}
	items, total, err := h.Service.ListDocPageRevisions(r.Context(), id, page, limit)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]any{"page": page, "limit": limit, "total": total},
	})
}

// GetDocRevision handles GET /api/v1/admin/docs/revisions/{revId}.
func (h *Handlers) GetDocRevision(w http.ResponseWriter, r *http.Request) {
	var revID pgtype.UUID
	if err := revID.Scan(chi.URLParam(r, "revId")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid revision ID")
		return
	}
	rev, err := h.Service.GetDocPageRevision(r.Context(), revID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	var pageID pgtype.UUID
	pageID.Scan(rev.DocPageID)
	if !h.checkPageAudienceGate(w, r, pageID) {
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rev})
}

// RestoreDocRevision handles POST /api/v1/admin/docs/pages/{id}/revisions/{revId}/restore.
func (h *Handlers) RestoreDocRevision(w http.ResponseWriter, r *http.Request) {
	var id, revID pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}
	if err := revID.Scan(chi.URLParam(r, "revId")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid revision ID")
		return
	}
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	if !h.checkPageAudienceGate(w, r, id) {
		return
	}
	var input struct {
		Summary string `json:"summary"`
	}
	json.NewDecoder(r.Body).Decode(&input) // optional body
	if err := h.Service.RestoreDocPageRevision(r.Context(), id, revID, strings.TrimSpace(input.Summary), user.UserID); err != nil {
		handleServiceError(w, err)
		return
	}
	h.enqueueSearchIndex(id, "index")
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "restored"}})
}

// DeleteDocRevision handles DELETE /api/v1/admin/docs/revisions/{revId}.
func (h *Handlers) DeleteDocRevision(w http.ResponseWriter, r *http.Request) {
	var revID pgtype.UUID
	if err := revID.Scan(chi.URLParam(r, "revId")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid revision ID")
		return
	}
	rev, err := h.Service.GetDocPageRevision(r.Context(), revID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	var pageID pgtype.UUID
	pageID.Scan(rev.DocPageID)
	if !h.checkPageAudienceGate(w, r, pageID) {
		return
	}
	if err := h.Service.DeleteDocPageRevision(r.Context(), revID); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ClearDocRevisions handles DELETE /api/v1/admin/docs/pages/{id}/revisions.
func (h *Handlers) ClearDocRevisions(w http.ResponseWriter, r *http.Request) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid page ID")
		return
	}
	if !h.checkPageAudienceGate(w, r, id) {
		return
	}
	if err := h.Service.ClearDocPageRevisions(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 3: Register routes in `internal/server/server.go`**

Inside the existing `docs.manage` route chain (next to
`Post("/admin/docs/pages/{id}/publish", ...)` ~line 6609), add:

```go
Get("/admin/docs/pages/{id}/revisions", docsHandlers.ListDocPageRevisions)
Delete("/admin/docs/pages/{id}/revisions", docsHandlers.ClearDocRevisions)
Post("/admin/docs/pages/{id}/revisions/{revId}/restore", docsHandlers.RestoreDocRevision)
Get("/admin/docs/revisions/{revId}", docsHandlers.GetDocRevision)
Delete("/admin/docs/revisions/{revId}", docsHandlers.DeleteDocRevision)
```

- [ ] **Step 4: Build + test + commit**

`go build ./... && go test ./internal/docs/` → PASS.
Commit: `feat(docs): revision list/get/restore/delete/clear endpoints`.

---

### Task 5: Frontend — types, feature-image picker, edit summary

**Files:**
- Modify: `apps/docs/src/lib/types/index.ts`
- Create: `apps/docs/src/lib/components/docs/FeatureImageField.svelte`
- Modify: `apps/docs/src/lib/components/docs/DocEditForm.svelte`
- Modify: `apps/docs/src/lib/components/docs/NewPageForm.svelte`
- Modify: `apps/docs/src/lib/server/docs-author.ts` (pass feature_image_id + summary)
- Modify: `apps/docs/src/lib/i18n/en.json` (+ `fr.json` mirror keys)

**Interfaces:**
- `FeatureImageField` props: `value: string` (media id), `imageUrl: string`
  (preview), `onchange: (id: string, url: string) => void`, `disabled?: boolean`.

- [ ] **Step 1: Types**

`types/index.ts` — add to `DocPage`, `DocPageSummary`, `DocNavItem`:
`feature_image_id?: string | null; feature_image_url?: string;`
To `DocPageCreateRequest`/`DocPageUpdateRequest`:
`feature_image_id?: string | null; summary?: string;`
New:

```ts
/** One revision row in a doc page's history (§44 amendment 5). */
export interface DocRevision {
	id: string;
	doc_page_id: string;
	version: number;
	title: string;
	summary?: string | null;
	feature_image_id?: string | null;
	created_by?: string | null;
	created_at: string;
	excerpt: string;
	is_latest: boolean;
	content?: unknown;
}
```

- [ ] **Step 2: `FeatureImageField.svelte`**

Pattern it on `DocEditor`'s MediaPickerModal wiring (lines 32–88 of
`DocEditor.svelte`): a thumbnail preview (when `imageUrl`), "Choose
image"/"Replace" + "Remove" buttons, `MediaPickerModal` with
`listMyMedia`/`uploadMediaImage`/`stockApi` from `$lib/api/media`.
Emits `onchange(item.id, item.url)` on select and `onchange('', '')` on
remove. Keep markup minimal — `.fif-preview` (16:9, max-h 160px,
`object-fit: cover`, border-radius card) above the buttons; use the app's
`var(--color-*)` tokens.

- [ ] **Step 3: `DocEditForm` — wire the two fields**

- State: `let editFeatureImageId = $state(untrack(() => page.feature_image_id ?? ''));`
  `let editFeatureImageUrl = $state(untrack(() => page.feature_image_url ?? ''));`
  `let editSummary = $state('');` — summary is per-save, never seeded.
  Add all three to the `seededFor` reseed effect and to `dirty`
  (`editFeatureImageId !== (page.feature_image_id ?? '') || editSummary.trim() !== ''`).
- Markup: `<FeatureImageField>` inside `.ef-grid` (or a full-width field above
  the editor); a summary `<input>` placed next to "Save changes" labeled
  `t('edit.summary_label', 'Edit summary (optional)')`.
- In the `use:enhance` callback, add:
  `formData.set('feature_image_id', editFeatureImageId);`
  `formData.set('summary', editSummary.trim());`
  and after a successful save, `editSummary = ''` (per-save field).

- [ ] **Step 4: `NewPageForm` — same wiring**

`let featureImageId = $state(''); let featureImageUrl = $state(''); let summary = $state('');`
Render the same two fields; push into the form data on submit (mirror the
existing `formData.set('content', ...)` pattern — check where NewPageForm
sets hidden fields in its enhance callback and add the same three keys).

- [ ] **Step 5: `docs-author.ts` — forward the fields**

In `createDocPageAction` and `saveDocPageAction`, read
`const featureImageId = String(form.get('feature_image_id') ?? '').trim();`
and `const summary = String(form.get('summary') ?? '').trim();`, then set
`body.feature_image_id = featureImageId || null` (PATCH: only set when the
form field is present — `form.has('feature_image_id')` guard, `null` clears)
and `body.summary = summary || undefined`.
`values` echo objects: add `featureImageId` so a failed submit reseeds.

- [ ] **Step 6: i18n keys** (en.json + fr.json)

`"edit.summary_label"`, `"edit.summary_placeholder"`,
`"edit.feature_image"`, `"edit.choose_image"`, `"edit.replace_image"`,
`"edit.remove_image"`, `"history.link"`, `"history.title"`,
`"history.empty"`, `"history.version"`, `"history.restore"`,
`"history.restore_confirm"`, `"history.delete"`, `"history.delete_confirm"`,
`"history.clear_all"`, `"history.clear_confirm"`, `"history.latest"`,
`"history.restored"`, `"history.by"`, `"page.feature_image_alt"`.

- [ ] **Step 7: `bun run check` (apps/docs) + commit**
`feat(docs): feature image picker + edit summary in authoring forms`.

---

### Task 6: Doc page hero + og:image + history link

**Files:**
- Modify: `apps/docs/src/routes/docs/[audience]/[slug]/+page.server.ts`
- Modify: `apps/docs/src/routes/docs/[audience]/[slug]/+page.svelte`

- [ ] **Step 1: og:image**

In the page load's `buildMeta` call add:
`image: page.feature_image_url ? new URL(page.feature_image_url, DOCS_ORIGIN).toString() : undefined`
(`media.MediaURL` may return a path — absolutize against `DOCS_ORIGIN`,
already imported).

- [ ] **Step 2: Hero + history link in +page.svelte**

- Import `History` from lucide-svelte. In `.doc-meta`, next to the existing
  `?edit=1` link add (inside `{#if canEdit}`):
  `<a href="/docs/{data.audience}/{data.page.slug}/history" class="meta-edit-link"><History size={13} />{t('history.link', 'History')}</a>`
- Render the hero between `</header>`... actually inside `<header>` after the
  `<h1>`/meta block or above `<h1>` — place above `<h1>` for a wiki-article
  feel:
  `{#if data.page.feature_image_url}<img class="doc-hero" src={data.page.feature_image_url} alt={t('page.feature_image_alt', 'Feature image')} loading="eager" />{/if}`
  with `.doc-hero { width:100%; max-height:360px; object-fit:cover; border-radius:var(--radius-card); margin-bottom:1.25rem; aspect-ratio:21/9; }` in the style block.

- [ ] **Step 3: `bun run check` + commit**
`feat(docs): hero feature image, og:image, history link on doc page`.

---

### Task 7: History page route

**Files:**
- Create: `apps/docs/src/routes/docs/[audience]/[slug]/history/+page.server.ts`
- Create: `apps/docs/src/routes/docs/[audience]/[slug]/history/+page.svelte`

- [ ] **Step 1: `+page.server.ts`**

Pattern: same shell data as the doc page. `docs.manage` gate via
`assertDocEditor` semantics — reuse the audience access check + permission
check from the sibling `+page.server.ts`; non-editors → 303 `/docs/access-denied`.

```ts
import { error, redirect, fail } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, apiDelete, ApiError } from '$lib/server/api';
import { renderDocContent } from '$lib/tiptap/serialize';
import { buildMeta, DOCS_ORIGIN } from '$lib/seo/meta';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';
import type { DocPage, DocRevision, DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const audience = event.params.audience as DocAudience;
	const slug = event.params.slug;
	const canEdit = event.locals.user?.permissions?.includes('docs.manage') ?? false;
	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}
	if (!canEdit || !event.locals.accessibleAudiences.includes(audience)) {
		throw redirect(303, '/docs/access-denied');
	}
	const ctx = apiContext(event);
	// Any-status fetch: history is an editor surface (§44 amendment 5).
	const page = await apiGet<DocPage>(
		ctx,
		`/admin/docs/page?audience=${encodeURIComponent(audience)}&slug=${encodeURIComponent(slug)}`
	);
	const pageNum = Math.max(1, Number(event.url.searchParams.get('page') ?? '1') || 1);
	const limit = [10, 20, 50, 100].includes(Number(event.url.searchParams.get('limit')))
		? Number(event.url.searchParams.get('limit')) : 20;
	const { items, meta } = await apiList<DocRevision>(
		ctx,
		`/admin/docs/pages/${page.id}/revisions?page=${pageNum}&limit=${limit}`
	);
	const site = (((await event.parent())?.siteSettings ?? {}) as Record<string, string>)['general.site_name'] ?? DEFAULT_SITE_NAME;
	const title = `${tServer(event.locals.locale, 'history.title', 'Revision history')} — ${page.title} · ${site}`;
	return {
		page, audience, slug,
		revisions: items,
		revMeta: meta ?? {},
		revPage: pageNum, revLimit: limit,
		audienceLabelKey: AUDIENCE_LABEL_KEY[audience],
		meta: buildMeta({ title, description: title, path: `/docs/${audience}/${slug}/history` })
	};
};
```

Actions (server-side so token/CSRF never hit the browser):

```ts
export const actions: Actions = {
	restore: async (event) => {
		const form = await event.request.formData();
		const pageId = String(form.get('page_id') ?? '');
		const revId = String(form.get('revision_id') ?? '');
		try {
			await apiPost(apiContext(event), `/admin/docs/pages/${pageId}/revisions/${revId}/restore`, {});
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
		throw redirect(303, `/docs/${event.params.audience}/${event.params.slug}?saved=restored`);
	},
	deleteRevision: async (event) => {
		const form = await event.request.formData();
		const revId = String(form.get('revision_id') ?? '');
		try {
			await apiDelete(apiContext(event), `/admin/docs/revisions/${revId}`);
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
		return { deleted: true };
	},
	clear: async (event) => {
		const form = await event.request.formData();
		const pageId = String(form.get('page_id') ?? '');
		try {
			await apiDelete(apiContext(event), `/admin/docs/pages/${pageId}/revisions`);
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
		return { cleared: true };
	},
	// Lazy fetch of a single revision's full content for the view dialog.
	view: async (event) => {
		const form = await event.request.formData();
		const revId = String(form.get('revision_id') ?? '');
		try {
			const rev = await apiGet<DocRevision>(apiContext(event), `/admin/docs/revisions/${revId}`);
			return { viewId: revId, viewHtml: renderDocContent(rev.content ?? null), viewTitle: rev.title, viewVersion: rev.version };
		} catch (err) {
			if (err instanceof ApiError) return fail(err.status, { error: err.code });
			return fail(500, { error: 'internal_error' });
		}
	}
};
```

(`renderDocContent` signature check: `renderDocContent(content)` returns HTML
string — verify in `lib/tiptap/serialize.ts`.)

- [ ] **Step 2: `+page.svelte`**

Structure mirrors the wiki history page + docs AppShell:

- `AppShell` with same props pattern as `[slug]/+page.svelte` (copy the prop
  block — nav, audiences, collapsed state, `SeoHead`).
- Header: back link to the page (`/docs/{audience}/{slug}`), title
  "Revision history — {page.title}", a **Clear history** button (opens a
  confirm `<details>`/inline confirm block or a small modal; simplest
  in-app convention: `confirm()`-gated form like the wiki pages' patterns —
  use an inline confirm row that posts `?/clear`).
- List: `{#each data.revisions as rev (rev.id)}` — version badge `v{rev.version}`
  (+ "latest" chip when `rev.is_latest`), `rev.title`, `rev.summary`, author +
  `rev.created_at` formatted with `toLocaleString`, `rev.excerpt`.
- Row actions: View (`?/view` → renders `form.viewHtml` in a `.prose` block
  below/in a dialog), Restore (`?/restore` with `confirm()` on submit via
  `use:enhance` `cancel()` when unconfirmed), Delete (`?/deleteRevision`,
  hidden when `rev.is_latest`, same confirm).
- Pagination footer: prev/next links `?page=N&limit=M` + limit select
  (10/20/50/100) — copy the wiki history `pageHref`/`onLimitChange` logic
  (basePath = this route).
- Empty state: `{t('history.empty', 'No revisions yet')}`.

- [ ] **Step 3: `bun run check` + commit**
`feat(docs): revision history page with view/restore/delete/clear`.

---

### Task 8: Listing card thumbnails

**Files:**
- Modify: `apps/docs/src/routes/docs/[audience]/+page.svelte` (docCard snippet)
- Modify: `apps/docs/src/routes/docs/category/[name]/+page.svelte` (item cards)

- [ ] **Step 1: Audience index**

`DocNavItem` now carries `feature_image_url` (backend Task 2 + nav query).
In the `docCard` snippet's `doc-card-link` variant render a thumb when
present:

```svelte
<a href={pageHref(item.slug)} class="doc-card doc-card-link">
	{#if item.feature_image_url}
		<img class="doc-thumb" src={item.feature_image_url} alt="" loading="lazy" />
	{:else}
		<FileText size={16} class="doc-link-icon" />
	{/if}
	<span class="doc-card-title">{item.title}</span>
	<ArrowRight size={15} class="doc-card-arrow rtl:-scale-x-100" />
</a>
```

`.doc-thumb { width: 56px; height: 40px; object-fit: cover; border-radius: calc(var(--radius) - 2px); flex-shrink: 0; }`

Apply the same in the unpublished-pages card loop (`item.feature_image_url`).

- [ ] **Step 2: Category page**

Check the card markup in `category/[name]/+page.svelte` — its items derive
from the nav tree (`DocsCategoryItem`) so the field is available once
`DocNavItem`/`DocsCategoryItem` carries it; add the same `doc-thumb` block.
(`DocsCategoryItem` is built in `lib/docs/catalog.ts` from nav items — pass
`feature_image_url` through if the item type maps fields explicitly.)

- [ ] **Step 3: `bun run check` + commit**
`feat(docs): feature image thumbnails on listing cards`.

---

### Task 9: Final verification

- [ ] `make sqlc-gen` produces no diff (gen committed in Task 1).
- [ ] `go build ./... && go test ./internal/docs/ ./internal/server/`
- [ ] `cd apps/docs && bun run check` clean.
- [ ] Manual pass on a running stack: create page w/ image → hero + card
  thumb + og:image in page source → edit with summary → history shows v1/v2 →
  view v1 → restore → new revision "Restored from v1" → delete a non-latest →
  latest delete blocked → clear all → empty state → next save starts v1.
- [ ] Update spec doc's Status to "Implemented" if anything drifted (note drift
  inline in §44 amendment).

## Self-review notes

- Spec coverage: feature image (Tasks 1,2,5,6,8) ✓; revision history +
  restore + delete + clear (Tasks 1,3,4,7) ✓; edit summary (Tasks 3,5) ✓;
  polish/history link (Task 6) ✓; JEOS + catalog (Task 1) ✓.
- `docs/manage` check — the new routes sit inside the existing
  `RequirePermission(rbacResolver, "docs.manage")` group — verified at
  server.go ~6593–6612.
- Open items the implementer must verify by reading code: `errpkg.ErrConflict`
  existence (fallback `ErrStaleWrite`), `gen` nullability of join columns
  (`*string` vs `string`), `renderDocContent` signature, `MediaURL` exact
  signature.
