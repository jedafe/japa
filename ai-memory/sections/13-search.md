# Section 13 — Search (Meilisearch)

**Status:** ✅ Locked

## Decided
Indexes for posts, forum_threads, spaces, users, tags — schema per entity covering searchable/filterable/sortable fields, including a privacy-respecting exclusion for users who've opted out of discoverability. Two-tier sync: immediate re-index on content changes, batched partial updates (~5 min) for engagement score changes. Grouped multi-index results (top 2-3 per category) rather than cross-index relevance merging. Search analytics logged to a lightweight Postgres table (not forced through Umami) for zero-result query detection and trending searches.

## See Also
ADR-017.
