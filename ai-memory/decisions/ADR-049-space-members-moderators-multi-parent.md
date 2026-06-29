# ADR-049: Space System Extensions — Membership, Moderators, Multi-Parent, Quota

**Status:** Accepted

**Context:** A feature requirements document from session 5 identified four architectural gaps in the Space system that were present in early planning discussions but never translated into the V1 documentation:

1. **Explicit membership** — `member_count` existed on `spaces` but no table backed it. The follow system was conflated with membership. Moderator assignment rules require distinguishing members from non-members.
2. **Multi-parent spaces** — The original `parent_id` (single FK) cannot represent a community like "Nigeria Workers in Brazil" that legitimately belongs to both the Nigeria and Brazil country spaces. The proposal explicitly requires "one or more primary spaces."
3. **Moderator assignment tiers** — Two classes of moderators with different membership requirements (site-admin-assigned: no membership required; space-owner-assigned: must be a member). This wasn't modelled anywhere.
4. **Space creation quota** — UserGroup-based quantity limits on how many custom spaces a user can create. Required for business-model differentiation (free/silver/gold tiers).

**Decisions made:**

`space_members(space_id, user_id, joined_at, invited_by)` — tracks explicit space membership. Joining creates a row here; following does not. Location selection in onboarding auto-joins all matching hierarchy levels (country + state + city if exists).

`space_parent_spaces(space_id, parent_space_id)` — junction table replacing single `parent_id`. Every custom space must have at least one primary-level (country/state/city) parent; enforced at the API handler. Primary spaces themselves have no rows in this table.

`space_moderators(space_id, user_id, assigned_by, assigned_by_user_id)` — `assigned_by` is 'site_admin' or 'space_owner'. The API handler enforces the membership requirement for the space_owner path. Bot users (`users.is_bot = true`) are valid assignees.

`user_groups.max_spaces_creatable int` — quota per UserGroup (0 = cannot create, -1 = unlimited, N = capped). `spaces.creator_user_id` added to support the count query at creation time. Permission node `spaces.create` gates the feature; quota is an additional check.

`spaces.moderate.assigned` RBAC node — resolved dynamically at runtime from `space_moderators`, no manual grant per space needed. `spaces.moderate.any` — static grant for site-wide moderation.

`content_spaces(content_id, space_id, is_primary)` — multi-space post assignment table (lives in Section 14, referenced here). At least one row with `is_primary = true` required for user-created content. `geo_space_id` on `contents` is kept for query compatibility and set to the primary space at publish time.

`<SpaceCombobox>` — reusable Svelte component (Bits UI Combobox foundation) backing all space selectors across the platform. Shows user's spaces first, Meilisearch search behind it.

**Rationale:** Separating membership from following is a clean architectural choice that matches real-world community semantics — you can follow a public space for its content without being a member. The junction table for parent spaces adds one join to hierarchy queries but is the only correct model for cross-country communities. The moderator tier distinction in the data model (`assigned_by` column) makes the business rule auditable and reversible without ambiguity. The quota as a UserGroup column (rather than a permission-value lookup) keeps it easy to query in a single join at creation time.

**Consequences:** `space_members` must be written on user registration (auto-join from location), on onboarding completion, and when a user joins a space post-registration. The `member_count` denormalized counter on `spaces` is now maintained by DB triggers on `space_members` inserts/deletes rather than estimated from follow counts. Fan-out in Section 32 must query `content_spaces` to determine which spaces to fan out to per post.
