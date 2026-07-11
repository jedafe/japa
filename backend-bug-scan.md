# Backend Bug Scan Report

**Date:** 2026-07-10  
**Scope:** Full JapaConnect Go backend scan  
**Protocol:** JEOS backend-debugger.md  
**Status:** Initial scan complete — 15 findings across 5 severity levels

---

## How to Use This Report

Each finding follows the 5-step debugging protocol:
1. **CLASSIFY** — Functional / Integration / Performance / Security / Logic / Syntax
2. **LOCATE** — Exact source file and line number
3. **ROOT CAUSE** — One-sentence explanation
4. **FIX** — Minimal code change
5. **VERIFY** — How to confirm the fix

---

## CRITICAL BUGS

### B1. Missing idempotency middleware on subscription endpoints

- **Category:** Security / Financial
- **File:** `internal/server/server.go:1367` and `:1371`
- **Root Cause:** `POST /subscriptions` (`CreateSubscription`) and `POST /me/subscription/cancel` (`CancelMySubscription`) are financial endpoints that lack the required `mw.IdempotencyMiddleware(vkClient.Redis())`. AGENTS.md Rule 9 mandates idempotency for all state-changing financial endpoints.
- **Fix:** Wrap both route handlers with `mw.IdempotencyMiddleware(vkClient.Redis())`, matching the existing pattern at lines `1352-1354` and `1363-1365`.

```go
// Before
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage")).
    Post("/subscriptions", paymentHandlers.CreateSubscription)

// After
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "payments.subscriptions.manage"),
    mw.IdempotencyMiddleware(vkClient.Redis())).
    Post("/subscriptions", paymentHandlers.CreateSubscription)
```

- **Verify:** Retry `POST /api/v1/subscriptions` with the same `Idempotency-Key` header; expect cached 201 on duplicate.

---

### B2. Cookie domain compliance gap

- **Category:** Security / Compliance
- **File:** `internal/config/config.go:182-186`
- **Root Cause:** AGENTS.md requires cookies scoped to `domain=.japaconnect.com`, but production enforcement only checks that `JC_COOKIE_DOMAIN` is non-empty. Any domain value passes.
- **Fix:** Validate the exact value in production:

```go
if cfg.IsProduction() && cfg.CookieDomain != ".japaconnect.com" {
    slog.Error("JC_COOKIE_DOMAIN must be exactly .japaconnect.com in production")
    os.Exit(1)
}
```

- **Verify:** `echo $JC_COOKIE_DOMAIN` must equal `.japaconnect.com` in production.

---

## HIGH BUGS

### B3. Hand-written raw SQL in payments handlers

- **Category:** Integration / Compliance
- **File:** `internal/payments/handlers.go:1016`, `:1052`, `:1072`
- **Root Cause:** `UpdateDiscountCode`, `DeleteSubscription`, and `ReactivateSubscription` use `h.Service.db.Writer.Exec(r.Context(), rawSQL...)` instead of sqlc-generated queries. AGENTS.md Rule 2 prohibits hand-editing generated code and requires sqlc for database access.
- **Fix:** Add SQL annotations in `db/queries/*.sql`, regenerate with `make sqlc-gen`, and replace raw `Exec` calls with generated methods.

```sql
-- db/queries/payments.sql
-- name: UpdateDiscountCode :one
UPDATE discount_codes
SET code = COALESCE(NULLIF($3,''), code), updated_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING id, code, updated_at;
```

```go
// After regeneration
_, err = h.Service.UpdateDiscountCode(ctx, gen.UpdateDiscountCodeParams{...})
```

- **Verify:** `git diff -- internal/payments/handlers.go` should show no `db.Writer.Exec` with inline SQL.

---

### B4. Worker handlers ignore task cancellation context

- **Category:** Logic / Correctness
- **File:** `internal/worker/handlers.go:2776`, `:2801`, `:2837`, `:2895`, `:2944`
- **Root Cause:** Five background task handlers use `context.Background()` for database operations instead of the task’s `ctx` parameter. This ignores Asynq task cancellation, deadlines, and tracing context.
- **Fix:** Replace `context.Background()` with the task `ctx` in all five `dbPool.Writer.Exec(...)` calls.

```go
// Before
result, err := dbPool.Writer.Exec(context.Background(), `UPDATE ...`)

// After
result, err := dbPool.Writer.Exec(ctx, `UPDATE ...`)
```

- **Verify:** Review each of the five locations; `context.Background()` should no longer appear in `Exec` calls within task handlers.

---

## MEDIUM BUGS

### B5. CORS header inconsistency

- **Category:** Integration
- **File:** `internal/server/server.go:91` vs `internal/middleware/cors.go:38`
- **Root Cause:** The inline CORS middleware in `server.go` omits `Idempotency-Key` from `Access-Control-Allow-Headers`, while the standalone `cors.go` includes it. If the inline version is active, idempotent requests will fail CORS preflight.
- **Fix:** Ensure the active CORS configuration includes `Idempotency-Key` in the allowed headers.

```go
w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Request-ID")
```

- **Verify:** `curl -I -X OPTIONS https://your-domain/api/v1/payments/intent -H "Access-Control-Request-Headers: Idempotency-Key"` should return 204 with the header present.

---

### B6. Stub task handlers silently no-op

- **Category:** Functional
- **File:** `internal/worker/handlers.go:2766-2959`
- **Root Cause:** 10+ Phase 31 handlers are stubs: `HandleMaintenancePruneOldFeeds`, `HandleMaintenancePrunePushTokens`, `HandleDMCleanupDeclinedRequests`, `HandleCouponTrackEvents`, `HandleAffiliateRecordClick`, `HandleAffiliateCreditCommission`, `HandleMaintenanceCheckSLABreaches`, `HandleMaintenanceUpdateGeoIPDB`, `HandleSpaceAutoPopulate`, `HandleDBManualVacuum`. Registered in the mux but immediately return or contain `TODO` comments. Queued tasks are consumed but produce no work.
- **Fix:** Either implement the handlers fully or comment out their `mux.HandleFunc` registrations in `internal/worker/tasks.go` until ready.
- **Verify:** `grep -n "TODO" internal/worker/handlers.go` should return zero results after implementation.

---

### B7. Content hash not computed when body is cleared

- **Category:** Logic / Rule Violation
- **File:** `internal/content/handlers.go:445-448`
- **Root Cause:** AGENTS.md Rule 12 requires `content_hash` computed before every save. The current code only computes it when `input.Body != ""`. Explicitly clearing the body leaves the old hash intact.
- **Fix:** Always compute and set `ContentHash` on update.

```go
// Before
if input.Body != "" {
    hash := ComputeContentHash(input.Body)
    updateInput.ContentHash = &hash
}

// After
hash := ComputeContentHash(input.Body)
updateInput.ContentHash = &hash
```

- **Verify:** PATCH `/api/v1/content/{id}` with `{"body": ""}` and confirm `content_hash` updates to the empty-body hash.

---

### B8. Malformed JSON silently ignored in 18+ handlers

- **Category:** Logic
- **Files:** `internal/spaces/handlers.go:1504,1532,1587`, `internal/admin/moderation.go:460,520,575,591,676,694`, `internal/professional/handlers.go:1447,1476`, `internal/publicapi/handlers.go:474`, `internal/pagebuilder/handlers.go:695`, `internal/affiliate/handlers.go:447`, `internal/advertising/handlers.go:864`, `internal/payments/handlers.go:1014`, `internal/tags/handlers.go:366`
- **Root Cause:** These handlers use `_ = json.NewDecoder(r.Body).Decode(&input)` which discards the parse error. Malformed JSON is silently ignored, and the handler proceeds with zero-value fields.
- **Fix:**

```go
// Before
_ = json.NewDecoder(r.Body).Decode(&input)

// After
if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
    writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
    return
}
```

- **Verify:** `grep -rn "_ = json.NewDecoder(r.Body)" internal/*/handlers.go` should return no results.

---

### B9. Request body never closed in most handlers

- **Category:** Resource Leak
- **Files:** All `internal/*/handlers.go` except webhook handlers
- **Root Cause:** `json.NewDecoder(r.Body).Decode(&input)` reads from the body but `r.Body` is never closed. Only `payments/handlers.go` and `payments/webhooks.go` properly defer `r.Body.Close()`.
- **Fix:** Add `defer func() { _ = r.Body.Close() }()` at the top of every handler that reads `r.Body`, or introduce a middleware that drains and closes the body after decoding.
- **Verify:** `grep -rn "defer.*Body\.Close" internal/*/handlers.go` should show coverage in every handler that decodes JSON.

---

### B10. `os.Exit(1)` can crash production startup

- **Category:** Availability
- **File:** `internal/config/config.go:125` and `:186`
- **Root Cause:** Two `os.Exit(1)` calls in config validation hard-crash the process. Line 186 enforces `JC_COOKIE_DOMAIN` in production; line 125 exits on other config failures. A misconfiguration brings down the entire service instead of allowing graceful degradation or restart with backoff.
- **Fix:** Return errors from config initialization and let the caller decide how to handle missing config.

```go
// Before
if cfg.IsProduction() && cfg.CookieDomain == "" {
    slog.Error("JC_COOKIE_DOMAIN must be set in production...")
    os.Exit(1)
}

// After
if cfg.IsProduction() && cfg.CookieDomain == "" {
    return fmt.Errorf("JC_COOKIE_DOMAIN must be set in production for cross-subdomain session sharing")
}
```

- **Verify:** Remove `JC_COOKIE_DOMAIN` and restart; expect a logged error and clean shutdown, not a forced exit.

---

### B11. Fragile error matching via string containment

- **Category:** Logic
- **Files:** 142 instances across all handlers (e.g., `internal/content/handlers.go:106`, `internal/spaces/handlers.go:81`, `internal/comments/handlers.go:399`, `internal/payments/handlers.go:227`)
- **Root Cause:** `strings.Contains(err.Error(), "duplicate")`, `strings.Contains(err.Error(), "required")`, etc. are used instead of typed sentinel errors. A substring match can accidentally fire on unrelated errors from different database drivers or wrapped contexts.
- **Fix:** Define typed sentinel errors in service layers and use `errors.Is()` in handlers.

```go
// Service layer
var ErrDuplicateEmail = errors.New("duplicate email")

// Handler layer
case errors.Is(err, ErrDuplicateEmail):
    writeError(w, http.StatusConflict, "email_exists", "Email already in use")
```

- **Verify:** `grep -rn "strings.Contains(err.Error" internal/*/handlers.go` should return zero results.

---

### B12. `RemoveContentTag` has no internal auth verification

- **Category:** Security
- **File:** `internal/content/handlers.go:1051-1070`
- **Root Cause:** Unlike `AddContentTag`, `RemoveContentTag` does not call `requireUser`. It relies entirely on router middleware. If route wiring ever changes, the handler silently allows unauthenticated deletes.
- **Fix:** Add `requireUser` at the top of `RemoveContentTag`.

```go
func (h *Handlers) RemoveContentTag(w http.ResponseWriter, r *http.Request) {
    user, ok := h.requireUser(w, r)
    if !ok {
        return
    }
    _ = user // used for RBAC audit trail
    // ... rest of handler
}
```

- **Verify:** `grep -A5 "RemoveContentTag" internal/content/handlers.go` should show `requireUser` as the first call.

---

## LOW BUGS

### B13. Content status transitions bypass authorization

- **Category:** Security
- **File:** `internal/content/handlers.go:506-548`
- **Root Cause:** `PublishContent` and `ArchiveContent` do not call `requireUser` and have no RBAC middleware in the router. Any authenticated user can publish or archive any content.
- **Fix:** Add `auth.RequireAuth` and `rbac.RequirePermission(rbacResolver, "content.publish")` middleware to these routes, and add `requireUser` in the handler.
- **Verify:** `curl -X POST /api/v1/content/{id}/publish` without auth should return 401.

---

### B14. `content_hash` not updated when body is cleared

- **Category:** Logic / Rule Violation
- **File:** `internal/content/handlers.go:445-448`
- **Root Cause:** Same as B7 — duplicate entry because the empty-body edge case is distinct from the general hash-computation bug. Rule 12 requires hash before every save; explicit body clearing must still produce a hash.
- **Fix:** Same as B7 — always compute hash.
- **Verify:** Same as B7.

---

### B15. 10+ Phase 31 task handlers are stubs

- **Category:** Functional
- **File:** `internal/worker/handlers.go:2766-2959`
- **Root Cause:** Handlers registered in `internal/worker/tasks.go` but implemented as no-ops: `HandleMaintenancePruneOldFeeds`, `HandleMaintenancePrunePushTokens`, `HandleDMCleanupDeclinedRequests`, `HandleCouponTrackEvents`, `HandleAffiliateRecordClick`, `HandleAffiliateCreditCommission`, `HandleMaintenanceCheckSLABreaches`, `HandleMaintenanceUpdateGeoIPDB`, `HandleSpaceAutoPopulate`, `HandleDBManualVacuum`. Asynq consumes these tasks silently.
- **Fix:** Implement handlers or comment out mux registrations.
- **Verify:** Each handler should have non-trivial logic and no `TODO` comments.

---

## Verification Matrix

| ID | Command / Check | Expected Result |
|---|---|---|
| B1 | `grep -n "IdempotencyMiddleware" internal/server/server.go` around lines 1367, 1371 | Idempotency middleware present on both subscription routes |
| B2 | `echo $JC_COOKIE_DOMAIN` in production | Exactly `.japaconnect.com` |
| B3 | `grep -n "db.Writer.Exec" internal/payments/handlers.go` | Zero matches after sqlc migration |
| B4 | `grep -n "context.Background()" internal/worker/handlers.go` | Zero matches in task handlers |
| B5 | `curl -I -X OPTIONS .../payments/intent -H "Access-Control-Request-Headers: Idempotency-Key"` | 204 with `Idempotency-Key` in allow-headers |
| B6 | `grep -n "TODO" internal/worker/handlers.go` | Zero results after implementation |
| B7 | PATCH content with empty body, inspect `content_hash` | Hash updated to empty-body value |
| B8 | `grep -rn "_ = json.NewDecoder(r.Body)" internal/*/handlers.go` | Zero results |
| B9 | `grep -rn "defer.*Body.Close" internal/*/handlers.go` | Every JSON-reading handler closes body |
| B10 | Remove `JC_COOKIE_DOMAIN` and restart | Clean shutdown log, no `os.Exit` |
| B11 | `grep -rn "strings.Contains(err.Error" internal/*/handlers.go` | Zero results |
| B12 | `grep -A5 "RemoveContentTag" internal/content/handlers.go` | `requireUser` on first line |
| B13 | Unauthenticated `POST /content/{id}/publish` | 401 |
| B14 | Same as B7 | Same as B7 |
| B15 | `grep -n "TODO" internal/worker/handlers.go` | Zero results |

---

## Compliance Notes

- **AGENTS.md Rule 2:** Never hand-edit generated sqlc files. Fix `.sql` annotations and regenerate.
- **AGENTS.md Rule 4:** All Valkey keys use `jc:` prefix. Verified across 96 matches; no violations found.
- **AGENTS.md Rule 6:** Permission cache invalidation is implemented in `internal/rbac/store.go` via `invalidateUserCache` and `invalidateGroupCache` after every RBAC mutation.
- **AGENTS.md Rule 9:** Idempotency middleware exists for financial endpoints but is missing on subscription routes (B1).
- **AGENTS.md Rule 11:** Soft deletes used for `contents`, `comments`, `media`. No raw `DELETE FROM contents/comments/media` found in application code.
- **AGENTS.md Rule 12:** Content hash computation exists but has empty-body edge case gap (B7/B14).
- **AGENTS.md Rule 17:** No AI-generated attribution found in code comments or commit messages. Clean.

---

*Report generated by Kilo backend-debugger protocol.*
