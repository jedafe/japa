# ADR-031: Social Layer — Unified Follow System

**Status:** Accepted

**Context:** Users needed to follow other users, Spaces, Tags, Organizations, and Professional Profiles. The initial instinct was separate tables (`user_follows`, `space_follows`, `tag_follows`). This creates duplicated fan-out logic and N separate queries to check "is this user following any of these entity types."

**Decision:** One polymorphic `follows (follower_id, entity_type, entity_id)` table with a UNIQUE constraint on `(follower_id, entity_type, entity_id)`. All follower counts are maintained as denormalized columns on the target entity, incremented/decremented via DB triggers. Joining a Space auto-follows it; following doesn't require joining. Mutual follow (A→B + B→A) is a connection — detected by a check query, not a separate `connections` table.

**Rationale:** A single table means a single fan-out task, a single API pattern (`POST /api/v1/follow`), and a single "is following" batch query that works for any entity type. The polymorphic pattern already exists in `reactions`, `bookmarks`, and `review_aggregates` — consistency across the platform.

**Consequences:** All activity feed fan-out (ADR-032) reads from the single `follows` table. DM request gating (Section 10) checks mutual follow via the same table. Discovery pages (Section 23) need "is_following" resolved in a bulk query, not N+1 per item.
