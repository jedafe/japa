# ADR-037: IP Geolocation — MaxMind GeoLite2 Self-Hosted

**Status:** Accepted

**Context:** Multiple platform features need per-request IP geolocation: ad targeting geo-filters, Primary Space suggestion during onboarding, analytics country breakdown, IP blocking CIDR enforcement, and user timezone defaulting. The choice was between a third-party API (ipinfo.io, ipstack) and self-hosting a local database.

**Decision:** MaxMind GeoLite2-City database, self-hosted. The `geoip2-golang` library reads the binary `.mmdb` file directly in-process — no network call, sub-millisecond lookup. Wired into Go request context as middleware from the very first commit — retrofitting geolocation later would require touching every handler that needs it. Weekly database update via `maintenance:update_geoip_db` Asynq task (low tier, Sunday 02:00 UTC) using the MaxMind API with a free license key (`JC_MAXMIND_LICENSE_KEY`). Atomic pointer swap on update — no server restart needed.

**Rationale:** Third-party API adds latency (network hop per request), cost at scale, and a hard external dependency. GeoLite2 is free (attribution required), accurate enough for the platform's use cases (country + city level), and the in-process read is effectively free in CPU terms. Self-hosted is consistent with the platform's self-hosted posture (Valkey, Meilisearch, MinIO, GlitchTip, Honcho, MaxMind).

**Consequences:** The `.mmdb` file is ~60MB and must be included in the Docker image or mounted as a volume. The `JC_MAXMIND_LICENSE_KEY` must be set for weekly updates — without it, the database will grow stale after a few months (IPs are reassigned; accuracy degrades over time).
