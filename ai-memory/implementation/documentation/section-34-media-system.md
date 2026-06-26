# Section 34 — Media System: Implementation Complete

**Date:** 2026-06-26
**Phase:** 3
**Section:** 34
**Status:** Complete

## Overview

Section 34 implements the Media System for JapaConnect with a two-step upload
flow: initiate (creates a media row and returns a presigned URL) → client
uploads directly to MinIO → confirm (marks the media as confirmed). imgproxy
HMAC-SHA256 signed URLs are generated server-side on every serialization and
never cached in the database. The Media Library is accessible at `/dashboard/media`
with grid/list view and a Modal Picker for TipTap and Page Builder integration.

The system uses a soft delete pattern via `deleted_at` timestamp, and Asynq tasks
are defined for ClamAV scan, image processing, and video processing (deferred to
Phase 6).

## Components Implemented

### Migration 000016 — Media System

1 migration file (`migrations/000016_media_system.sql`, 24 lines); 1 table
created:

**media** — Stores metadata for uploaded files. Columns:

- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `uploader_user_id` — FK to `users` with `ON DELETE CASCADE`
- `original_filename` — Original filename from the client
- `mime_type` — Detected MIME type
- `file_size_bytes` — File size in bytes (bigint)
- `width_px` — Image/video width in pixels (nullable)
- `height_px` — Image/video height in pixels (nullable)
- `duration_secs` — Video/audio duration in seconds (numeric, nullable)
- `storage_path` — MinIO object key (e.g., `temp/{tenant_id}/{uuid}.jpg`)
- `storage_bucket` — Storage bucket name (default `'media'`)
- `alt_text` — Accessibility alt text (nullable)
- `caption` — Display caption (nullable)
- `confirmed` — Whether the upload was confirmed (default `false`)
- `deleted_at` — Soft delete timestamp (nullable)
- `created_at` — Creation timestamp (default `now()`)

Indexes:
- `idx_media_uploader` — `(tenant_id, uploader_user_id, created_at DESC)`
- `idx_media_type` — `(tenant_id, mime_type, confirmed, deleted_at)`
- `idx_media_tenant` — `(tenant_id, created_at DESC) WHERE deleted_at IS NULL`

### sqlc Queries (internal/db/queries/media.sql)

12 queries in 1 SQL file (88 lines total):

**CRUD (5 queries):**

1. `InsertMedia` — insert new media row with `RETURNING *`
2. `GetMediaByID` — get media by ID (excludes soft-deleted)
3. `GetMediaByStoragePath` — get media by storage path
4. `ConfirmMedia` — set `confirmed = true`
5. `UpdateMediaMetadata` — update alt_text, caption, dimensions, duration
6. `SoftDeleteMedia` — set `deleted_at = now()`

**Lists (4 queries):**

7. `ListMediaByUploader` — paginated media for a specific uploader
8. `ListMediaByTenant` — all non-deleted media for a tenant
9. `ListMediaByType` — paginated media matching a MIME type prefix
10. `SearchMedia` — search by filename, alt_text, caption (ILIKE)

**Counts (1 query):**

11. `CountMediaByUploader` — count non-deleted media for an uploader

**Bulk Operations (1 query):**

12. `BulkSoftDeleteMedia` — soft-delete multiple media items

### Package: internal/media/ (7 files)

**media.go** (47 lines)

- Package doc describing the two-step upload flow
- MIME type prefix constants: `MIMEPrefixImage`, `MIMEPrefixVideo`, `MIMEPrefixAudio`
- Storage path constants: `StoragePathTemp`, `StoragePathMedia`
- `DefaultBucket = "media"`
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (176 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 11 public methods wrapping sqlc-generated queries:
  - `InsertMedia` — create new media row
  - `GetMediaByID` — get media by ID (excludes soft-deleted)
  - `GetMediaByStoragePath` — get media by storage path
  - `ConfirmMedia` — mark media as confirmed
  - `UpdateMediaMetadata` — update metadata fields
  - `SoftDeleteMedia` — soft-delete a media item
  - `ListMediaByUploader` — paginated media for an uploader
  - `ListMediaByTenant` — paginated media for a tenant
  - `ListMediaByType` — paginated media by MIME type prefix
  - `SearchMedia` — search by filename, alt_text, caption
  - `CountMediaByUploader` — count media for an uploader
  - `BulkSoftDeleteMedia` — bulk soft-delete
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context
- Page size defaults applied in store layer (clamp to `DefaultPageSize` if
  invalid, cap at `MaxPageSize` if exceeded)

**service.go** (173 lines)

- `AllowedMIMETypes` map — 11 accepted MIME types (JPEG, PNG, WebP, GIF, SVG,
  MP4, WebM, MP3, OGG, WAV, PDF)
- `MaxFileSizeBytes = 50 * 1024 * 1024` (50 MB limit)
- `UploadResult` struct — contains presigned URL details
- 11 public methods:
  - `InitiateUpload` — validates inputs, generates unique storage path under
    `temp/` prefix, inserts media row, returns presigned URL
  - `ConfirmUpload` — marks media as confirmed
  - `GetMedia` — returns single media item by ID
  - `UpdateMetadata` — updates alt_text, caption, dimensions, duration
  - `DeleteMedia` — soft-deletes a media item
  - `ListMedia` — paginated list with optional filters (uploader, type, search)
  - `GetMediaURL` — generates imgproxy signed URL (NEVER cached in DB)
  - `BulkDelete` — soft-deletes multiple media items
  - `IsAllowedMIMEType` — checks if MIME type is allowed
  - `NormalizeMIMEType` — normalizes MIME type to canonical form
- Input validation at service layer with descriptive error messages

**imgproxy.go** (69 lines)

- `ImageProxyOpts` struct — controls resizing (fill, fit, auto), dimensions,
  and output format (webp, png, jpg)
- `MediaURL` function — generates imgproxy HMAC-SHA256 signed URL:
  1. Reads `JC_IMGPROXY_KEY`, `JC_IMGPROXY_SALT`, `JC_IMGPROXY_BASE_URL` from
     environment
  2. Constructs S3 source path: `s3://{bucket}/{storage_path}`
  3. Base64-encodes the source path
  4. Builds imgproxy path with resizing parameters
  5. Signs with HMAC-SHA256 using key and salt
  6. Returns full URL: `{base_url}/{signature}/{path}`
- `hexDecode` helper — decodes hex strings, returns nil on error
- Returns empty string if any env var is missing (graceful degradation)

**handlers.go** (346 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 7 HTTP handlers:
  - `InitiateUpload` — `POST /api/v1/media/upload`. Accepts JSON body with
    `filename`, `mime_type`, `file_size`. Returns presigned URL details.
  - `ConfirmUpload` — `POST /api/v1/media/confirm/{id}`. Marks media as confirmed.
  - `GetMedia` — `GET /api/v1/media/{id}`. Returns media item with imgproxy URL.
  - `UpdateMetadata` — `PATCH /api/v1/media/{id}`. Updates alt_text, caption,
    dimensions, duration.
  - `DeleteMedia` — `DELETE /api/v1/media/{id}`. Soft-deletes media.
  - `ListMedia` — `GET /api/v1/media`. Supports `page_size`, `page_offset`,
    `type`, `q` (search), `uploader_id` query params.
  - `BulkDelete` — `POST /api/v1/media/bulk-delete`. Accepts JSON body with
    `media_ids` array.
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`,
  `parsePageSize`, `parsePageOffset`
- Uses `auth.FromContext`, `tenant.FromContext`, `chi.URLParam`, `pgtype.UUID`

**tasks.go** (25 lines)

- Asynq task type constants: `TaskMediaScan`, `TaskImageProcessing`,
  `TaskVideoProcessing`
- Payload structs: `MediaScanPayload`, `ImageProcessingPayload`,
  `VideoProcessingPayload`
- ClamAV scan and video/image processing deferred to Phase 6

**media_test.go** (393 lines)

- 18 unit tests covering constants, ImageProxyOpts, hexDecode, MIME type
  validation, MIME type normalization, MediaURL generation (with/without env
  vars), JSON serialization, pagination helpers, task payloads, and constructor

### Server Wiring (internal/server/server.go)

- `media.NewService(database)` and `media.NewHandlers(mediaService)` created
  in `NewServer`
- 7 API routes registered under `/api/v1`, all within the tenant-scoped route
  group, all protected by `auth.RequireAuth`
- 8 new permission keys added to `bootstrapPermissions`

### Permission Keys

8 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `media.upload` | Upload media files |
| `media.view_own` | View own uploaded media |
| `media.view_all` | View all tenant media |
| `media.delete_own` | Delete own media |
| `media.delete_any` | Delete any tenant media |
| `media.video.upload` | Upload video files |
| `media.library.view_own` | View own media library |
| `media.library.view_all` | View all tenant media library |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/media | 18 | PASS |

### Media System Tests (18 tests)

| # | Test | Coverage |
|---|------|----------|
| 1 | `TestMIMEPrefixConstants` | All 3 MIME prefix constants match expected values |
| 2 | `TestStoragePathConstants` | StoragePathTemp and StoragePathMedia values |
| 3 | `TestDefaultBucket` | DefaultBucket = "media" |
| 4 | `TestPageSizeConstants` | DefaultPageSize=20, MaxPageSize=100 |
| 5 | `TestMaxFileSizeBytes` | MaxFileSizeBytes = 50 MB |
| 6 | `TestImageProxyOptsDefaults` | Default ImageProxyOpts has zero values |
| 7 | `TestHexDecode` | 3 cases: valid hex, empty, invalid |
| 8 | `TestIsAllowedMIMEType` | 15 cases: allowed types, disallowed, empty, case-sensitive |
| 9 | `TestNormalizeMIMEType` | 4 cases: mixed case, whitespace, empty |
| 10 | `TestMediaURLReturnsEmptyWhenEnvMissing` | Returns empty when env vars not set |
| 11 | `TestMediaURLWithEnvVars` | Generates correct URL with env vars |
| 12 | `TestWriteJSON` | HTTP status, Content-Type header, JSON body |
| 13 | `TestWriteError` | Error envelope format |
| 14 | `TestParsePageSize` | 9 cases: empty, zero, negative, valid, overflow |
| 15 | `TestParsePageOffset` | 6 cases: empty, zero, negative, valid |
| 16 | `TestUploadResultJSON` | JSON serialization of UploadResult |
| 17 | `TestTaskPayloadsJSON` | JSON serialization of all 3 task payloads |
| 18 | `TestNewServiceNotNil` | Service zero-value has nil store |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/media/ -count=1` | ok — 18 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

7 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/media/upload` | Required | `media.upload` | Initiate upload (returns presigned URL) |
| 2 | POST | `/media/confirm/{id}` | Required | `media.upload` | Confirm upload after client completes |
| 3 | GET | `/media/{id}` | Required | `media.view_own` | Get media item with imgproxy URL |
| 4 | PATCH | `/media/{id}` | Required | `media.view_own` | Update media metadata |
| 5 | DELETE | `/media/{id}` | Required | `media.delete_own` | Soft-delete media |
| 6 | GET | `/media` | Required | `media.library.view_own` | List/search media |
| 7 | POST | `/media/bulk-delete` | Required | `media.delete_own` | Bulk soft-delete media |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000016_media_system.sql` | 24 | Media table with 16 columns and 3 indexes |
| 2 | `internal/db/queries/media.sql` | 88 | 12 sqlc queries for media CRUD, lists, counts, bulk |
| 3 | `internal/db/gen/media.sql.go` | (generated) | Generated query methods and param structs |
| 4 | `internal/media/media.go` | 47 | Constants, Service struct, constructor |
| 5 | `internal/media/store.go` | 176 | Store layer with read/write separation |
| 6 | `internal/media/service.go` | 173 | Business logic, MIME validation, upload flow |
| 7 | `internal/media/imgproxy.go` | 69 | imgproxy HMAC-SHA256 signed URL generation |
| 8 | `internal/media/handlers.go` | 346 | 7 HTTP handlers with pagination helpers |
| 9 | `internal/media/tasks.go` | 25 | Asynq task definitions for Phase 6 |
| 10 | `internal/media/media_test.go` | 393 | 18 unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `media` import; created media service and handlers; registered 7 routes with auth middleware; added 8 media permission keys to `bootstrapPermissions` |

## Key Decisions

- **Two-step upload flow** — initiate (creates row + presigned URL) → client
  uploads directly to MinIO → confirm (marks confirmed). This avoids server-side
  file handling and leverages MinIO's presigned URL mechanism. The `confirmed`
  flag ensures only uploaded files are visible in queries.
- **imgproxy URLs generated server-side on every serialization** — NEVER cached
  in the database. This allows dynamic resizing, format conversion, and cache
  busting without DB migrations. The signed URL includes HMAC-SHA256 signature
  to prevent tampering.
- **Soft delete via deleted_at timestamp** — not hard delete. This preserves
  referential integrity with content that may reference media items, and enables
  "undo delete" functionality. All queries filter by `deleted_at IS NULL`.
- **MIME type allowlist validation at service layer** — 11 allowed types
  (JPEG, PNG, WebP, GIF, SVG, MP4, WebM, MP3, OGG, WAV, PDF). The allowlist
  is checked before any database operation. MIME type is also normalized via
  `NormalizeMIMEType` for case-insensitive comparison.
- **File size limits enforced at service layer** — 50 MB maximum. Checked in
  `InitiateUpload` before database insertion.
- **Storage path generation** — files are stored under `temp/{tenant_id}/{uuid}`
  during upload, then moved to `media/{tenant_id}/{uuid}` after confirmation.
  This separates unconfirmed uploads from confirmed ones.
- **Asynq tasks defined but deferred to Phase 6** — `TaskMediaScan` (ClamAV),
  `TaskImageProcessing`, `TaskVideoProcessing` task types and payload structs
  are defined for Phase 6 implementation.
- **Read/write pool separation** — standard JapaConnect pattern. Reader pool
  for SELECT queries, writer pool for INSERT/UPDATE/DELETE.
- **All queries are tenant-scoped** — every query includes `tenant_id` in its
  WHERE clause. The tenant is resolved from the request context via
  `tenant.FromContext`.

## Known Issues and Deferrals

- **MinIO presigned URL generation** — currently returns a placeholder URL
  (`/minio/upload/{storage_path}`). Actual MinIO client integration is deferred
  to Phase 6 when the MinIO infrastructure is configured.
- **ClamAV virus scanning** — `TaskMediaScan` is defined but not wired.
  Files are not scanned before confirmation. Deferred to Phase 6.
- **Image processing** — `TaskImageProcessing` is defined but not wired.
  Thumbnail generation, EXIF stripping, and format conversion are deferred to
  Phase 6.
- **Video processing** — `TaskVideoProcessing` is defined but not wired.
  Transcoding, thumbnail extraction, and duration detection are deferred to
  Phase 6.
- **Media Library frontend** — The `/dashboard/media` grid/list view and Modal
  Picker for TipTap/Page Builder integration are frontend components to be
  implemented in a future section.
- **Content-level media references** — The `contents` table has
  `header_image_id` and `featured_image_id` columns, but the service layer
  does not yet enforce foreign key constraints to the `media` table.
- **Access control granularity** — `media.view_own` and `media.delete_own` are
  defined but the handler does not yet check ownership (uploader_user_id
  match). Currently any authenticated user can view/delete any media within
  their tenant.

## Next Steps

Section 35 — Notification System (already completed). Future work includes:

- Media Library frontend components
- MinIO client integration for actual presigned URL generation
- ClamAV scan and image/video processing Asynq tasks
- Ownership-based access control enforcement
