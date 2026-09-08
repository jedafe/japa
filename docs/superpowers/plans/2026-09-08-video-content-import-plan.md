# Video Content Type, Import, and Channels — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` (recommended) or `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `video` as a first-class JapaConnect content type, allow single universal video import from YouTube/TikTok/Reels/etc., and represent external authors as synthetic user channels that can be claimed or taken down.

**Architecture:** A new `internal/video` package owns `video_sources`, `video_import_jobs`, and `video_channel_claims`. It provides a pluggable `VideoProvider` for URL resolution, creates synthetic users on first import, and builds `video` `contents` rows through the existing content service. The `/create` page and feed cards are extended to support the new type. Phase 2 adds bulk YouTube import and the `/studio` dashboard.

**Tech Stack:** Go 1.26+, Chi, sqlc/pgx, Asynq, SvelteKit 2 + Svelte 5 + Bits UI + Tailwind v4, PostgreSQL 18, Meilisearch, Valkey.

## Global Constraints

- Every DB query is sqlc-generated from `internal/db/queries/*.sql`; never hand-edit `internal/db/gen`.
- All new routes are catalogued in `conventions/api-routes-catalog.md`; all new RBAC nodes in `sections/03-rbac.md`; all new Asynq tasks in `sections/11-job-queue-asynq.md`; all new Valkey keys in `conventions/valkey-keys-catalog.md`; all new system settings in `conventions/system-settings-catalog.md`.
- All new migrations are `000630_*` and above; use the next available numbers at implementation time.
- No hardcoded data in SvelteKit `load()` functions (Rule 16).
- No AI attribution in commit messages, comments, or docs.

---

## Phase 0 — Schema & Core Tables

### Task 0.1: Create migrations for `video_sources`, `video_channel_claims`, `video_import_jobs`

**Files:**
- Create: `migrations/000630_video_sources.up.sql`
- Create: `migrations/000630_video_sources.down.sql`
- Create: `migrations/000631_video_channel_claims.up.sql`
- Create: `migrations/000631_video_channel_claims.down.sql`
- Create: `migrations/000632_video_import_jobs.up.sql`
- Create: `migrations/000632_video_import_jobs.down.sql`
- Create: `migrations/000633_video_user_columns.up.sql`
- Create: `migrations/000633_video_user_columns.down.sql`
- Create: `migrations/000634_contents_video_source_id.up.sql`
- Create: `migrations/000634_contents_video_source_id.down.sql`

**Interfaces:**
- Consumes: existing `contents`, `users`, `user_profiles`, `media` tables.
- Produces: `video_sources`, `video_channel_claims`, `video_import_jobs` tables and new columns.

- [ ] **Step 1: Write migration `000630_video_sources.up.sql`**

```sql
-- Ref: .jeos/sections/106-video-content-and-import.md
CREATE TABLE video_sources (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  author_user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_type       text NOT NULL CHECK (source_type IN ('upload','youtube','tiktok','instagram_reel','vimeo','other')),
  source_url        text NOT NULL,
  external_id       text,
  title             text NOT NULL,
  description       text NOT NULL DEFAULT '',
  category          text,
  thumbnail_url     text,
  thumbnail_media_id uuid REFERENCES media(id) ON DELETE SET NULL,
  duration_secs     int,
  status            text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active','takedown_requested','removed')),
  field_values      jsonb NOT NULL DEFAULT '{}',
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, source_type, external_id) WHERE external_id IS NOT NULL,
  UNIQUE (tenant_id, source_url)
);
CREATE INDEX idx_video_sources_author ON video_sources (tenant_id, author_user_id, status, created_at DESC);
```

- [ ] **Step 2: Write migration `000630_video_sources.down.sql`**

```sql
DROP TABLE IF EXISTS video_sources;
```

- [ ] **Step 3: Write migration `000631_video_channel_claims.up.sql` and `000631_video_channel_claims.down.sql`**

Use the schema from §106. Down migration drops the table.

- [ ] **Step 4: Write migration `000632_video_import_jobs.up.sql` and `000632_video_import_jobs.down.sql`**

Use the schema from §106. Down migration drops the table.

- [ ] **Step 5: Write migration `000633_video_user_columns.up.sql`**

```sql
ALTER TABLE users
  ADD COLUMN is_synthetic boolean NOT NULL DEFAULT false,
  ADD COLUMN synthetic_source text;

ALTER TABLE user_profiles
  ADD COLUMN location text,
  ADD COLUMN banner_media_id uuid REFERENCES media(id) ON DELETE SET NULL;
```

- [ ] **Step 6: Write migration `000633_video_user_columns.down.sql`**

```sql
ALTER TABLE user_profiles DROP COLUMN IF EXISTS banner_media_id, DROP COLUMN IF EXISTS location;
ALTER TABLE users DROP COLUMN IF EXISTS is_synthetic, DROP COLUMN IF EXISTS synthetic_source;
```

- [ ] **Step 7: Write migration `000634_contents_video_source_id.up.sql`**

```sql
ALTER TABLE contents
  ADD COLUMN video_source_id uuid REFERENCES video_sources(id) ON DELETE SET NULL;
CREATE INDEX idx_contents_video_source ON contents (video_source_id) WHERE video_source_id IS NOT NULL;
```

- [ ] **Step 8: Write migration `000634_contents_video_source_id.down.sql`**

```sql
DROP INDEX IF EXISTS idx_contents_video_source;
ALTER TABLE contents DROP COLUMN IF EXISTS video_source_id;
```

- [ ] **Step 9: Run `make migrate` and verify no errors**

- [ ] **Step 10: Run `make sqlc-gen` and verify generated code compiles**

- [ ] **Step 11: Commit**

```bash
git add migrations/000630_video_sources.*.sql migrations/000631_video_channel_claims.*.sql migrations/000632_video_import_jobs.*.sql migrations/000633_video_user_columns.*.sql migrations/000634_contents_video_source_id.*.sql
git commit -m "feat: add video sources, claims, import jobs, and synthetic user columns"
```

---

## Phase 1 — Content Type & `/create` Foundation

### Task 1.1: Register `video` as a built-in content type

**Files:**
- Modify: `internal/content/content.go`
- Modify: `apps/main/src/lib/components/create/contentTypeRegistry.ts`
- Modify: `apps/main/src/lib/components/create/contentTypeRegistry.test.ts`
- Create: `migrations/000635_seed_video_content_type.up.sql`
- Create: `migrations/000635_seed_video_content_type.down.sql`

**Interfaces:**
- Consumes: `validContentTypes` map in `content.go`.
- Produces: `TypeVideo = "video"`; `CREATE_ALLOWED_TYPES` includes `video`.

- [ ] **Step 1: Add `TypeVideo` constant in `internal/content/content.go`**

Add the constant at line 105 and add it to `validContentTypes` at line 140.

```go
const (
    TypeBlogPost   = "blog_post"
    ...
    TypeServiceListing = "service_listing"
    // Video content type, §106.
    TypeVideo = "video"
)

var validContentTypes = map[string]bool{
    ...
    TypeServiceListing: true,
    TypeVideo:          true,
}
```

- [ ] **Step 2: Add `video` to the composer registry in `contentTypeRegistry.ts`**

```typescript
export const CREATE_ALLOWED_TYPES = ['blog_post', 'comparison', 'guide', 'story', 'video'] as const;

const COMPOSER_OVERRIDES: Record<string, Partial<Omit<TypeEntry, 'label'>>> = {
    ...
    video: {
        formKind: 'standard',
        supportsSeries: false,
        featureFlag: 'video.enabled'
    }
};
```

- [ ] **Step 3: Update `contentTypeRegistry.test.ts` expected lists**

- [ ] **Step 4: Write `migrations/000635_seed_video_content_type.up.sql`**

```sql
-- Ref: .jeos/sections/106-video-content-and-import.md
INSERT INTO content_type_jsonld_templates (tenant_id, content_type, template, created_at, updated_at)
SELECT id, 'video', '{
  "@context": "https://schema.org",
  "@type": "VideoObject",
  "name": "{{title}}",
  "description": "{{description}}",
  "thumbnailUrl": "{{featured_image_url}}",
  "embedUrl": "{{source_url}}",
  "uploadDate": "{{published_at}}"
}'::jsonb, now(), now()
FROM tenants;

INSERT INTO system_settings (tenant_id, key, value, updated_at)
SELECT id, 'video.enabled', 'false'::jsonb, now() FROM tenants
ON CONFLICT (tenant_id, key) DO NOTHING;
```

- [ ] **Step 5: Write `migrations/000635_seed_video_content_type.down.sql`**

```sql
DELETE FROM content_type_jsonld_templates WHERE content_type = 'video';
DELETE FROM system_settings WHERE key = 'video.enabled';
```

- [ ] **Step 6: Run `go test ./internal/content/...` and `bun test contentTypeRegistry.test.ts`**

### Task 1.2: Add `video_source_id` to sqlc content queries and models

**Files:**
- Modify: `internal/db/queries/contents.sql`
- Modify: `internal/db/gen/contents.sql.go` (regenerated)

**Interfaces:**
- Consumes: `contents` table schema.
- Produces: `CreateContent` and `UpdateContent` queries include `video_source_id`.

- [ ] **Step 1: Update `CreateContentParams` and `UpdateContent` queries in `internal/db/queries/contents.sql` to include `video_source_id`.**

- [ ] **Step 2: Run `make sqlc-gen`.**

### Task 1.3: Create a `VideoProvider` interface and YouTube oEmbed resolver

**Files:**
- Create: `internal/video/provider.go`
- Create: `internal/video/youtube.go`
- Create: `internal/video/provider_test.go`
- Modify: `internal/customfields/embed.go` to share oEmbed/SSRF utilities with `internal/video`

**Interfaces:**
- Consumes: existing `customfields.EmbedResolver` and `linkpreview` SSRF blocklist.
- Produces: `VideoProvider.Resolve(ctx, url) (*VideoMetadata, error)`; `VideoMetadata` struct.

- [ ] **Step 1: Define `VideoMetadata` and `VideoProvider` in `internal/video/provider.go`**

```go
package video

import "context"

type VideoMetadata struct {
    SourceType    string
    SourceURL     string
    ExternalID    string
    Title         string
    Description   string
    Category      string
    ThumbnailURL  string
    DurationSecs  int
    AuthorName    string
    AuthorHandle  string
    AuthorAvatar  string
    AuthorDesc    string
    AuthorLocation string
    ChannelName   string
    ChannelURL    string
    ChannelLogo   string
    ChannelBanner string
}

type VideoProvider interface {
    Resolve(ctx context.Context, url string) (*VideoMetadata, error)
    Platform() string
}

// sanitizeUsername reduces a channel name to a URL-safe string.
func sanitizeUsername(s string) string {
    re := regexp.MustCompile(`[^a-zA-Z0-9_.-]`)
    return strings.ToLower(strings.Trim(re.ReplaceAllString(s, "-"), "-"))
}

// ChannelKey returns a stable, tenant-scoped identifier for the external channel.
// It is used to look up an existing synthetic user or create a new one.
func (m *VideoMetadata) ChannelKey() string {
    return fmt.Sprintf("%s:%s", m.SourceType, sanitizeUsername(m.ChannelName))
}
```

- [ ] **Step 2: Implement `YouTubeProvider` in `internal/video/youtube.go` using oEmbed first, Data API v3 as fallback for category/channel info.**

- [ ] **Step 3: Write a table-driven test in `provider_test.go` using `httptest.Server` to mock oEmbed and YouTube Data API v3.**

- [ ] **Step 4: Run `go test ./internal/video/...`**

### Task 1.4: Add `internal/video` service and handlers

**Files:**
- Create: `internal/db/queries/video_sources.sql`
- Create: `internal/db/queries/video_channel_claims.sql`
- Create: `internal/db/queries/video_import_jobs.sql`
- Create: `internal/video/store.go`
- Create: `internal/video/service.go`
- Create: `internal/video/handlers.go`
- Create: `internal/video/service_test.go`
- Modify: `internal/server/server.go`
- Modify: `internal/content/service.go` and `internal/content/handlers.go`

**Interfaces:**
- Consumes: `video_sources` sqlc queries, `content.Service` for creating posts, `users.Service` for synthetic users, `media.Service` for thumbnail storage.
- Produces: `POST /api/v1/videos/resolve`, `GET /api/v1/u/:username/videos`, `GET /api/v1/me/video-imports`, `POST /api/v1/me/video-imports`.

- [ ] **Step 1: Write sqlc queries in `internal/db/queries/video_sources.sql`, `video_channel_claims.sql`, and `video_import_jobs.sql` for CRUD.**

- [ ] **Step 2: Run `make sqlc-gen`.**

- [ ] **Step 3: Implement `video.Service.ResolveURL(ctx, url)`** that:
  1. Calls `VideoProvider.Resolve(url)`.
  2. Generates `channelKey := metadata.ChannelKey()`.
  3. Looks up synthetic user by `username = channelKey` (or by `synthetic_source` + derived username).
  4. If not found, creates a synthetic user with `is_synthetic = true`, `synthetic_source = metadata.SourceType`, `username = channelKey`, `email = "synthetic-" + channelKey + "@synthetic.local"`, `profile_type = 'regular'`, and a matching `user_profiles` row with `display_name = metadata.ChannelName`, `bio = metadata.AuthorDesc`, `avatar_url = metadata.ChannelLogo`, `location = metadata.AuthorLocation`.
  5. Fetches and stores the thumbnail via `media.Service`, producing a `media_id`.
  6. Creates or updates `video_sources` with `author_user_id = syntheticUser.ID`, `thumbnail_media_id = mediaID`.
  7. Returns `VideoSource` + embeddable TipTap JSON.

- [ ] **Step 4: Implement `video.Service.ListChannelVideos(ctx, username)`** that returns `video_sources` rows for a user profile.

- [ ] **Step 5: Modify `internal/content/service.go` `CreateContent` and `UpdateContent` to accept `video_source_id` and `featured_image_id`.**

- Add `content.ComputeContentHashWithDistinguisher` in `internal/content/slug.go`:

```go
func ComputeContentHashWithDistinguisher(body, videoSourceID string) string {
    normalised := stripMarkdown(body)
    normalised = strings.TrimSpace(normalised)
    normalised = whitespaceRun.ReplaceAllString(normalised, " ")
    normalised = strings.ToLower(normalised)
    if videoSourceID != "" {
        normalised += "|" + videoSourceID
    }
    h := sha256.Sum256([]byte(normalised))
    return hex.EncodeToString(h[:])
}
```

- In `CreateContent`/`UpdateContent`, when `content_type == 'video'` and `video_source_id` is not null, call `ComputeContentHashWithDistinguisher(body, videoSourceID)` instead of `ComputeContentHash`.
- Add a regression test that two `video` posts with identical body text but different `video_source_id` values do not collide.

- [ ] **Step 6: Modify `internal/content/handlers.go` `CreateContent`/`UpdateContent` JSON request DTOs to include `video_source_id` and pass it to the service.**

- [ ] **Step 7: Register handlers in `internal/video/handlers.go` and wire in `internal/server/server.go`.**

```go
// In server.go after content handlers:
videoStore := video.NewStore(database.Writer, database.Reader)
videoService := video.NewService(videoStore, userService, contentService, mediaService, providerRegistry)
videoHandlers := video.NewHandlers(videoService)
videoHandlers.RegisterRoutes(r)
```

- [ ] **Step 8: Write service tests for ResolveURL, synthetic user reuse, and content hash uniqueness.**

- [ ] **Step 9: Run `go test ./internal/video/... ./internal/content/...` and `go build ./...`.**

---

## Phase 2 — Front-end `/create` and Video Block

### Task 2.1: Extend the `/create` composer for `video`

**Files:**
- Create: `apps/main/src/lib/components/create/VideoField.svelte`
- Create: `apps/main/src/lib/tiptap/nodes/VideoNode.ts`
- Modify: `apps/main/src/lib/components/create/CreateShell.svelte`
- Modify: `apps/main/src/lib/components/create/editors/PostEditor.svelte`
- Modify: `apps/main/src/lib/tiptap/extensions.ts`

**Interfaces:**
- Consumes: `POST /api/v1/videos/resolve`, `POST /api/v1/content`, `GET /api/v1/content-types`.
- Produces: A `VideoField` component and a TipTap `video` node.

- [ ] **Step 1: Add a `video` TipTap node in `apps/main/src/lib/tiptap/nodes/VideoNode.ts`**

```typescript
import { Node } from '@tiptap/core';

export const VideoNode = Node.create({
    name: 'video',
    group: 'block',
    atom: true,
    draggable: true,
    addAttributes() {
        return {
            video_source_id: { default: '' },
            source_url: { default: '' },
            source_type: { default: '' },
            embed_html: { default: '' }
        };
    },
    parseHTML() { return [{ tag: 'div[data-video-source-id]' }]; },
    renderHTML({ HTMLAttributes }) {
        return ['div', { 'data-video-source-id': HTMLAttributes.video_source_id, class: 'video-embed' }, 0];
    }
});
```

- [ ] **Step 2: Register `VideoNode` in `apps/main/src/lib/tiptap/extensions.ts`.**

- [ ] **Step 3: Create `apps/main/src/lib/components/create/VideoField.svelte` with "Import URL" and "Upload" tabs.**

- Import URL tab: an input calling `POST /api/v1/videos/resolve`; on success, auto-fill `title`, `description`, `featured_image_id` (from `thumbnail_media_id`), and append a `video` TipTap node to the editor body.
- Upload tab: reuses the existing media upload endpoint, creates a `video_sources` row with `source_type='upload'`, and appends a `video` TipTap node.

- [ ] **Step 4: Update `CreateShell.svelte` to render `VideoField` when the selected type is `video` and pass the current draft to it.**

- [ ] **Step 5: On save, the `POST /api/v1/content` payload includes `content_type='video'`, `video_source_id`, and `featured_image_id`.**

### Task 2.2: Add `video` feed card and single page

**Files:**
- Modify: `apps/main/src/lib/components/feed/FeedCard.svelte`
- Modify: `apps/main/src/routes/(app)/content/[slug]/+page.server.ts`
- Modify: `apps/main/src/routes/(app)/content/[slug]/+page.svelte`

**Interfaces:**
- Consumes: `GET /api/v1/videos/:id` or `GET /api/v1/content/:slug`.
- Produces: Video feed card with play overlay and single video page with embed.

- [ ] **Step 1: Modify `apps/main/src/routes/(app)/content/[slug]/+page.svelte` to render a video embed when `content_type === 'video'`. Use the `video_source` returned in `GET /api/v1/content/{slug}`.**

- [ ] **Step 2: Update `apps/main/src/lib/components/feed/FeedCard.svelte` to show a play button overlay and duration for `video` type.**

- [ ] **Step 3: Update `apps/main/src/routes/(app)/content/[slug]/+page.server.ts` to include `video_source` in the load data.**

---

## Phase 3 — Channel Profile and Follow/Feed

### Task 3.1: Add Videos tab to user profile

**Files:**
- Modify: `apps/main/src/routes/(app)/u/[username]/+page.svelte`
- Modify: `apps/main/src/routes/(app)/u/[username]/+page.server.ts`

**Interfaces:**
- Consumes: `GET /api/v1/u/:username/videos`.
- Produces: A "Videos" tab on public profiles.

- [ ] **Step 1: Add the "Videos" tab to the profile tabs list.**

- [ ] **Step 2: Load videos via `GET /api/v1/u/:username/videos` in `+page.server.ts`.**

- [ ] **Step 3: Render a grid/list of video cards in the tab.**

### Task 3.2: Ensure synthetic users are excluded from real engagement analytics

**Files:**
- Modify: `internal/content/content.go` (content counts)
- Modify: `internal/analytics/*` or `internal/feed/service.go`

**Interfaces:**
- Consumes: `users.is_synthetic`.
- Produces: Feed and analytics queries filter `is_synthetic = false` where appropriate.

- [ ] **Step 1: Add `WHERE u.is_synthetic = false` to feed and analytics queries that should not include synthetic channel content as real engagement.**

- [ ] **Step 2: Run relevant tests.**

---

## Phase 4 — Claim / Takedown

### Task 4.1: Build claim/takedown flow

**Files:**
- Create: `internal/db/queries/video_channel_claims.sql`
- Modify: `internal/video/service.go`
- Modify: `internal/video/handlers.go`
- Create: `apps/main/src/routes/(app)/u/[username]/claim/+page.svelte`
- Create: `migrations/000636_seed_video_claim_form.up.sql`
- Create: `migrations/000636_seed_video_claim_form.down.sql`

**Interfaces:**
- Consumes: `forms` and `form_submissions` (§06), `ownership_claims` pattern (§07).
- Produces: `POST /api/v1/video-channels/:username/claims`, admin review queue.

- [ ] **Step 1: Create a `video-channel-claim` Form Builder form with fields: `evidence_url`, `notes`, `claim_type`.**

- [ ] **Step 2: Implement `POST /api/v1/video-channels/:username/claims` that creates a `video_channel_claims` row with `form_submission_id`.**

- [ ] **Step 3: Implement admin review endpoints `GET /api/v1/admin/video-claims` and `PATCH /api/v1/admin/video-claims/:id`.**

- [ ] **Step 4: On approved claim, reassign `video_sources.author_user_id` from synthetic user to real user and hide/merge the synthetic user.**

- [ ] **Step 5: On approved takedown, set `video_sources.status = 'removed'` and hide embeds.**

---

## Phase 5 — Bulk Import & `/studio` (Phase 2)

### Task 5.1: Bulk YouTube import background task

**Files:**
- Create: `internal/video/bulk.go`
- Create: `internal/video/bulk_test.go`
- Modify: `internal/worker/tasks.go`
- Modify: `internal/worker/handlers.go`
- Modify: `migrations/000632_video_import_jobs.up.sql` (already created in Task 0.1)

**Interfaces:**
- Consumes: YouTube Data API v3, `video_import_jobs` table, `video.Service`.
- Produces: `video:import_bulk` Asynq task.

- [ ] **Step 1: Implement `YouTubeProvider.SearchChannel`, `SearchPlaylist`, and `SearchKeyword` using the Data API v3 client.**

- [ ] **Step 2: Implement `video.Service.StartBulkImport(ctx, req)` that creates a `video_import_jobs` row and enqueues `video:import_bulk`.**

- [ ] **Step 3: Implement the `video:import_bulk` Asynq handler that iterates results, creates `video_sources` + `contents` rows, and updates the job status with counts/errors.**

- [ ] **Step 4: Register the task in `worker/tasks.go` and `worker/handlers.go`.**

### Task 5.2: Build `/studio` bulk import page

**Files:**
- Create: `apps/main/src/routes/(app)/studio/+page.server.ts`
- Create: `apps/main/src/routes/(app)/studio/+page.svelte`
- Create: `apps/main/src/routes/(app)/studio/VideoBulkImport.svelte`

**Interfaces:**
- Consumes: `POST /api/v1/me/video-imports`, `GET /api/v1/me/video-imports/:id`.
- Produces: A `/studio` page with a bulk import form and job status.

- [ ] **Step 1: Create the `/studio` route inside the unified app shell.**

- [ ] **Step 2: Add a bulk import form with source type tabs (Channel / Playlist / Keyword).**

- [ ] **Step 3: Poll the job status and show a summary.**

---

## Phase 6 — AI Summary (Optional / Phase 3)

### Task 6.1: AI-generated video draft

**Files:**
- Modify: `internal/video/service.go`
- Modify: `internal/ai/gateway.go` or use existing `content.generate_draft`.

**Interfaces:**
- Consumes: `video.VideoMetadata`, `ai.AIGateway`.
- Produces: `video:generate_summary` Asynq task.

- [ ] **Step 1: Implement `video.Service.GenerateDraft(ctx, videoSourceID)` that calls the AI Gateway to generate a title and body from metadata/thumbnail.**

- [ ] **Step 2: Store generated draft as `contents` with `ai_generated = true` and `status = 'in_review'` (HITL gate, ADR-088).**

- [ ] **Step 3: Register `video:generate_summary` Asynq task.**

---

## Follow-up — Admin CRUD for `video_sources` and `video_import_jobs`

Per `jc-backend-develop` conventions, every new entity should have a full admin CRUD surface in `apps/admin` and `apps/control` (where applicable). This is intentionally deferred so the initial implementation can focus on user-facing `/create`, `/studio`, and public channel surfaces.

### Tasks to add after core feature is complete

- **Admin DataTable for `video_sources` (`/admin/content/videos`)**
  - List, search, filter by `status`, `source_type`, `author_user_id`.
  - Single-row view with metadata, source URL, thumbnail.
  - `PATCH` to update `status` (`active` → `removed`) without deleting.
  - `DELETE` as a soft remove (set `status = 'removed'`, do not hard-delete per Rule 11).
  - Reuse `DataTable` spec (sort/search/filter/paginate/actions/bulk).

- **Admin DataTable for `video_import_jobs` (`/admin/content/video-imports`)**
  - List, filter by `status`, `source_type`.
  - View job summary, error log, retry from admin.
  - `POST /api/v1/admin/video-imports/:id/retry`.

- **Admin review queue for `video_channel_claims` (`/admin/content/video-claims`)**
  - Already covered in Phase 4.1; this task tracks the `apps/admin` page that uses `GET /api/v1/admin/video-claims` and `PATCH /api/v1/admin/video-claims/:id`.

- **RBAC nodes to seed:**
  - `admin:video_sources:read`
  - `admin:video_sources:manage`
  - `admin:video_import_jobs:read`
  - `admin:video_import_jobs:manage`
  - `admin:video_claims:read`
  - `admin:video_claims:manage`

---

## Phase 7 — Verification & Documentation

### Task 7.1: Update catalogs and run verification

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/sections/11-job-queue-asynq.md`
- Modify: `.jeos/sections/03-rbac.md`

- [ ] **Step 1: Add all new routes to `api-routes-catalog.md`.**

- [ ] **Step 2: Add all new Asynq tasks to `sections/11-job-queue-asynq.md`.**

- [ ] **Step 3: Add new RBAC nodes to `sections/03-rbac.md` and seed them in a migration `000637_seed_video_rbac.up.sql`.**

- [ ] **Step 4: Add system settings to `system-settings-catalog.md` and seed them in `000638_seed_video_settings.up.sql`.**

- [ ] **Step 5: Add new Valkey key patterns (e.g., `jc:video:resolve:{url_hash}` cache) to `valkey-keys-catalog.md`.**

### Task 7.2: Final verification

- [ ] **Step 1: `go build ./... && go test ./...`**
- [ ] **Step 2: `bun run check` in `apps/main`**
- [ ] **Step 3: `golangci-lint run ./...`**
- [ ] **Step 4: Manual smoke test: resolve a YouTube URL, create a video post, view the channel, submit a takedown.**

---

## Self-Review Checklist

- [ ] Schema matches §106 and ADR-117.
- [ ] No TODO or TBD in the plan.
- [ ] Every task ends with a test or verification command.
- [ ] RBAC nodes, Asynq tasks, system settings, routes, and Valkey keys are catalogued.
- [ ] `video` content type is integrated with existing `/create`, feed, and single-page surfaces.
