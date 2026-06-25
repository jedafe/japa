# ADR-017: Grouped Multi-Index Results Over Cross-Index Relevance Merging

**Status:** Accepted

**Context:** Global search spans multiple Meilisearch indexes (posts, forum threads, spaces, users, tags), each with its own internal relevance scoring.

**Decision:** Present results grouped by category (top 2-3 per index, labeled sections) with a "see all" link into a category-specific results page, rather than attempting to merge cross-index scores into one unified ranked list.

**Rationale:** Normalizing relevance scores across indexes with fundamentally different scoring contexts is a genuinely hard problem with no clean solution, and the complexity isn't justified by the marginal UX benefit over a clearly-grouped presentation.

**Consequences:** If a future version wants true unified ranking, that's a deliberate re-architecture decision, not an incremental tweak to this pattern.
