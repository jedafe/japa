# ADR-034: Media System — Two-Step Upload, Library, Modal Picker

**Status:** Accepted

**Context:** The platform needs media upload for user content, profile images, Space headers, and media-rich posts. Requirements: large file support without timing out the API server, virus scanning before persistence, a WordPress-style browsable media library, and a modal picker integrated into TipTap and Page Builder so users can reuse existing uploads without re-uploading.

**Decision:** Two-step upload flow: (1) client requests a presigned MinIO upload URL from the API (server validates permissions, creates a `media` row with `confirmed=false`, returns presigned URL); (2) client uploads bytes directly to MinIO (bypassing the API server entirely, no bandwidth bottleneck); (3) client calls `/api/v1/media/confirm/:id` which moves the file from `temp/` to `media/` bucket prefix and triggers async processing tasks (`media_scan` for ClamAV, `image_processing` for imgproxy). imgproxy HMAC-SHA256 signed URLs are generated server-side on every serialization — never cached in DB — because signed URLs must stay current as transformation parameters change. CDN and S3-compatible backend are admin-configurable via `system_settings` keys, not environment variables, so they can be changed without a redeploy. MP4/GIF uploads gated by `media.video.upload` RBAC permission node; GIFs auto-converted to WebM/MP4 for bandwidth.

**Rationale:** Presigned direct-to-MinIO upload avoids file proxying through the Go server, which would eat bandwidth and memory. ClamAV scan before `confirmed=true` ensures no virus-infected files ever become visible to users. Server-side imgproxy URL signing is required for security (prevents unauthorized transformations); the cost of per-request signing is negligible compared to a DB query. Modal Picker as a shared Svelte store/portal means one component instance, zero duplication.

**Consequences:** The `media_scan` task is blocking for the confirm flow — if ClamAV is unavailable, media confirm fails. ClamAV must be in the Docker Compose setup. The presigned URL has a TTL (15 minutes) — if the client takes longer, the upload fails and they must request a new URL.
