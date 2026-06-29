# ADR-048: Object Storage — Replace MinIO with SeaweedFS

**Status:** Accepted

**Context:** The `minio/minio` GitHub repository was archived on April 25, 2026. The project had been walking away from open-source maintenance since May 2025 (admin console stripped from community edition), stopped publishing Docker images in October 2025, declared maintenance mode in December 2025, and was formally archived in April 2026. MinIO Inc.'s official successor is AIStor, a proprietary commercial product priced out of range for an early-stage platform. The original MinIO AGPL v3 license also presented latent commercial-use concerns that were accepted as acceptable at planning time but are now moot — the project is dead.

A direct replacement must satisfy: S3-compatible API (presigned URLs required for the two-step upload flow in Section 34), self-hostable single-server Docker Compose story for local development, active maintenance with security patch cadence, and a clean permissive license consistent with the project's established license hygiene principles (established by ADR-021, the Redis → Valkey switch).

**Considered:**
- **AIStor (MinIO commercial)** — Rejected. Proprietary, ~$96k/year enterprise pricing. Non-starter for a solo-founder early-stage project.
- **RustFS** — Rejected. Self-described alpha (alpha 99 as of April 2026). Thirteen security advisories in six months. Not production-ready.
- **Garage** — Considered. Rust-based, single binary, AGPL v3, designed for geo-distributed deployments on modest hardware. No advantage over SeaweedFS for a single-region self-hosted deployment; AGPL v3 repeats the licensing concern; actively maintained but smaller community.
- **pgsty/minio fork** — Rejected. Community fork packaging the last known-good MinIO source binary. Downstream of an abandoned upstream; security patch dependency on volunteers.
- **SeaweedFS** — Selected.

**Decision:** Replace `minio` in the Docker Compose stack and all documentation references with **SeaweedFS** (Apache 2.0, Go, `github.com/seaweedfs/seaweedfs`).

- **Local development (Docker Compose):** Use the `weed mini` all-in-one command — single container, single command, S3 endpoint on port 8333, admin UI on port 23646. No separate master/volume/filer processes needed for dev. Drop-in replacement for the existing `minio:` service block.
- **Production:** Run SeaweedFS with Filer configured to use the existing PostgreSQL instance (the `japa` or a dedicated `seaweedfs` logical database) as the metadata store. Master + Volume server + Filer + S3 gateway can all run in a single Docker container on a single VPS with adequate disk. The S3 endpoint URL changes; all application code is unchanged.
- **imgproxy integration:** Unchanged. imgproxy's `s3://bucket/path` source URL format works against any S3-compatible endpoint via the `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `IMGPROXY_USE_S3` / `AWS_S3_ENDPOINT` env vars.
- **Presigned URLs:** SeaweedFS S3 gateway supports `GetObject` and `PutObject` presigned URLs, satisfying the two-step upload flow in Section 34 without code changes.
- **Admin-configurable backend (Section 34):** The existing `system_settings` keys (`media.storage.provider`, `media.storage.endpoint`, etc.) already abstract the storage backend at runtime. No schema changes. The Go storage client is already written against the standard `aws-sdk-go-v2/service/s3` interface.

**Files requiring update:**
- `ai-memory/sections/01-foundation-tech-stack.md` — replace MinIO with SeaweedFS in stack table and Docker Compose reference
- `ai-memory/sections/21-control-panel.md` — update health monitoring reference from "MinIO storage" to "SeaweedFS storage"
- `ai-memory/sections/34-media-system.md` — update Docker Compose service reference and env var examples
- `roadmap/ROADMAP-v1.0.0.md` — update Docker Compose services block (replace `minio:` service)
- `AGENTS.md` — update stack versions table

**License note:** SeaweedFS Apache 2.0 is a strictly better outcome than MinIO AGPL v3. Apache 2.0 imposes no copyleft obligations — consistent with the principle established by ADR-021 (choosing Valkey's BSD license over Redis's SSPL). This replacement is a license hygiene improvement, not just a maintenance-continuity fix.

**Rationale:** SeaweedFS is the most mature actively maintained S3-compatible self-hosted object store as of mid-2026. v4.25 released May 14, 2026 (six weeks before this decision). 32k+ GitHub stars, 2.8k forks, Apache 2.0, written in Go (consistent with the project's primary language). The `weed mini` single-command dev server matches the operational simplicity of the original MinIO deployment. PostgreSQL as the Filer metadata store means no new infrastructure dependency for production. The existing Section 34 storage abstraction means this replacement requires zero application code changes — only Docker Compose configuration and documentation updates.

**Consequences:** Docker Compose `minio:` service replaced by `seaweedfs:` using the `chrislusf/seaweedfs` image with `weed mini`. S3 endpoint shifts from port 9000 to port 8333 (configurable). All `JC_MINIO_*` env vars renamed to `JC_S3_*` for provider-agnosticism (endpoint, access key, secret key, bucket, region). Section 34's S3 client initialization updated to use the new env var names. No application logic changes; no schema changes.
