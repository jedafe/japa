# Section 46 — Admin Impersonation

**Status:** ✅ Locked

## Overview

Admin Impersonation allows a permitted admin (or delegated UserGroup member) to temporarily assume the identity of any non-admin user for support and debugging purposes. Every impersonation session is recorded in `audit_logs` — there is no silent impersonation. This is a RULES.md hard constraint, not a recommendation.

## RBAC Permission Node

```
admin.impersonate          — start/end an impersonation session
```

Gated to `super_admin` by default. Can be delegated to specific UserGroups (e.g., a "Support Tier 2" group). Not included in any default role. A user with `admin.impersonate` cannot impersonate another user who holds any admin-level role — impersonation targets must be non-privileged members or guests only.

## Valkey Session Key

```
jc:impersonate:{admin_id}   →   {target_user_id}   TTL: 30 minutes
```

- Only one active impersonation per admin at a time. Starting a new session while one is active silently terminates the previous session and logs the termination.
- TTL is non-renewable — the admin must re-initiate to extend. This is intentional (limits blast radius of an unattended session).

## API Endpoints

```go
// Start impersonation
// POST /api/v1/admin/impersonate
// Body: { "target_user_id": "uuid" }
// Auth: valid admin JWT + admin.impersonate permission
// Response: { "session_key": "jc:impersonate:{admin_id}", "expires_at": "ISO8601", "target": { "id", "username", "email" } }

// End impersonation
// DELETE /api/v1/admin/impersonate
// Auth: valid admin JWT (impersonation token OR normal token)

// Check current impersonation status (for banner)
// GET /api/v1/admin/impersonate/status
// Returns: { "active": bool, "target_user_id": "uuid|null", "expires_at": "ISO8601|null" }
```

## Go Handler Pattern

```go
// ImpersonateStart — POST /api/v1/admin/impersonate
func (h *AdminHandler) ImpersonateStart(w http.ResponseWriter, r *http.Request) {
    adminID := MustUserID(r)

    var body struct { TargetUserID uuid.UUID `json:"target_user_id"` }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        renderError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    // Safety: refuse to impersonate any user with admin-level permissions
    if h.rbac.HasAdminCapability(r.Context(), body.TargetUserID) {
        renderError(w, http.StatusForbidden, "cannot impersonate admin-level users")
        return
    }

    // Terminate any existing session (logged below)
    existing, _ := h.valkey.Get(r.Context(), "jc:impersonate:"+adminID.String())
    if existing != "" {
        h.auditLog(r.Context(), AuditEntry{
            ActorUserID:       &adminID,
            ImpersonatedUserID: uuidFromString(existing),
            ActionType:        "impersonate.terminate_previous",
            IPAddress:         realIP(r),
        })
    }

    // Set new session
    ttl := 30 * time.Minute
    if err := h.valkey.Set(r.Context(), "jc:impersonate:"+adminID.String(),
        body.TargetUserID.String(), ttl); err != nil {
        renderError(w, http.StatusInternalServerError, "failed to create impersonation session")
        return
    }

    h.auditLog(r.Context(), AuditEntry{
        ActorUserID:       &adminID,
        ImpersonatedUserID: &body.TargetUserID,
        ActionType:        "impersonate.start",
        Metadata:          map[string]any{"ttl_seconds": ttl.Seconds()},
        IPAddress:         realIP(r),
    })

    renderJSON(w, http.StatusOK, map[string]any{
        "expires_at": time.Now().Add(ttl).UTC().Format(time.RFC3339),
        "target":     h.users.GetPublicProfile(r.Context(), body.TargetUserID),
    })
}

// ImpersonateEnd — DELETE /api/v1/admin/impersonate
func (h *AdminHandler) ImpersonateEnd(w http.ResponseWriter, r *http.Request) {
    adminID := MustAdminID(r) // resolves from either normal or impersonation token
    existing, err := h.valkey.GetDel(r.Context(), "jc:impersonate:"+adminID.String())
    if err != nil || existing == "" {
        renderError(w, http.StatusNotFound, "no active impersonation session")
        return
    }
    h.auditLog(r.Context(), AuditEntry{
        ActorUserID:       &adminID,
        ImpersonatedUserID: uuidFromString(existing),
        ActionType:        "impersonate.end",
        IPAddress:         realIP(r),
    })
    renderJSON(w, http.StatusOK, map[string]any{"terminated": true})
}
```

## Middleware: Impersonation Identity Resolution

The impersonation middleware runs after auth, injecting the impersonated user's identity into the request context for all downstream handlers. The admin's real identity is preserved in a separate context key so audit logging can always access both.

```go
// ImpersonationMiddleware — wraps all non-admin routes
func ImpersonationMiddleware(valkey *valkey.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            adminID, ok := AdminIDFromContext(r.Context())
            if !ok {
                next.ServeHTTP(w, r)
                return
            }
            target, err := valkey.Get(r.Context(), "jc:impersonate:"+adminID.String())
            if err != nil || target == "" {
                next.ServeHTTP(w, r)
                return
            }
            targetID := uuid.MustParse(target)
            ctx := context.WithValue(r.Context(), ctxKeyEffectiveUserID, targetID)
            ctx = context.WithValue(ctx, ctxKeyRealAdminID, adminID) // preserved for audit
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## Audit Log Schema Extension

The `audit_logs` table (defined in Section 21) requires one additional column:

```sql
ALTER TABLE audit_logs
  ADD COLUMN impersonated_user_id uuid REFERENCES users(id) ON DELETE SET NULL;

-- Non-null only when an action is performed during an impersonation session.
-- actor_user_id = the real admin; impersonated_user_id = the user being viewed as.
CREATE INDEX idx_audit_impersonation ON audit_logs (impersonated_user_id, created_at DESC)
  WHERE impersonated_user_id IS NOT NULL;
```

## UI Banner

A yellow sticky banner displayed to the logged-in admin at the top of every page during an active session:

```svelte
<!-- ImpersonationBanner.svelte — rendered in root layout when session active -->
{#if $impersonationStore.active}
  <div class="impersonation-banner" role="alert" aria-live="polite">
    <span>⚠ Viewing as <strong>{$impersonationStore.target.username}</strong></span>
    <span class="timer">Session expires in {$impersonationStore.remainingMinutes}m</span>
    <button onclick={endImpersonation}>End Session</button>
  </div>
{/if}
```

The `impersonationStore` polls `/api/v1/admin/impersonate/status` every 60 seconds to refresh the remaining time. On expiry or on `End Session` click, it calls `DELETE /api/v1/admin/impersonate` and reloads to the admin panel root.

## Safety Rules (RULES.md constraints)

- **No silent impersonation.** Every start, extension attempt, and end MUST write to `audit_logs`. A failed Valkey write must abort the request — never proceed without logging.
- **No admin-on-admin impersonation.** Any user holding a role with admin-panel access cannot be impersonated. Enforced at the API handler level, not just UI.
- **Impersonation sessions do not inherit 2FA.** The impersonated identity has no MFA context — 2FA-gated actions within the session will be blocked. This is intentional and prevents privilege escalation through impersonation.
- **No impersonation token refresh.** The 30-minute TTL is final. The frontend timer must warn the admin at 5 minutes remaining.

## Release Placement

V1.2.0 — Admin Operations & Safety phase. Depends on: Section 02 (Auth/Sessions), Section 03 (RBAC), Section 21 (Audit Log / `audit_logs` table), Section 11 (Asynq — no direct dependency but audit log writes are synchronous).

## See Also

Section 02 (auth token model), Section 03 (RBAC permission nodes), Section 21 (audit_logs canonical store), ADR-047 (impersonation design rationale).
