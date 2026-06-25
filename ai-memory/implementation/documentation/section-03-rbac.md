# Section 03 - RBAC (Dynamic Three-Tier) Implementation Report

**Date:** 2026-06-25  
**Section:** 03 - RBAC  
**Phase:** 1  
**Status:** Complete  

## Overview

Implemented the dynamic three-tier RBAC system for JapaConnect. The model is:
User Groups -> User Roles -> Permissions. Every access decision resolves
through `HasPermission` at runtime. No role name is ever hardcoded in
business logic.

Three system default groups exist and cannot be deleted:
`super_admin`, `members`, `guest`. All other groups are admin-created.

## Files Created (5)

| # | File | Purpose |
|---|------|---------|
| 1 | `migrations/000006_create_rbac_tables.sql` | Database schema for RBAC tables and system group seeds |
| 2 | `internal/rbac/rbac.go` | Core RBAC resolver with HasPermission, per-request cache, UserContext |
| 3 | `internal/rbac/store.go` | CRUD operations for groups, roles, permissions, and mappings |
| 4 | `internal/rbac/rbac_test.go` | Unit tests (4 functions, 10 subtests) |
| 5 | `internal/rbac/middleware.go` | RequirePermission HTTP middleware, FromContext helper |

## Database Schema

Migration `000006_create_rbac_tables.sql` creates six tables:

- **user_groups** - Tenant-scoped groups (e.g. Editors, Moderators). `is_system` flag protects the three defaults. Unique on `(tenant_id, slug)`.
- **user_roles** - Tenant-scoped roles that serve as the intermediate tier. Unique on `(tenant_id, slug)`.
- **permissions** - Global permission nodes (e.g. `blog.publish`). Unique on `key`.
- **role_permissions** - Many-to-many join: roles to permissions.
- **group_roles** - Many-to-many join: groups to roles.
- **user_group_memberships** - Many-to-many join: users to groups. Unique on `(tenant_id, user_id, group_id)`.

All tenant-scoped tables carry `tenant_id` and reference `tenants(id)` with `ON DELETE CASCADE`.

### Seeded Data

Three system default groups are inserted for the default tenant
(`00000000-0000-0000-0000-000000000001`):

| UUID suffix | Name | Slug | is_system |
|-------------|------|------|-----------|
| `...0010` | Super Admin | `super_admin` | true |
| `...0020` | Members | `members` | true |
| `...0030` | Guest | `guest` | true |

## Core API

### rbac.go

- `RBAC` struct holds a `*db.DB` and a per-request `permissionCache`.
- `New(db *db.DB) *RBAC` - constructor.
- `HasPermission(ctx, userID, tenantID, permKey) bool` - canonical access check. Loads UserContext (with caching), returns true if the permission key exists in the user's resolved permission set.
- `GetUserContext(ctx, userID, tenantID) (*UserContext, error)` - loads groups and permissions, caches per user ID string.
- `loadUserContext` - executes a 6-table JOIN query: `user_group_memberships -> user_groups -> group_roles -> user_roles -> role_permissions -> permissions`.
- `ClearCache()` - empties the permission cache (call at end of request or when permissions change).
- `IsSystemGroup(slug) bool` - checks if a slug is one of the three system defaults.
- Constants: `SystemGroupSuperAdmin`, `SystemGroupMembers`, `SystemGroupGuest`.

### store.go

- `Store` struct wraps `*db.DB` for write operations.
- `NewStore(db *DB) *Store` - constructor.
- `CreateGroup(ctx, tenantID, name, slug)` - rejects system group slugs, inserts and returns the new group ID.
- `DeleteGroup(ctx, tenantID, slug)` - rejects system groups, deletes with `is_system = false` guard.
- `AddUserToGroup` / `RemoveUserFromGroup` - membership management with `ON CONFLICT DO NOTHING` for idempotency.
- `RegisterPermission(ctx, key, description)` - upserts a permission node (called at app startup for all known keys).
- `AssignPermissionToRole(ctx, roleID, permKey)` - grants a permission to a role via subquery.
- `AssignRoleToGroup(ctx, groupID, roleID)` - grants a role to a group.

### middleware.go

- `RequirePermission(resolver *RBAC, permKey string)` - middleware factory. Must be used after `RequireAuth`. Returns 401 if no authenticated user, 403 if permission check fails. Stores the resolver in request context via `ResolverKey`.
- `FromContext(ctx) *RBAC` - extracts the resolver from context for use in handlers.

## Test Results

```
=== RUN   TestIsSystemGroup
--- PASS: TestIsSystemGroup (0.00s)
    6 subtests PASS (super_admin, members, guest, custom_group, empty, admin)

=== RUN   TestUserContext_PermissionCheck
--- PASS: TestUserContext_PermissionCheck (0.00s)
    4 subtests PASS (has blog.publish, has blog.edit_own, no blog.delete, no admin.access)

=== RUN   TestRBAC_ClearCache
--- PASS: TestRBAC_ClearCache (0.00s)

=== RUN   TestSystemGroupConstants
--- PASS: TestSystemGroupConstants (0.00s)

PASS
ok  github.com/jedafe/japa/internal/rbac  0.003s
```

## Build & Vet

- `go build ./cmd/api/` - success (no errors)
- `go vet ./...` - clean (no issues)

## Usage Example

```go
// In server setup:
rbacResolver := rbac.New(dbPool)

// Protected route with permission check:
r.With(
    auth.RequireAuth(jwtKeyRotation),
    rbac.RequirePermission(rbacResolver, "blog.publish"),
).Post("/api/v1/posts", createPostHandler)

// In a handler, check additional permissions:
if !rbacResolver.HasPermission(ctx, user.UserID, user.TenantID, "blog.edit_others") {
    // restrict to own posts only
}
```

## Next Steps

Section 03 (RBAC) is complete. The next step in the Phase 1 build order is
Section 42 (Feature Flags), followed by Section 37 (IP Geolocation).
