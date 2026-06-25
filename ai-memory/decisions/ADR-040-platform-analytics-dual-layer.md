# ADR-040: Platform Analytics — Dual-Layer Architecture

**Status:** Accepted

**Context:** Umami self-hosted handles page views well but can't capture platform-specific events (course enrolled, space joined, reaction given, wallet credit earned). A native analytics layer was needed alongside Umami.

**Decision:** Two layers: Umami (self-hosted, cookieless, privacy-first, handles page views/sessions/countries) and a native `platform_events` table (write-optimised `bigint` identity primary key, indexed by tenant + event_type + created_at) for platform-specific events. Materialized views refreshed hourly by `analytics:refresh_views` Asynq task provide aggregated stats without hitting the raw events table on every dashboard load. Security scanner runs as a separate `security:scan` daily task writing to `security_scan_results`. Cache flush operations (Valkey, Cloudflare, DB query cache) are distinct admin-triggered actions logged to the Audit Log.

**Rationale:** Umami handles the browser-level analytics that are expensive to implement from scratch (session stitching, UTM params, bounce detection). The native table handles platform-specific events that Umami's JS snippet can't see (server-side events, background task completions). Materialized views avoid scanning tens of millions of raw event rows on every dashboard request.

**Consequences:** The `platform_events` table will grow large quickly — a partition strategy by `created_at` month is recommended as a V2 optimisation, not required for V1 launch volumes.
