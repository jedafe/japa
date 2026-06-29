# Section 03 — RBAC (Dynamic Three-Tier)

**Status:** ✅ Locked

## Decided
User Groups → User Roles → Permissions. Fully admin-configurable except three system defaults: `super_admin`, `members`, `guest`. No hardcoded role checks anywhere in the codebase — every access decision resolves through a permission node at runtime. This was explicitly corrected mid-planning when an earlier instinct treated some access (e.g. Page Builder) as fixed-role rather than dynamic-permission.

## See Also
ADR-003. Pattern reused throughout: Page Builder (`content.page_builder.create`), Blog Extension (`blog.*` granular nodes), language pack management (scoped via join table, not a global permission).

## Amendment — AI Context Permission Nodes (post-audit)

New `ai.context.*` node family gates which site-activity categories feed JapaBot's Honcho-backed persistent context per User Group (e.g. `ai.context.points_balance`, `ai.context.course_progress`, `ai.context.mentorship_activity`, `ai.context.forum_activity`, `ai.context.referral_activity`, `ai.context.moderation_history`). All default unassigned/off, including for `members` **and `super_admin`** — every node is granted manually by an admin, with no default-grant exception for any Group, system-default or otherwise. No new schema; this is the existing pattern applied to a new domain. See ADR-025, Section 20 (AI Core), Section 28 (Wallet & Credits).

## Amendment — Group Membership Is Never Points-Automated (post-audit)

Section 28's Points (Karma) total is never a trigger for Group membership changes. All Group assignment, including any future "promote trusted users" workflow, is a manual admin action — there is no automated rule, job, or threshold check anywhere in the codebase that moves a user between Groups based on their Points balance. See ADR-028.

## Amendment — Space Permission Nodes (session 5)

New permission node family for Space management. These are in addition to the general `spaces.*` nodes that may already exist for content moderation:

```
spaces.create               — create a new custom space (quantity gated by user_groups.max_spaces_creatable)
spaces.moderate.assigned    — moderate a specific space the user has been assigned to (via space_moderators)
spaces.moderate.any         — site-wide space moderation; can moderate any space; assigned by admin only
spaces.moderator.assign     — assign moderators to spaces the user owns (space creator only; enforced at handler)
```

`spaces.moderate.assigned` is automatically resolved at runtime by checking `space_moderators` — it is not a static grant in the RBAC table. The RBAC resolution layer checks for this node by querying the `space_moderators` table for the requesting user and the target space, meaning it requires no admin manual grant per space — the assignment record in `space_moderators` is the grant.

`spaces.moderate.any` is a static grant (stored in the permission table like all other nodes), assigned to the `super_admin` group by default and optionally to a dedicated moderation UserGroup.

## Amendment — RBAC Schema (audit pass — tables were missing)

```sql
CREATE TABLE user_groups (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id            uuid NOT NULL REFERENCES tenants(id),
  name                 text NOT NULL,
  slug                 text NOT NULL,
  description          text,
  is_system            boolean NOT NULL DEFAULT false,  -- system groups cannot be deleted
  max_spaces_creatable int NOT NULL DEFAULT 0,          -- 0=none, -1=unlimited, N=capped (Section 05)
  created_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);
-- System defaults seeded: super_admin, members, guest

CREATE TABLE user_roles (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     uuid NOT NULL REFERENCES tenants(id),
  user_group_id uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
  name          text NOT NULL,
  slug          text NOT NULL,
  is_system     boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE(tenant_id, slug)
);

-- Permission nodes are global (not per-tenant) — they're feature capabilities, not data
CREATE TABLE permissions (
  node        text PRIMARY KEY,   -- dot-notation: 'blog.create', 'spaces.moderate.any'
  label       text NOT NULL,
  description text,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
  role_id    uuid NOT NULL REFERENCES user_roles(id) ON DELETE CASCADE,
  permission text NOT NULL REFERENCES permissions(node) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission)
);

-- A user's primary group is set on users.user_group_id.
-- Additional role assignments (beyond the group's roles) are rare but supported:
CREATE TABLE user_role_assignments (
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id    uuid NOT NULL REFERENCES user_roles(id) ON DELETE CASCADE,
  granted_by uuid REFERENCES users(id) ON DELETE SET NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_id)
);
```

**Runtime permission resolution:**
```go
func HasPermission(ctx context.Context, userID uuid.UUID, node string) bool {
    // 1. Get user's primary group → get group's roles → get roles' permissions
    // 2. Check user_role_assignments for additional roles
    // 3. For spaces.moderate.assigned: also check space_moderators table
    // Result cached in Valkey: jc:perm:{user_id} → JSON set of nodes, TTL 5min
    // Invalidated on: group change, role grant/revoke, moderator assignment/removal
}
```

## Amendment — api.keys.self_manage node (session 6)

```
api.keys.self_manage      — user can create/revoke their own API keys (on by default for verified members)
api.keys.view.all         — see all API keys across all users (admin)
api.keys.manage           — suspend, revoke, change tier, create system keys (admin)
api.keys.analytics.view   — view per-key usage analytics (admin)
```

## Amendment — Permission Cache Invalidation (audit pass 2)

The `jc:perm:{user_id}` Valkey key (TTL 5 min) must be explicitly deleted on every event that could change a user's resolved permissions. Without this, stale permissions persist for up to 5 minutes.

**Required invalidation triggers — delete `jc:perm:{user_id}` on:**
```go
// Call InvalidatePermCache(ctx, userID) after any of these mutations:
// 1. users.user_group_id changed (group reassignment)
// 2. user_role_assignments INSERT or DELETE (role granted/revoked)
// 3. space_moderators INSERT or DELETE for this user (moderator status changed)
// 4. role_permissions INSERT or DELETE (role's permission set changed — ALL users in role affected)
//    → for role permission changes: SCAN jc:perm:* and delete all (expensive but rare; admin action)
// 5. user_bans INSERT (ban takes effect immediately — cache must not serve old capabilities)

func InvalidatePermCache(ctx context.Context, v *valkey.Client, userID uuid.UUID) error {
    return v.Del(ctx, "jc:perm:"+userID.String())
}
```

Note on role-wide permission changes: when a permission is added to or removed from a role, all users who hold that role are affected. Rather than enumerating them at write time (potentially thousands of users), prefer the 5-minute TTL to naturally expire — unless the change is security-critical (e.g. removing a dangerous permission node), in which case a `SCAN jc:perm:*` + batch DEL is justified. Document the decision to not immediately invalidate in the admin audit log.
