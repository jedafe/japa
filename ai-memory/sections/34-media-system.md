# Section 34 — Media System

**Status:** ✅ Locked

## Upload Flow (Two-Step)

```
1. Client → POST /api/v1/media/upload  (multipart, metadata only on first call)
   Server: validate MIME (content inspection), size check, ClamAV scan (media_scan task)
   Server: store to MinIO at temp/{tenant_id}/{media_id}.{ext}
   Server: return media_id + presigned upload URL

2. Client: upload file bytes to presigned URL (direct to MinIO, no server bandwidth)

3. Client → POST /api/v1/media/confirm/:id
   Server: move temp/{id}.ext → media/{tenant_id}/{id}.{ext}
   Server: trigger image_processing task (resize/optimize via imgproxy)
   Server: mark media row confirmed=true
```

```sql
CREATE TABLE media (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  uploader_user_id uuid NOT NULL REFERENCES users(id),
  original_filename text NOT NULL,
  mime_type       text NOT NULL,
  file_size_bytes bigint NOT NULL,
  width_px        int,
  height_px       int,
  duration_secs   numeric,             -- for video/audio
  storage_path    text NOT NULL,       -- MinIO object key
  storage_bucket  text NOT NULL DEFAULT 'media',
  alt_text        text,
  caption         text,
  confirmed       boolean NOT NULL DEFAULT false,
  deleted_at      timestamptz,         -- soft delete
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_media_uploader ON media (tenant_id, uploader_user_id, created_at DESC);
CREATE INDEX idx_media_type ON media (tenant_id, mime_type, confirmed, deleted_at);
```

## imgproxy URL Generation

```go
// imgproxy HMAC-SHA256 signed URL — generated server-side on every serialization
// NEVER cached in DB (image transformations change; signed URLs must stay current)
func MediaURL(m *Media, opts imgproxyOpts) string {
  key := os.Getenv("JC_IMGPROXY_KEY")
  salt := os.Getenv("JC_IMGPROXY_SALT")
  src := "s3://" + m.StorageBucket + "/" + m.StoragePath
  path := fmt.Sprintf("/rs:%s:%d:%d/plain/%s@%s",
    opts.ResizingType, opts.Width, opts.Height,
    base64.URLEncoding.EncodeToString([]byte(src)),
    opts.Format)
  mac := hmac.New(sha256.New, hexDecode(key))
  mac.Write(append(hexDecode(salt), []byte(path)...))
  sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
  return imgproxyBaseURL + "/" + sig + path
}
```

## Video & GIF Support

- MP4 and GIF uploads gated by `media.video.upload` RBAC permission node
- `ffmpeg` processing via `video_processing` Asynq task (default tier): generate thumbnail frame at 5s, extract duration, transcode to web-optimized H.264 if > 50MB
- Video served via imgproxy streaming or direct MinIO presigned URL depending on size
- GIFs auto-converted to WebM/MP4 for bandwidth efficiency (GIF served as fallback for email)

## Media Library (WordPress-style)

A dedicated page at `/dashboard/media` in the main site frontend:

```
Layout: switchable grid (default) / list view
Filters: type (image|video|document|gif), date range, uploader (admin sees all; user sees own)
Search: by original_filename, alt_text (full-text, Meilisearch indexed)
Pagination: cursor-based, 48 items per page, no infinite scroll
Actions per item: edit alt_text/caption, delete (soft), copy URL, view details panel
Bulk actions: delete (with confirm), assign to folder (future)
RBAC: media.library.view_own | media.library.view_all
```

## Modal Picker (TipTap + Page Builder Integration)

When a user clicks "Insert Image/Media" in TipTap or Page Builder, a modal opens over the current editor surface without navigating away:

```ts
// SvelteKit component: MediaPickerModal.svelte
// Props: allowedTypes (MIME filter), onSelect (callback), multiple (bool)
// State: same query/filter state as the /dashboard/media page
// UX: search bar at top, grid of thumbnails, click to select, "Upload New" tab
// On confirm: returns media[] to the TipTap node or Page Builder block
export let allowedTypes: string[] = ['image/*'];
export let multiple: boolean = false;
export let onSelect: (media: Media[]) => void;
```

The modal is a shared Svelte store/portal — opening it from any editor context uses the same component instance.

## CDN & S3-Compatible Object Storage

Admin-configurable in Control Panel under Settings > Media:
- Storage backend: MinIO (default self-hosted) or any S3-compatible provider (AWS S3, Cloudflare R2, Backblaze B2) — configured via `system_settings` keys `media.storage.provider`, `media.storage.endpoint`, `media.storage.bucket`, `media.storage.region`, `media.storage.access_key`, `media.storage.secret_key`
- CDN prefix: optional `media.cdn.base_url` — if set, all `MediaURL()` calls use this prefix instead of imgproxy's own hostname; used to front imgproxy with Cloudflare

```go
func MediaBaseURL() string {
  if cdnURL := getSystemSetting("media.cdn.base_url"); cdnURL != "" {
    return cdnURL
  }
  return os.Getenv("JC_IMGPROXY_BASE_URL")
}
```

## Asynq Tasks Added

- `media_scan` (default, blocking) — ClamAV ICAP scan before confirm; rejects on virus detection
- `image_processing` (default) — resize, optimize, generate thumbnails via imgproxy
- `video_processing` (default) — ffmpeg thumbnail + transcode for uploaded videos

## See Also

Section 01 (MinIO + imgproxy stack entries), Section 15 (TipTap media insertion), Section 16 (Page Builder media blocks), ADR-023 (file upload validation). ADR-034.
