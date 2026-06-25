# ADR-023: File Upload Validation & Malware Scanning

**Status:** Accepted

**Context:** Identified as a gap during the V1.0.0 production-readiness audit — the Media Uploader component existed in the design with no specified validation, size limits, or malware scanning, despite handling user-supplied files across avatars, post images, and content media broadly.

**Decision:** Three checks, all server-side, all before persistence to MinIO:

1. **MIME type verification by content inspection**, never by trusting the client-supplied `Content-Type` header or the file's extension alone.
2. **Per-context size limits** — avatar, post image, and document uploads each get their own ceiling rather than one global limit.
3. **Malware scanning via ClamAV**, self-hosted, run as a new `media_scan` Asynq task (default tier, added to Section 11's catalog) that blocks the file from being persisted to MinIO until the scan clears. A flagged scan rejects the upload and notifies the user rather than silently dropping it.

**Rationale:** Trusting client-reported file type is a well-known bypass vector — renaming a malicious executable's extension or spoofing the Content-Type header defeats extension/header-based checks trivially. Content-based MIME sniffing (checking actual file headers/magic bytes) doesn't have that gap. ClamAV was chosen over a paid scanning API because it's self-hostable and fits the platform's existing self-hosted-infrastructure pattern (same reasoning already applied to Meilisearch and Honcho) rather than adding another external API dependency and per-scan cost.

**Consequences:** Upload latency increases slightly since the scan must complete before the file is considered ready — this is treated as the acceptable tradeoff versus the alternative (persist immediately, scan after the fact, and have a window where infected files are already being served). See the validation pipeline pattern in `ai-memory/skills/file-upload-validation-patterns.md`.
