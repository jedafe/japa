# ADR-009: Render-Time Over Save-Time Affiliate Link Processing

**Status:** Accepted

**Context:** Affiliate links, mentions, and content interlinks all face the same underlying problem: referenced entities can change after the content is saved.

**Decision:** Affiliate link processing — and by extension mention/interlink resolution — happens at render time, not at save time. Stored nodes carry entity IDs, not pre-resolved text or URLs.

**Rationale:** If processing happens at save time, a renamed user, moved space, or updated affiliate program terms leaves stale content scattered across the platform with no single point to fix it. Render-time resolution means one update to the source entity fixes every reference to it instantly.

**Consequences:** Slightly more render-time computation per page load, mitigated by Redis-cached rendered HTML (see the Block Editor caching pattern in `ai-memory/skills/`).
