# Deferred Route Reconciliation — Implementation Plan

**Date:** 2026-07-09
**Scope:** 4 deferred items from route catalog reconciliation
**Current diff:** MATCH=627, CATALOG_ONLY=219, CODE_ONLY=1

---

## Task 1: Public Page-Templates Routes (Low complexity)

Wire existing `ListTemplates`/`GetTemplate` service methods as public routes (no auth).

### Files
- `internal/server/server.go` — add 2 routes outside admin group

### Steps
1. After the existing `r.Get("/pages/{slug}", ...)` line (~1886), add:
   ```go
   r.Get("/page-templates", pagebuilderHandlers.ListTemplates)
   r.Get("/page-templates/{id}", pagebuilderHandlers.GetTemplate)
   ```
2. `go build ./internal/server/` — confirm PASS
3. `python3 scripts/route_diff.py` — verify these move from CATALOG_ONLY to MATCH

### Notes
- `ListTemplates` and `GetTemplate` already extract tenantID from context via `requireTenant` — no auth needed for reads
- These are read-only public views of templates (no create/update/delete)

---

## Task 2: `/site/settings/public` Route (Medium — schema migration)

### 2.1: Schema Migration

Add `is_public` boolean column to `system_settings`.

**File:** `migrations/000113_system_settings_is_public.sql`

```sql
ALTER TABLE system_settings ADD COLUMN is_public boolean NOT NULL DEFAULT false;
CREATE INDEX idx_system_settings_is_public ON system_settings(tenant_id, is_public) WHERE is_public = true;
```

**File:** `migrations/000113_system_settings_is_public.down.sql`

```sql
DROP INDEX IF EXISTS idx_system_settings_is_public;
ALTER TABLE system_settings DROP COLUMN IF EXISTS is_public;
```

### 2.2: Query Method

**File:** `internal/config/settings.go`

Add `GetPublicSettings` method:
```go
func (s *Settings) GetPublicSettings(ctx context.Context, tenantID pgtype.UUID) (map[string]any, error) {
    rows, err := s.db.Reader.Query(ctx,
        `SELECT key, value FROM system_settings WHERE tenant_id = $1 AND is_public = true`,
        tenantID,
    )
    // ... scan into map[string]any and return
}
```

### 2.3: Handler

**File:** `internal/config/settings.go` (or a new `handlers.go` in config package)

Add HTTP handler that calls `GetPublicSettings` and returns JSON.

### 2.4: Route Wiring

**File:** `internal/server/server.go`

```go
r.Get("/site/settings/public", configHandlers.GetPublicSettings)
```

Place this near the existing health/ready routes (public, no auth).

### 2.5: Catalog Update

The catalog already documents `GET /site/settings/public`. No catalog change needed.

### Steps
1. Write migration files
2. Add `GetPublicSettings` to settings.go
3. Create handler (either in config package or inline in server.go)
4. Wire route
5. `go build ./internal/server/` — confirm PASS
6. Run migration against test DB
7. `python3 scripts/route_diff.py` — verify MATCH

---

## Task 3: Admin API Keys → Control Panel Move + Full Control Panel (Medium)

### 3.1: Bootstrap Control Panel Route Group

**File:** `internal/server/server.go`

After the existing admin routes group, add a new `/control` route group:

```go
r.Route("/control", func(r chi.Router) {
    r.Use(auth.RequireAuth(kr, vkClient.Redis()))
    r.Use(rbac.RequirePermission(rbacResolver, "control.panel.access"))

    // Tenants
    r.Get("/tenants", controlHandlers.ListTenants)
    r.Post("/tenants", controlHandlers.CreateTenant)
    r.Get("/tenants/{id}", controlHandlers.GetTenant)
    r.Patch("/tenants/{id}", controlHandlers.UpdateTenant)
    r.Delete("/tenants/{id}", controlHandlers.DeleteTenant)
    r.Post("/tenants/{id}/suspend", controlHandlers.SuspendTenant)
    r.Post("/tenants/{id}/unsuspend", controlHandlers.UnsuspendTenant)

    // System Settings
    r.Get("/system-settings", controlHandlers.ListSystemSettings)
    r.Patch("/system-settings", controlHandlers.UpdateSystemSettings)
    r.Patch("/system-settings/{key}", controlHandlers.UpdateSystemSetting)
    r.Post("/system-settings/{key}/reset", controlHandlers.ResetSystemSetting)

    // Feature Flags
    r.Get("/feature-flags", controlHandlers.ListFeatureFlags)
    r.Patch("/feature-flags/{flag_name}", controlHandlers.SetFeatureFlag)

    // Infrastructure Health
    r.Get("/health", controlHandlers.HealthCheck)
    r.Get("/health/db", controlHandlers.DBHealthCheck)
    r.Post("/health/db/vacuum", controlHandlers.DBVacuum)

    // Queue Monitor
    r.Get("/queue/stats", controlHandlers.QueueStats)
    r.Get("/queue/dlq", controlHandlers.ListDLQ)
    r.Post("/queue/dlq/{task_id}/retry", controlHandlers.RetryDLQTask)
    r.Delete("/queue/dlq/{task_id}", controlHandlers.DiscardDLQTask)
    r.Post("/queue/dlq/discard-all", controlHandlers.DiscardAllDLQ)

    // Cache Management
    r.Get("/cache/stats", controlHandlers.CacheStats)
    r.Post("/cache/flush", controlHandlers.FlushCache)

    // Security
    r.Get("/security", controlHandlers.SecurityOverview)
    r.Get("/security/ip-blocks", controlHandlers.ListIPBlocks)
    r.Post("/security/ip-blocks", controlHandlers.CreateIPBlock)
    r.Delete("/security/ip-blocks/{ip}", controlHandlers.DeleteIPBlock)

    // API Keys (moved from /admin/api-keys)
    r.Get("/api-keys", publicAPIHandlers.ListAllKeys)
    r.Post("/api-keys/{id}/suspend", publicAPIHandlers.SuspendKey)

    // AI Budget
    r.Get("/ai-budget", controlHandlers.GetAIBudget)
    r.Patch("/ai-budget", controlHandlers.UpdateAIBudget)

    // Audit Log (full cross-tenant view)
    r.Get("/audit-logs", controlHandlers.ListAuditLogs)

    // Deployments
    r.Get("/deployments", controlHandlers.ListDeployments)
})
```

### 3.2: Control Panel Handlers

**File:** `internal/control/handlers.go` (new file)

The `control` package already has a `Service` with tenant methods. Add HTTP handlers for all control panel operations. Some handlers will delegate to the existing `Service` methods, others will need new service methods.

**Handlers to implement:**

| `ListTenants` | `Service.ListTenants` | Already exists |
| `CreateTenant` | `Service.CreateTenant` | Already exists |
| `GetTenant` | `Service.GetTenant` | Already exists |
| `UpdateTenant` | `Service.UpdateTenant` | Already exists |
| `DeleteTenant` | `Service.DeleteTenant` | Already exists |
| `SuspendTenant` | `Service.SuspendTenant` | Already exists |
| `UnsuspendTenant` | `Service.UnsuspendTenant` | Already exists |
| `ListSystemSettings` | New service method | Query `system_settings` for tenant |
| `UpdateSystemSettings` | New service method | Bulk update settings |
| `UpdateSystemSetting` | New service method | Update single setting |
| `ResetSystemSetting` | New service method | Delete setting row |
| `ListFeatureFlags` | New service method | Query `feature_flags` table |
| `SetFeatureFlag` | New service method | Update feature flag |
| `HealthCheck` | New | Return server health |
| `DBHealthCheck` | New | Ping database |
| `DBVacuum` | New | Run VACUUM |
| `QueueStats` | New | Query Asynq stats |
| `ListDLQ` | New | List dead-letter tasks |
| `RetryDLQTask` | New | Re-enqueue DLQ task |
| `DiscardDLQTask` | New | Delete DLQ task |
| `DiscardAllDLQ` | New | Purge all DLQ |
| `CacheStats` | New | Query Valkey stats |
| `FlushCache` | New | Flush Valkey cache |
| `SecurityOverview` | New | Aggregate security metrics |
| `ListIPBlocks` | Delegate to `spam.Store` | Reuse existing |
| `CreateIPBlock` | Delegate to `spam.Store` | Reuse existing |
| `DeleteIPBlock` | Delegate to `spam.Store` | Reuse existing |
| `GetAIBudget` | New service method | Query AI budget |
| `UpdateAIBudget` | New service method | Update AI budget |
| `ListAuditLogs` | New service method | Query `audit_logs` |
| `ListDeployments` | New | Return deployment info |

### 3.3: Permission Bootstrap

**File:** `internal/server/server.go` — `bootstrapPermissions()`

Add control panel permissions:
```go
{Key: "control.panel.access", Description: "Access super-admin control panel"},
{Key: "control.tenants.manage", Description: "Manage tenants"},
{Key: "control.settings.manage", Description: "Manage system settings"},
{Key: "control.feature_flags.manage", Description: "Manage feature flags"},
{Key: "control.health.view", Description: "View infrastructure health"},
{Key: "control.queue.manage", Description: "Manage job queue"},
{Key: "control.cache.manage", Description: "Manage cache"},
{Key: "control.security.manage", Description: "Manage security settings"},
{Key: "control.api_keys.manage", Description: "Manage API keys"},
{Key: "control.ai_budget.manage", Description: "Manage AI budget"},
{Key: "control.audit_logs.view", Description: "View audit logs"},
```

All these are granted to `super_admin` via `bootstrapPermissions`.

### 3.4: Remove Old Admin API Key Routes

**File:** `internal/server/server.go`

Remove lines 1498-1502:
```go
// Admin API Key management (Section 47)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.view.all")).
    Get("/admin/api-keys", publicAPIHandlers.ListAllKeys)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "api.keys.manage")).
    Post("/admin/api-keys/{id}/suspend", publicAPIHandlers.SuspendKey)
```

### 3.5: Frontend Update

**File:** `apps/control/src/routes/api-keys/+page.svelte`

Update API calls:
- `/admin/api-keys` → `/control/api-keys`
- `/admin/api-keys/${id}/suspend` → `/control/api-keys/${id}/suspend`

### 3.6: Catalog Update

Remove from admin section:
```
GET    /admin/api-keys
POST   /admin/api-keys/:id/suspend
```

These are already documented in the control panel section.

### Steps
1. Create `internal/control/handlers.go` with handler stubs
2. Add control permissions to `bootstrapPermissions()`
3. Add `/control` route group in server.go
4. Remove old `/admin/api-keys` routes
5. Update frontend callers
6. `go build ./internal/server/` — confirm PASS
7. `python3 scripts/route_diff.py` — verify API keys move to MATCH
8. Implement remaining service methods incrementally

---

## Task 4: OAuth GET → POST (Medium — handler + frontend rework)

### 4.1: Handler Change

**File:** `internal/auth/oauth_handlers.go`

Change `HandleInitiate` to:
1. Accept POST requests (add POST method check or change route registration)
2. Instead of `http.Redirect`, return JSON: `{"auth_url": "https://..."}`
3. Keep the CSRF state cookie logic

```go
func (h *OAuthHandlers) HandleInitiate(w http.ResponseWriter, r *http.Request) {
    // ... existing provider validation + state generation ...

    // Return auth URL as JSON instead of redirect
    writeJSON(w, http.StatusOK, map[string]any{
        "data": map[string]string{"auth_url": provider.AuthURL(state)},
    })
}
```

### 4.2: Route Registration

**File:** `internal/server/server.go`

Change line 573:
```go
// Before:
r.Get("/oauth/{provider}/initiate", oauthHandlers.HandleInitiate)
// After:
r.Post("/oauth/{provider}/initiate", oauthHandlers.HandleInitiate)
```

Update the comment to remove the DEFERRED note.

### 4.3: Frontend Changes

**Files:**
- `apps/main/src/routes/login/+page.svelte` (lines 124, 137)
- `apps/main/src/routes/register/+page.svelte` (lines 55, 68)

Change `<a href="/api/v1/auth/oauth/google/initiate">` links to button clicks that:
1. POST to `/api/v1/auth/oauth/google/initiate`
2. Parse JSON response
3. `window.location.href = data.auth_url`

Pattern:
```svelte
<script>
async function initiateOAuth(provider) {
    const res = await fetch(`/api/v1/auth/oauth/${provider}/initiate`, { method: 'POST' });
    const { data } = await res.json();
    window.location.href = data.auth_url;
}
</script>

<button onclick={() => initiateOAuth('google')}>Continue with Google</button>
```

### 4.4: Catalog Update

The catalog already has `POST /auth/oauth/:provider`. Remove the old `GET /auth/oauth/:provider/initiate` from CODE_ONLY.

### 4.5: Full OAuth Regression Test

After implementation, test the complete flow:
1. Click "Continue with Google" on login page
2. Verify POST returns JSON with auth_url
3. Verify browser redirects to Google
4. Complete Google login
5. Verify callback works and JWT is issued
6. Test with GitHub provider
7. Test CSRF state validation (tamper with cookie → should fail)
8. Test with invalid provider → should return 400

### Steps
1. Change `HandleInitiate` to return JSON
2. Change route registration from GET to POST
3. Update frontend login/register pages
4. Update catalog (remove old GET path from reconciliation)
5. `go build ./internal/server/` — confirm PASS
6. Manual OAuth flow test (Google + GitHub)
7. `python3 scripts/route_diff.py` — verify CODE_ONLY = 0

---

## Validation

After all tasks:
1. `python3 scripts/route_diff.py` — CODE_ONLY should be 0
2. `go build ./internal/server/` — must be green
3. `go test ./internal/server/...` — must be green
4. Frontend typecheck: `bun run check` for touched apps
5. OAuth flow manual test
6. Migration applied to test DB

---

## Task Dependencies

```
Task 1 (page-templates) ─── independent
Task 2 (settings/public) ── independent
Task 3 (control panel) ──── independent
Task 4 (OAuth) ──────────── independent
```

All four tasks are independent and can be implemented in any order or in parallel.
