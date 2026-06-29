# ADR-005: Geo-First Space System

**Status:** Accepted (updated session 5 — multi-parent spaces, explicit membership, moderator model)

**Context:** The platform's core organizing principle is geography (origin country, destination country) intersected with topic. Users need to belong to a geo context that changes when they relocate, while also being able to join topical communities (custom spaces) that cut across geo boundaries.

**Decision:** Primary Space (exactly one per hierarchy level, changeable on relocation) plus unbounded Followed Spaces and Joined Spaces, reusing the existing universal Follow infrastructure (Section 31) for following, with a dedicated `space_members` table for explicit membership.

**Multi-parent spaces (session 5 update):** Custom spaces belong to one or more primary spaces (country/state/city) via the `space_parent_spaces` junction table. The original single `parent_id` column is replaced by this junction table to support spaces like "Nigeria Workers in Brazil" that legitimately belong to two country spaces. Every custom space must have at least one primary space parent — enforced at the API handler, not the DB level (to allow system-created spaces without parents where needed).

**Space membership (session 5 update):** `space_members` table tracks who has explicitly joined a space, separate from following. Following a space does not create a membership row. Joining auto-follows (via Section 31). This separation is required because the moderator assignment rules treat members and non-members differently.

**Space moderators (session 5 update):** `space_moderators` table with `assigned_by` ('site_admin' | 'space_owner'). Site-admin-assigned moderators have no membership requirement. Space-owner-assigned moderators must be space members. Bot/AI users (`users.is_bot = true`) can be assigned as moderators.

**Space creation quota (session 5 update):** `user_groups.max_spaces_creatable` (0 = none, -1 = unlimited, N = capped). Permission node `spaces.create` gates the ability; quota enforced at the handler.

**Rationale:** Reusing Follow infrastructure for Followed Spaces avoids a parallel following system. The explicit membership table is necessary for the moderator membership rule and for accurate member counts without relying on follow counts (a user can follow without joining). The junction table for parent spaces is necessary because real diaspora communities frequently span two countries by definition. The quota system allows business-model differentiation (free = no custom spaces, silver = 3, gold = 10) without hardcoding tier logic in code — it's a per-UserGroup config value.

**Consequences:** Country and state/province Spaces are seeded from an external dataset (ADR-020); city-level Spaces are deliberately not auto-seeded. The `space_parent_spaces` junction table must be consulted for post bubbling rather than a simple `parent_id` walk. Fan-out in Section 32 must handle multiple spaces per post (content_spaces table, Section 14). The `spaces` table keeps a `creator_user_id` column for quota enforcement and ownership checks.
