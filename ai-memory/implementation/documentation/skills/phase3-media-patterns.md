# Phase 3 Media Patterns

**Date:** 2026-06-26
**Phase:** 3
**Purpose:** Implementation patterns learned during Section 34 (Media System), reusable in future sections.

## Pattern: Two-Step Upload (Initiate → Presigned URL → Confirm)

The Media System uses a two-step upload flow that offloads file transfer to
MinIO presigned URLs, avoiding server-side file handling:

1. **Initiate** — Client sends metadata (filename, MIME type, file size).
   Server validates, creates a media row with `confirmed = false`, generates
   a unique storage path, and returns a presigned URL.

2. **Client Upload** — Client uploads the file directly to MinIO using the
   presigned URL. The server never sees the file bytes.

3. **Confirm** — Client calls the confirm endpoint after upload completes.
   Server sets `confirmed = true`.

```go
func (s *Service) InitiateUpload(ctx context.Context, tenantID, uploaderID pgtype.UUID,
    filename, mimeType string, fileSize int64) (UploadResult, error) {
    // Guard: validate inputs early.
    if filename == "" {
        return UploadResult{}, fmt.Errorf("filename is required")
    }
    if !AllowedMIMETypes[mimeType] {
        return UploadResult{}, fmt.Errorf("unsupported MIME type: %s", mimeType)
    }

    // Generate unique storage path under temp/ prefix.
    ext := filepath.Ext(filename)
    storagePath := fmt.Sprintf("%s/%s/%s%s",
        StoragePathTemp, tenantID.String(), uuid.New().String(), ext)

    // Insert media row with confirmed = false.
    m, err := s.store.InsertMedia(ctx, gen.InsertMediaParams{...})
    if err != nil {
        return UploadResult{}, fmt.Errorf("initiating upload: %w", err)
    }

    return UploadResult{
        MediaID:      m.ID.String(),
        StoragePath:  storagePath,
        PresignedURL: presignedURL,
        ExpiresAt:    expiresAt.Format(time.RFC3339),
    }, nil
}
```

The `confirmed` flag ensures only uploaded files appear in queries — the
`GetMediaByID` query filters by `deleted_at IS NULL` but does not filter by
`confirmed`, so confirmed status is checked at the application layer if needed.

**Why not direct upload to server:** Presigned URLs reduce server load, avoid
memory pressure from large files, and leverage MinIO's native upload capabilities.
The server only handles metadata, not file bytes.

## Pattern: imgproxy HMAC-SHA256 Signed URLs

Media URLs are generated server-side on every serialization using imgproxy with
HMAC-SHA256 signing. The signed URL includes a cryptographic signature that
prevents tampering with the URL parameters:

```go
func MediaURL(storageBucket, storagePath string, opts ImageProxyOpts) string {
    key := os.Getenv("JC_IMGPROXY_KEY")
    salt := os.Getenv("JC_IMGPROXY_SALT")
    baseURL := os.Getenv("JC_IMGPROXY_BASE_URL")

    if key == "" || salt == "" || baseURL == "" {
        return ""  // Graceful degradation
    }

    src := "s3://" + storageBucket + "/" + storagePath
    encoded := base64.RawURLEncoding.EncodeToString([]byte(src))

    path := fmt.Sprintf("/rs:%s:%d:%d/plain/%s@%s",
        resizingType, width, height, encoded, format)

    keyBytes := hexDecode(key)
    saltBytes := hexDecode(salt)

    mac := hmac.New(sha256.New, keyBytes)
    mac.Write(append(saltBytes, []byte(path)...))
    sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

    return baseURL + "/" + sig + path
}
```

**Critical rule: NEVER cache imgproxy URLs in the database.** URLs are generated
on every serialization because:
- imgproxy key rotation changes all URLs
- Resizing parameters can change per request
- Cache busting works automatically

**Graceful degradation:** If env vars are missing, `MediaURL` returns an empty
string. The handler returns this empty string to the client, which can fall back
to the original file path or show a placeholder.

## Pattern: Soft Delete via deleted_at Timestamp

Soft delete uses a nullable `deleted_at` timestamp column instead of hard
deletion. This preserves referential integrity and enables "undo delete":

```sql
-- Soft delete
UPDATE media SET deleted_at = now() WHERE id = @id AND tenant_id = @tenant_id;

-- All read queries filter by deleted_at IS NULL
SELECT * FROM media WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;
```

The `SoftDeleteMedia` handler returns success even if the row is already deleted
(idempotent). The bulk delete operation uses the same pattern:

```go
func (s *Store) BulkSoftDeleteMedia(ctx context.Context, tenantID pgtype.UUID,
    mediaIDs []pgtype.UUID) error {
    if len(mediaIDs) == 0 {
        return nil  // Early exit for empty input
    }
    return s.write.BulkSoftDeleteMedia(ctx, gen.BulkSoftDeleteMediaParams{
        TenantID: tenantID,
        MediaIds: mediaIDs,
    })
}
```

**Why soft delete:** Content items may reference media via `header_image_id` or
`featured_image_id`. Hard deletion would break those references. Soft delete
preserves the row while hiding it from normal queries.

## Pattern: MIME Type Validation with Allowlist

MIME types are validated at the service layer using an allowlist map, not at the
database level. This provides descriptive error messages and flexibility:

```go
var AllowedMIMETypes = map[string]bool{
    "image/jpeg":       true,
    "image/png":        true,
    "image/webp":       true,
    "image/gif":        true,
    "image/svg+xml":    true,
    "video/mp4":        true,
    "video/webm":       true,
    "audio/mpeg":       true,
    "audio/ogg":        true,
    "audio/wav":        true,
    "application/pdf":  true,
}

func IsAllowedMIMEType(mimeType string) bool {
    return AllowedMIMETypes[mimeType]
}

func NormalizeMIMEType(mimeType string) string {
    return strings.ToLower(strings.TrimSpace(mimeType))
}
```

**Validation chain:** Input → `NormalizeMIMEType` → `IsAllowedMIMEType` → error
if not in allowlist. The normalize step handles case variations and whitespace
from client-reported MIME types.

**Why not database CHECK constraint:** The allowlist is more flexible — adding
a new MIME type requires only a code change, not a migration. The database only
stores what was validated.

## Pattern: Asynq Task Definition Pattern (Phase 6 Prep)

Task types and payload structs are defined in `tasks.go` even though the
handlers are not yet wired. This creates a clear contract for Phase 6:

```go
const (
    TaskMediaScan       = "media_scan"
    TaskImageProcessing = "image_processing"
    TaskVideoProcessing = "video_processing"
)

type MediaScanPayload struct {
    MediaID string `json:"media_id"`
}

type ImageProcessingPayload struct {
    MediaID string `json:"media_id"`
}

type VideoProcessingPayload struct {
    MediaID   string `json:"media_id"`
    Transcode bool   `json:"transcode"`
}
```

**Why define now:** The task types and payloads establish the interface between
Phase 5 (media upload) and Phase 6 (processing). Defining them early prevents
interface drift and enables parallel development.

## Pattern: Tenant-Scoped Media Queries

Every query includes `tenant_id` in its WHERE clause, resolved from the request
context. This is non-negotiable for multi-tenancy:

```sql
-- Get media by ID (tenant-scoped)
SELECT * FROM media WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- List by uploader (tenant-scoped)
SELECT * FROM media
WHERE tenant_id = @tenant_id AND uploader_user_id = @uploader_user_id AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- Search (tenant-scoped)
SELECT * FROM media
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
  AND (original_filename ILIKE '%' || @query || '%'
       OR alt_text ILIKE '%' || @query || '%'
       OR caption ILIKE '%' || @query || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;
```

The tenant middleware runs before all media handlers, so the tenant ID is always
available. The `requireTenant` helper returns early if the tenant is missing:

```go
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
    t := tenant.FromContext(r.Context())
    if t == nil {
        writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
        return pgtype.UUID{}, false
    }
    return t.ID, true
}
```

## Lesson: Storage Path Generation Separates Temp and Confirmed

The two-phase storage path pattern (`temp/` during upload, `media/` after
confirmation) provides visual separation and enables cleanup of abandoned
uploads:

```go
storagePath := fmt.Sprintf("%s/%s/%s%s",
    StoragePathTemp, tenantID.String(), uuid.New().String(), ext)
// Result: "temp/{tenant_id}/{uuid}.jpg"
```

A background job (deferred) can scan the `temp/` prefix and delete rows where
`confirmed = false` and `created_at` is older than 24 hours. This prevents
orphaned media rows from abandoned uploads.

## Lesson: Input Validation Guard Clauses

The `InitiateUpload` method demonstrates the early-exit pattern for input
validation. Each guard clause catches a specific invalid state with a
descriptive error message:

```go
func (s *Service) InitiateUpload(ctx context.Context, ...) (UploadResult, error) {
    if filename == "" {
        return UploadResult{}, fmt.Errorf("filename is required")
    }
    if mimeType == "" {
        return UploadResult{}, fmt.Errorf("mime_type is required")
    }
    if fileSize <= 0 {
        return UploadResult{}, fmt.Errorf("file_size_bytes must be positive")
    }
    if fileSize > MaxFileSizeBytes {
        return UploadResult{}, fmt.Errorf("file too large: %d bytes exceeds limit of %d bytes",
            fileSize, MaxFileSizeBytes)
    }
    if !AllowedMIMETypes[mimeType] {
        return UploadResult{}, fmt.Errorf("unsupported MIME type: %s", mimeType)
    }
    // ... proceed with valid inputs
}
```

Each guard returns immediately without executing any database operations. This
is the "fail fast" principle — invalid states halt with descriptive errors
before any side effects occur.
