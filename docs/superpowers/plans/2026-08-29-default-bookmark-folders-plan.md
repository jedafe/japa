# Default Bookmark Folders by Content Type — Implementation Plan

> **For agentic workers:** Use this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Each task ends with a testable deliverable.

**Goal:** Automatically file every new bookmark into a default folder based on the bookmarked item's content or entity type, pre-seed those folders at registration, and consolidate forum bookmarks into the unified `bookmarks` table so they also appear in `dashboard/bookmarks`.

**Architecture:** Add a `DefaultFolderResolver` in `internal/bookmarkfolders`, wire it into `content`, `users`, and `forum` bookmark creation paths, pre-seed folders on user creation, and migrate `forum_bookmarks` into the existing `bookmarks` table with the new entity types `forum_thread` and `forum_reply`.

**Tech Stack:** Go 1.23+, sqlc + pgx/v5, Chi, SvelteKit 2 + Svelte 5 + Bun, golang-migrate.

**Global Constraints**
- Do not hand-edit `internal/db/gen/*.sql.go`; edit `.sql` queries and run `make sqlc-gen`.
- All Go errors wrap with `fmt.Errorf("...: %w", err)`.
- Migrations must have up/down pairs.
- No hardcoded data in SvelteKit `load()` functions (Rule 16).
- Use `bun` for JS tooling (Rule 18).
- Reference JEOS section files in code comments.

---

### Task 1: Add default-folder resolver to `internal/bookmarkfolders`

**Files:**
- Create: `internal/bookmarkfolders/defaults.go`
- Modify: `internal/bookmarkfolders/bookmarkfolders.go`
- Test: `internal/bookmarkfolders/defaults_test.go`

**Interfaces:**
- Consumes: `gen.Queries` (via existing `Service.read` / `Service.write`).
- Produces: `DefaultFolderResolver` struct with methods `Resolve(ctx, tenantID, userID, entityType, contentType) (FolderResponse, error)` and `CreateDefaultFoldersForUser(ctx, tenantID, userID) error`.

- [ ] **Step 1: Define mapping and resolver**

In `internal/bookmarkfolders/defaults.go`, add a package-level map from content/entity type to folder name and a `DefaultFolderResolver` that uses the existing `Service` to find-or-create folders.

```go
package bookmarkfolders

import (
    "context"
    "fmt"
    "strings"

    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jedafe/japa/internal/db/gen"
)

var contentTypeFolderNames = map[string]string{
    "blog_post":       "blog",
    "discussion":      "discussions",
    "guide":           "guides",
    "page":            "pages",
    "comparison":      "comparisons",
    "event":           "events",
    "job_listing":     "jobs",
    "story":           "stories",
    "achievement":     "achievements",
    "service_listing": "services",
}

var entityTypeFolderNames = map[string]string{
    "blog_post":         "blog",
    "learning_track":    "learn",
    "doc_page":          "docs",
    "forum_thread":      "discussions",
    "forum_reply":       "discussions",
    "thread":            "discussions",
    "reply":             "discussions",
    "space":             "spaces",
    "tag":               "tags",
    "comment":           "comments",
}

var defaultFolderOrder = []string{
    "blog", "discussions", "guides", "pages", "comparisons", "events",
    "jobs", "stories", "achievements", "services", "learn", "docs",
    "spaces", "tags", "comments",
}
```

- [ ] **Step 2: Implement `Resolve`**

```go
func (r *DefaultFolderResolver) Resolve(ctx context.Context, tenantID, userID pgtype.UUID, entityType, contentType string) (FolderResponse, error) {
    folderName := ""
    switch entityType {
    case "content", "blog_post":
        folderName = contentTypeFolderNames[contentType]
    default:
        if n, ok := entityTypeFolderNames[entityType]; ok {
            folderName = n
        }
    }
    if folderName == "" {
        return FolderResponse{}, nil
    }
    return r.findOrCreateFolder(ctx, tenantID, userID, folderName)
}
```

`findOrCreateFolder` should first attempt `GetBookmarkFolderByName` (new query or inline `ListBookmarkFoldersByUser` + filter) and, if missing, create via `CreateBookmarkFolder`. Determine position by looking up `folderName` in `defaultFolderOrder`.

- [ ] **Step 3: Implement `CreateDefaultFoldersForUser`**

Iterate `defaultFolderOrder` and call `findOrCreateFolder` for each name, swallowing duplicate-key errors. This pre-seeds all default folders at registration.

- [ ] **Step 4: Add `DefaultFolderResolver` to `Service`**

Modify `internal/bookmarkfolders/bookmarkfolders.go` to expose `Service.Resolver()` or add a `NewDefaultFolderResolver(service *Service)` helper. Ensure the resolver is nil-safe and uses `s.write` for creates.

- [ ] **Step 5: Write tests**

In `internal/bookmarkfolders/defaults_test.go`:
- Test `Resolve` for `blog_post` -> `blog`, `learning_track` -> `learn`, `forum_thread` -> `discussions`.
- Test unknown entity/content type returns empty `FolderResponse{}` and no folder is created.
- Test `CreateDefaultFoldersForUser` creates all folders and is idempotent on a second call.

Run: `cd /home/jmobile/websites/japa/japaconnect && go test ./internal/bookmarkfolders/...`

---

### Task 2: Pre-seed default folders at user registration

**Files:**
- Modify: `internal/auth/handlers.go`
- Modify: `internal/auth/oauth_handlers.go`
- Modify: `internal/admin/users.go`
- Modify: `internal/server/server.go`

**Interfaces:**
- Consumes: `bookmarkfolders.Service` and its `CreateDefaultFoldersForUser` method.
- Produces: Default folders exist immediately after user creation.

- [ ] **Step 1: Add setter/field to auth handlers and admin**

Add a `bookmarkFolders *bookmarkfolders.Service` field to `internal/auth.Handlers` and `internal/admin.Handlers` (or store it on the existing service struct). Add setters if constructors are already wired.

- [ ] **Step 2: Call `CreateDefaultFoldersForUser` after `CreateUser`**

In each handler where `Queries.CreateUser` returns a `userID`, add:

```go
if err := h.bookmarkFolders.CreateDefaultFoldersForUser(r.Context(), tenantID, userID); err != nil {
    slog.Warn("failed to create default bookmark folders", "error", err, "user_id", userID)
}
```

Do this in:
- `internal/auth/handlers.go` after password-based registration.
- `internal/auth/oauth_handlers.go` after OAuth auto-registration.
- `internal/admin/users.go` after admin user creation.

- [ ] **Step 3: Wire in `internal/server/server.go`**

`bookmarkFoldersService` is already created at line 433. Pass it to the auth and admin handlers:

```go
authHandlers.SetBookmarkFolders(bookmarkFoldersService)
oauthHandlers.SetBookmarkFolders(bookmarkFoldersService)
adminHandlers.SetBookmarkFolders(bookmarkFoldersService)
```

(Use the existing setter pattern or adjust constructor calls.)

---

### Task 3: Wire resolver into `content.CreateBookmark`

**Files:**
- Modify: `internal/content/content.go`
- Modify: `internal/content/store.go`
- Modify: `internal/content/handlers.go`
- Modify: `internal/server/server.go`
- Modify: `internal/content/content_test.go` (or create `internal/content/bookmark_test.go`)

**Interfaces:**
- Consumes: `bookmarkfolders.DefaultFolderResolver`.
- Produces: `CreateBookmark` sets `folder` and `folder_id` based on `content_type` when the caller does not override.

- [ ] **Step 1: Add resolver dependency to `content.Service`**

In `internal/content/content.go` add:

```go
type folderResolver interface {
    Resolve(ctx context.Context, tenantID, userID pgtype.UUID, entityType, contentType string) (bookmarkfolders.FolderResponse, error)
}

type Service struct {
    ...
    folderResolver folderResolver
}

func (s *Service) SetFolderResolver(fr folderResolver) { s.folderResolver = fr }
```

In `internal/server/server.go` after `bookmarkFoldersService` is created:

```go
contentService.SetFolderResolver(bookmarkFoldersService.Resolver())
```

- [ ] **Step 2: Update `CreateBookmark` in `internal/content/store.go`**

Change the function signature to accept the content type or look it up. Prefer adding a `contentType` parameter because the handler already loads the content for permission checks in some paths; if not available, the store can call `s.read.GetContentByID`.

```go
func (s *Service) CreateBookmark(ctx context.Context, tenantID, userID pgtype.UUID, contentType string, contentID pgtype.UUID, folder string, folderID pgtype.UUID) (gen.CreateBookmarkRow, error) {
    if folder == "" {
        if s.folderResolver != nil {
            f, err := s.folderResolver.Resolve(ctx, tenantID, userID, EntityContent, contentType)
            if err == nil && f.ID != "" {
                folder = f.Name
                var fid pgtype.UUID
                _ = fid.Scan(f.ID)
                folderID = fid
            }
        }
        if folder == "" {
            folder = "default"
        }
    }
    // existing CreateBookmark call
}
```

- [ ] **Step 3: Update `internal/content/handlers.go:AddBookmark`**

The handler already has `contentID`. Load the content row to pass `content.ContentType` to `CreateBookmark`. `GetContent` already exists on the service.

```go
content, err := h.Service.GetContent(r.Context(), contentID, tenantID)
if err != nil {
    handleServiceError(w, err)
    return
}

bm, err := h.Service.CreateBookmark(r.Context(), tenantID, user.UserID, content.ContentType, contentID, input.Folder, folderID)
```

- [ ] **Step 4: Update persona dispatcher in `internal/server/server.go`**

At line 7243 the dispatcher calls `d.content.CreateBookmark` with a hardcoded `folder = "default"` and `zeroFolderID`. Update this call to pass `entityType` and the entity's content type if known; otherwise rely on resolver.

```go
folder, folderID, err := d.resolveDefaultFolder(ctx, tenantID, personaUserID, entityType, entityID)
if err != nil {
    return err
}
_, err = d.content.CreateBookmark(ctx, tenantID, personaUserID, entityType, entityID, folder, folderID)
```

If `entityType == "content"`, load the content type from `contents` via `GetContentByID`.

- [ ] **Step 5: Test**

Add a test in `internal/content/bookmark_test.go` that:
- Creates a content with `content_type = blog_post`.
- Calls `CreateBookmark` with empty `folder`.
- Verifies the resulting `folder` string is `blog` and `folder_id` matches a newly created `blog` folder for that user.

Run: `go test ./internal/content/...`

---

### Task 4: Wire resolver into `users.CreateBookmark`

**Files:**
- Modify: `internal/users/service.go`
- Modify: `internal/users/users.go` (or `store.go` if Service struct lives there)
- Modify: `internal/server/server.go`
- Modify: `internal/users/service_test.go` (or create `internal/users/bookmark_test.go`)

**Interfaces:**
- Consumes: `bookmarkfolders.DefaultFolderResolver`.
- Produces: `POST /me/bookmarks` auto-files by `entity_type` (and `content_type` when `entity_type = content`).

- [ ] **Step 1: Add resolver dependency to `users.Service`**

```go
type folderResolver interface {
    Resolve(ctx context.Context, tenantID, userID pgtype.UUID, entityType, contentType string) (bookmarkfolders.FolderResponse, error)
}

type Service struct {
    ...
    folderResolver folderResolver
}

func (s *Service) SetFolderResolver(fr folderResolver) { s.folderResolver = fr }
```

Wire in `internal/server/server.go`:

```go
usersService.SetFolderResolver(bookmarkFoldersService.Resolver())
```

- [ ] **Step 2: Update `CreateBookmark` in `internal/users/service.go`**

```go
func (s *Service) CreateBookmark(ctx context.Context, tenantID, userID, entityID, folderID pgtype.UUID, entityType, folder string) (gen.CreateBookmarkRow, error) {
    if folder == "" {
        contentType := ""
        if entityType == "content" || entityType == "blog_post" {
            c, err := s.store.read.GetContentByID(ctx, gen.GetContentByIDParams{ID: entityID, TenantID: tenantID})
            if err == nil {
                contentType = c.ContentType
            }
        }
        if s.folderResolver != nil {
            f, err := s.folderResolver.Resolve(ctx, tenantID, userID, entityType, contentType)
            if err == nil && f.ID != "" {
                folder = f.Name
                var fid pgtype.UUID
                _ = fid.Scan(f.ID)
                folderID = fid
            }
        }
        if folder == "" {
            folder = "default"
        }
    }
    // existing CreateBookmark call
}
```

- [ ] **Step 3: Test**

Add test for `POST /me/bookmarks` with `entity_type: learning_track` -> folder `learn`, and `entity_type: content` with a blog_post -> folder `blog`.

Run: `go test ./internal/users/...`

---

### Task 5: Consolidate forum bookmarks into `bookmarks` and wire resolver

**Files:**
- Create: `migrations/000XXX_consolidate_forum_bookmarks.up.sql`
- Create: `migrations/000XXX_consolidate_forum_bookmarks.down.sql`
- Modify: `internal/db/queries/forum.sql`
- Modify: `internal/db/queries/users.sql`
- Modify: `internal/db/queries/contents.sql` (if needed for `CreateBookmark` conflict target)
- Modify: `internal/forum/service.go`
- Modify: `internal/forum/store.go`
- Modify: `internal/forum/handlers.go`
- Modify: `internal/forum/forum.go` (for `EntityType*` constants)
- Modify: `internal/server/server.go`
- Modify: `internal/forum/forum_test.go`

**Interfaces:**
- Consumes: `bookmarkfolders.DefaultFolderResolver`, `bookmarks` table.
- Produces: Forum bookmark CRUD uses `bookmarks` with `entity_type = forum_thread` / `forum_reply`.

- [ ] **Step 1: Add migration to backfill `forum_bookmarks` into `bookmarks`**

`migrations/000XXX_consolidate_forum_bookmarks.up.sql`:

```sql
INSERT INTO bookmarks (tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at)
SELECT tenant_id, user_id,
       CASE WHEN entity_type = 'thread' THEN 'forum_thread' ELSE 'forum_reply' END,
       entity_id, folder, NULL, created_at
FROM forum_bookmarks
ON CONFLICT (user_id, entity_type, entity_id) DO NOTHING;
```

Down migration: delete rows from `bookmarks` where `entity_type IN ('forum_thread','forum_reply')` and `user_id, entity_id` existed in `forum_bookmarks`. (Best-effort; down migrations are for rollback in development.)

```sql
DELETE FROM bookmarks
WHERE entity_type IN ('forum_thread', 'forum_reply')
  AND (user_id, entity_id) IN (
      SELECT user_id, entity_id FROM forum_bookmarks
      WHERE entity_type IN ('thread', 'reply')
  );
```

- [ ] **Step 2: Add `EntityTypeForumThread` and `EntityTypeForumReply` constants**

In `internal/forum/forum.go`:

```go
const (
    EntityTypeThread = "thread"
    EntityTypeReply  = "reply"
    EntityTypeForumThread = "forum_thread"
    EntityTypeForumReply  = "forum_reply"
)
```

- [ ] **Step 3: Replace `forum_bookmarks` queries with `bookmarks` queries**

In `internal/db/queries/forum.sql`:
- Replace `AddForumBookmark` with an insert into `bookmarks` using `entity_type IN ('forum_thread','forum_reply')` and `folder_id`.
- Replace `RemoveForumBookmark` with `DELETE FROM bookmarks ...`.
- Replace `GetForumBookmarkByID` with `SELECT ... FROM bookmarks ...`.
- Replace `DeleteForumBookmarkByIDForUser` with `DELETE FROM bookmarks ...`.
- Replace `ListForumBookmarksByUser` with `SELECT ... FROM bookmarks WHERE entity_type IN ('forum_thread','forum_reply')`.
- Replace `IsForumBookmarked` with `SELECT EXISTS(...) FROM bookmarks ...`.
- Replace `GetForumBookmarkByEntity` with `SELECT ... FROM bookmarks ...`.

Remove or rename the old query names so sqlc does not generate them.

- [ ] **Step 4: Update `internal/forum/store.go`**

Change methods to use the new query names against `bookmarks`. Keep the same method signatures where possible.

- [ ] **Step 5: Update `internal/forum/service.go:Service`**

Add a `folderResolver folderResolver` field and setter. Update `AddBookmark` to resolve default folder when `folder` is empty and normalize `entity_type` to `forum_thread`/`forum_reply`.

```go
func (s *Service) AddBookmark(ctx context.Context, tenantID, userID pgtype.UUID, entityType string, entityID pgtype.UUID, folder string) (gen.Bookmark, error) {
    normalized := entityType
    if entityType == EntityTypeThread {
        normalized = EntityTypeForumThread
    } else if entityType == EntityTypeReply {
        normalized = EntityTypeForumReply
    }

    if folder == "" && s.folderResolver != nil {
        f, err := s.folderResolver.Resolve(ctx, tenantID, userID, normalized, "")
        if err == nil && f.ID != "" {
            folder = f.Name
        }
    }
    if folder == "" {
        folder = "discussions"
    }

    bm, err := s.store.addBookmark(ctx, gen.AddForumBookmarkParams{
        TenantID:   tenantID,
        UserID:     userID,
        EntityType: normalized,
        EntityID:   entityID,
        Folder:     folder,
    })
    ...
}
```

- [ ] **Step 6: Update `internal/forum/handlers.go`**

`GetThread` calls `GetBookmarkByEntity(..., EntityTypeThread, ...)`. Update to use `EntityTypeForumThread` if the service stores that. The frontend still passes `entity_type: 'thread'` to `POST /forum/bookmarks`; the service normalizes it.

- [ ] **Step 7: Update `ListUserBookmarks` in `internal/db/queries/users.sql`**

Remove the `forum_bookmarks UNION ALL` leg. Extend the single `bookmarks` query to:
- Join `forum_threads ft` for `forum_thread` (already there).
- Join `forum_replies fr` and `forum_threads ft2` for `forum_reply` so we can return the thread's title/slug and the reply's `id`.

Example snippet for the title/slug CASE:

```sql
CASE
    WHEN b.entity_type IN ('content', 'blog_post') THEN c.title
    WHEN b.entity_type = 'forum_thread'            THEN ft.title
    WHEN b.entity_type = 'forum_reply'             THEN ft2.title
    WHEN b.entity_type = 'space'                   THEN s.name
    WHEN b.entity_type = 'tag'                     THEN t.name
    WHEN b.entity_type = 'learning_track'          THEN lt.title
    WHEN b.entity_type = 'doc_page'                THEN dp.title
END AS title,
```

And the slug CASE similarly. Add joins:

```sql
LEFT JOIN forum_replies fr
    ON b.entity_type = 'forum_reply' AND b.entity_id = fr.id
LEFT JOIN forum_threads ft2
    ON b.entity_type = 'forum_reply' AND fr.thread_id = ft2.id
```

- [ ] **Step 8: Regenerate sqlc**

Run: `make sqlc-gen` or `sqlc generate` (project-specific).

- [ ] **Step 9: Test**

In `internal/forum/forum_test.go`:
- Test `POST /forum/bookmarks` with `entity_type = 'thread'` creates a `bookmarks` row with `entity_type = 'forum_thread'` and `folder = 'discussions'`.
- Test `GET /me/bookmarks` returns the forum bookmark with `title` and `slug` from the thread.
- Test deleting a forum bookmark by ID works and uses the `bookmarks` table.

Run: `go test ./internal/forum/... ./internal/users/...`

---

### Task 6: Update `dashboard/bookmarks` UI for `forum_reply`

**Files:**
- Modify: `apps/main/src/routes/(app)/dashboard/bookmarks/+page.svelte`

**Interfaces:**
- Consumes: `Bookmark.entity_type` and `Bookmark.slug` (thread slug) plus reply id.
- Produces: Correct link and label for `forum_reply`.

- [ ] **Step 1: Update `entityUrl`**

```ts
function entityUrl(b: Bookmark): string {
    const idOrSlug = b.slug || b.entity_id;
    switch (b.entity_type) {
        case 'content':
        case 'blog_post':
            return `/blog/${idOrSlug}`;
        case 'forum_thread':
            return `/forum/${idOrSlug}`;
        case 'forum_reply':
            return `/forum/${b.slug}#reply-${b.entity_id}`;
        case 'space':
            return `/spaces/${idOrSlug}`;
        case 'comment':
            return `/content/${b.entity_id}`;
        default:
            return `/content/${b.entity_id}`;
    }
}
```

- [ ] **Step 2: Update `entityLabel`**

For `forum_reply`, prefer the thread title with a reply suffix:

```ts
if (b.entity_type === 'forum_reply') {
    return b.title ? `${b.title} (reply)` : `${t('bookmarks.type.forum_reply', 'Reply')} #${b.entity_id.slice(0, 8)}`;
}
```

- [ ] **Step 3: Add i18n keys**

In `apps/main/src/lib/i18n/en.json` and `fr.json` add:

```json
"bookmarks.type.forum_reply": "Reply",
"bookmarks.type.forum_thread": "Discussion"
```

- [ ] **Step 4: Build/typecheck**

Run: `cd /home/jmobile/websites/japa/japaconnect/apps/main && bun run check`

---

### Task 7: End-to-end verification

**Files:**
- All touched files.

- [ ] **Step 1: Run backend build and vet**

```bash
cd /home/jmobile/websites/japa/japaconnect
go build ./...
go vet ./...
```

- [ ] **Step 2: Run backend tests**

```bash
go test ./internal/bookmarkfolders/... ./internal/content/... ./internal/users/... ./internal/forum/...
```

- [ ] **Step 3: Apply migrations and run a manual smoke test**

```bash
cd /home/jmobile/websites/japa/japaconnect
make migrate-up
# or the project's migration command
```

Start the backend (`make run` / `go run ./cmd/server`) and the main app (`bun run dev`), then:
1. Register a new user; verify `/me/bookmark-folders` returns the default folders.
2. Bookmark a blog post; verify it lands in the `blog` folder.
3. Bookmark a course (learning_track); verify it lands in the `learn` folder.
4. Bookmark a doc page; verify it lands in the `docs` folder.
5. Bookmark a forum thread; verify it lands in the `discussions` folder.
6. Visit `http://localhost:3000/dashboard/bookmarks` and switch folders.

- [ ] **Step 4: Run main app build**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run build
```

---

### Task 8: Commit and close

- [ ] **Step 1: Stage changes and commit**

```bash
git add -A
git commit -m "feat: default bookmark folders by content type

- Resolve default folder from entity/content type.
- Pre-seed default folders at registration and auto-create on first bookmark.
- Consolidate forum bookmarks into the unified bookmarks table.
- Update dashboard to link forum replies correctly.

Ref: docs/superpowers/specs/2026-08-29-default-bookmark-folders-design.md"
```

- [ ] **Step 2: Final user confirmation**

Summarize what changed and confirm `dashboard/bookmarks` works as requested.
