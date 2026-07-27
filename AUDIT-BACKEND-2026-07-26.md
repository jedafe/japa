# JapaConnect Backend Audit Report

**Date:** 2026-07-26
**Scope:** Full systematic audit of the JapaConnect Go backend against JEOS specification catalogs
**Auditor:** Devin (japa-backend-auditor skill)
**Status:** Complete

---

## SUMMARY

| Severity | Count |
|----------|-------|
| Critical | 15 |
| High | 18 |
| Medium | 33 |
| Low | 14 |
| **Total** | **80** |

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Routes | 0 | 1 | 15 | 0 | 16 |
| Asynq Tasks | 0 | 1 | 1 | 6 | 8 |
| Valkey Keys | 6 | 3 | 1 | 2 | 12 |
| System Settings | 0 | 5 | 11 | 0 | 16 |
| Migrations | 0 | 0 | 1 | 2 | 3 |
| RBAC Permissions | 4 | 1 | 1 | 2 | 8 |
| Database Hygiene | 4 | 0 | 1 | 0 | 5 |
| RBAC Middleware | 0 | 5 | 0 | 0 | 5 |
| Financial Integrity | 1 | 0 | 0 | 0 | 1 |
| Security | 0 | 0 | 0 | 1 | 1 |
| Environment | 0 | 0 | 1 | 0 | 1 |
| Search/Meilisearch | 0 | 0 | 1 | 1 | 2 |
| Monitoring | 0 | 0 | 0 | 0 | 0 |
| House Rules | 0 | 0 | 0 | 0 | 0* |

*House Rules git-label audit blocked — no `.git` history in working copy.

---

## CRITICAL FINDINGS (15)

### AUDIT-BE-RBAC-002 — `dm.view` permission used but never seeded
- **Category:** RBAC Permissions
- **Severity:** Critical
- **Location:** internal/server/server.go:1259
- **Finding:** Code uses `dm.view` permission node, but no migration seeds it (000099 seeds `dm.send`, `dm.request`, `dm.manage_privacy` but not `dm.view`).
- **Action:** Add `dm.view` to migration 000099 or create a new seed migration 000170.
- **Status:** **Already Implemented** — `dm.view` IS seeded in migration 000099_seed_data.up.sql:231 (`('dm.view', 'View direct messages')`). Audit finding was a false positive.

### AUDIT-BE-RBAC-003 — `spaces.moderator.assign` permission used but never seeded
- **Category:** RBAC Permissions
- **Severity:** Critical
- **Location:** internal/server/server.go:1020-1023
- **Finding:** Code uses `spaces.moderator.assign` permission node, but only `spaces.moderate.assigned` is seeded in migrations.
- **Action:** Add `spaces.moderator.assign` to a seed migration.
- **Status:** **Already Implemented** — `spaces.moderator.assign` IS seeded in migration 000125_seed_missing_rbac_permissions.up.sql:19 (`('spaces.moderator.assign', 'Assign moderators to spaces')`). Audit finding was a false positive.

### AUDIT-BE-RBAC-004 — `control.panel.access` permission used but never seeded
- **Category:** RBAC Permissions
- **Severity:** Critical
- **Location:** internal/server/server.go:2951
- **Finding:** Code uses `control.panel.access` permission node, documented in 03-rbac.md, but not seeded in migration 000099.
- **Action:** Add `control.panel.access` to a seed migration.
- **Status:** **Already Implemented** — `control.panel.access` IS seeded in migration 000099_seed_data.up.sql:380 AND 000125_seed_missing_rbac_permissions.up.sql:21. Audit finding was a false positive.

### AUDIT-BE-RBAC-001 — `admin.feature_flags.manage` vs `features.manage` mismatch
- **Category:** RBAC Permissions
- **Severity:** Critical
- **Location:** internal/server/server.go:844-851
- **Finding:** Code uses `admin.feature_flags.manage` (5 occurrences), but JEOS 03-rbac.md documents `features.manage`. Mismatch between docs and implementation.
- **Action:** Reconcile — either update JEOS to document `admin.feature_flags.manage` or update code to use `features.manage`.
- **Status:** **Already Implemented** — Reconciled to `admin.feature_flags.manage` for all 5 admin feature-flag routes (PATCH toggle + GET/POST/PUT/DELETE CRUD). JEOS §174 updated to name `admin.feature_flags.manage` as canonical; `features.manage` documented as deprecated alias. Matches admin frontend `+page.svelte:49`.

### AUDIT-BE-DB-001 — Raw SQL in ban enforcement middleware (existence check)
- **Category:** Database Hygiene
- **Severity:** Critical
- **Location:** internal/server/server.go:719-724
- **Finding:** Ban enforcement middleware uses raw `database.Reader.QueryRow` with inline SQL to check `user_moderation_actions` instead of a sqlc query.
- **Action:** Create sqlc query `CheckUserBanExists` in admin_moderation.sql and replace raw SQL.
- **Status:** **Already Implemented** — Added `CheckUserBanExists` query to `internal/db/queries/admin_moderation.sql`, regenerated via `sqlc generate`, and replaced raw SQL in `internal/server/server.go` with `banQueries.CheckUserBanExists()`.

### AUDIT-BE-DB-002 — Raw SQL in ban enforcement middleware (expiration)
- **Category:** Database Hygiene
- **Severity:** Critical
- **Location:** internal/server/server.go:729-733
- **Finding:** Ban enforcement middleware uses raw SQL to get `expires_at` instead of sqlc query.
- **Action:** Create sqlc query `GetUserBanExpiration` in admin_moderation.sql.
- **Status:** **Already Implemented** — Added `GetUserBanExpiration` query to `internal/db/queries/admin_moderation.sql`, regenerated via `sqlc generate`, and replaced raw SQL in `internal/server/server.go` with `banQueries.GetUserBanExpiration()`.

### AUDIT-BE-DB-003 — Raw SQL in operation ban enforcement (existence check)
- **Category:** Database Hygiene
- **Severity:** Critical
- **Location:** internal/server/server.go:744-749
- **Finding:** Operation ban enforcement uses raw SQL to check `user_operation_bans` instead of sqlc query.
- **Action:** Create sqlc query `CheckUserOperationBanExists` in granular_bans.sql.
- **Status:** **Already Implemented** — Added `CheckUserOperationBanExists` query to `internal/db/queries/granular_bans.sql`, regenerated via `sqlc generate`, and replaced raw SQL in `internal/server/server.go` with `banQueries.CheckUserOperationBanExists()`.

### AUDIT-BE-DB-004 — Raw SQL in operation ban enforcement (expiration)
- **Category:** Database Hygiene
- **Severity:** Critical
- **Location:** internal/server/server.go:754-758
- **Finding:** Operation ban enforcement uses raw SQL to get `expires_at` instead of sqlc query.
- **Action:** Create sqlc query `GetUserOperationBanExpiration` in granular_bans.sql.
- **Status:** **Already Implemented** — Added `GetUserOperationBanExpiration` query to `internal/db/queries/granular_bans.sql`, regenerated via `sqlc generate`, and replaced raw SQL in `internal/server/server.go` with `banQueries.GetUserOperationBanExpiration()`.

### AUDIT-BE-FIN-001 — Mutable balance column UPDATE violates immutable ledger
- **Category:** Financial Integrity
- **Severity:** Critical
- **Location:** internal/worker/affiliate_enhancements.go:41
- **Finding:** Direct `UPDATE users SET credits_balance = credits_balance + $1 WHERE id = $2` violates the immutable ledger requirement (Section 28). The `users` table schema does not contain a `credits_balance` column, suggesting this code is either dead or targets a non-existent column.
- **Action:** Remove or replace with a `wallet_ledger` INSERT per Section 28. Verify if this code path is reachable.
- **Status:** **Already Implemented** — Replaced the mutable `UPDATE users SET credits_balance` with an immutable `wallet_ledger` INSERT inside a transaction holding `pg_advisory_xact_lock(user_id)`, matching the `SpendCredits`/`ConvertPointsToCredits` pattern. Added sqlc queries `GetAffiliateContentCommissionForCredit` and `MarkAffiliateCommissionCredited` to `internal/db/queries/affiliates.sql`. The `author_share` (numeric(12,2) major units) is converted to kobo (minor units) for the BIGINT ledger amount. Idempotent on commission row ID.

### AUDIT-BE-VALKEY-001 — `jc:llms_txt:{tenant_id}` documented but not implemented
- **Category:** Valkey Keys
- **Severity:** Critical
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** `jc:llms_txt:{tenant_id}` documented in catalog but not found in code.
- **Action:** Implement in bots/llms.go or remove from catalog if deprecated.
- **Status:** **Already Implemented** — False positive. The key IS implemented in `internal/bots/llms.go:15,126` as `llmsCacheKey + ":" + t.ID.String()` where `llmsCacheKey = "jc:llms_txt"`. The audit's grep missed it because the key is built via string concatenation, not a single literal.

### AUDIT-BE-VALKEY-002 — `jc:llms_full_txt:{tenant_id}` documented but not implemented
- **Category:** Valkey Keys
- **Severity:** Critical
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** `jc:llms_full_txt:{tenant_id}` documented in catalog but not found in code.
- **Action:** Implement in bots/llms.go or remove from catalog if deprecated.
- **Status:** **Already Implemented** — False positive. Implemented in `internal/bots/llms.go:16,129` as `llmsFullCacheKey + ":" + t.ID.String()` where `llmsFullCacheKey = "jc:llms_full_txt"`.

### AUDIT-BE-VALKEY-003 — `jc:robots_txt:{tenant_id}` documented but not implemented
- **Category:** Valkey Keys
- **Severity:** Critical
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** `jc:robots_txt:{tenant_id}` documented in catalog but not found in code.
- **Action:** Implement in bots/robots.go or remove from catalog if deprecated.
- **Status:** **Already Implemented** — False positive. Implemented in `internal/bots/robots.go:17,105` as `robotsCacheKey + ":" + t.ID.String()` where `robotsCacheKey = "jc:robots_txt"`.

### AUDIT-BE-VALKEY-004 — `jc:editor_lock:{content_type}:{content_id}` documented but not implemented
- **Category:** Valkey Keys
- **Severity:** Critical
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** `jc:editor_lock:{content_type}:{content_id}` documented in catalog but not found in code.
- **Action:** Implement in middleware/editlock.go or remove from catalog if deprecated.
- **Status:** **Already Implemented** — False positive. Implemented in `internal/middleware/editlock.go:5,65` (full `EditLockMiddleware` with the key pattern `jc:editor_lock:{content_type}:{content_id}` and 90s default TTL, configurable via `content.edit_lock_ttl_seconds` system setting).

### AUDIT-BE-VALKEY-005 — `jc:ratelimit:forgot_password:{email}` documented but not implemented
- **Category:** Valkey Keys
- **Severity:** Critical
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** `jc:ratelimit:forgot_password:{email}` documented in catalog but not found in code.
- **Action:** Implement in auth handlers or remove from catalog if using generic bruteforce key.
- **Status:** **Already Implemented** — False positive. Implemented in `internal/auth/handlers.go:797` as `emailKey := "jc:ratelimit:forgot_password:" + req.Email` inside `HandleForgotPassword`.

### AUDIT-BE-VALKEY-006 — `jc:ratelimit:resend_verification:{email}` documented but not implemented
- **Category:** Valkey Keys
- **Severity:** Critical
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** `jc:ratelimit:resend_verification:{email}` documented in catalog but not found in code.
- **Action:** Implement in auth handlers or remove from catalog if using generic bruteforce key.
- **Status:** **Already Implemented** — False positive. Implemented in `internal/auth/handlers.go:1103` as `emailKey := "jc:ratelimit:resend_verification:" + req.Email` inside `HandleResendVerification`.

---

## HIGH FINDINGS (18)

### AUDIT-BE-ROUTE-040 — Learning track review endpoints missing
- **Category:** Routes
- **Severity:** High
- **Location:** catalog: Learning section
- **Finding:** Catalog lists `POST /api/v1/learn/tracks/{id}/reviews` and `GET /api/v1/learn/tracks/{id}/reviews` for Japa University course reviews — NOT FOUND in code.
- **Action:** Implement learning track review endpoints (reuse Section 29 reviews system with `entity_type=learning_track`).
- **Status:** **Already Implemented** — Added `CreateTrackReview` and `ListTrackReviews` handlers to `internal/learning/handlers.go`, and `CreateTrackReview`/`ListTrackReviews`/`GetTrackReviewAggregate` store methods to `internal/learning/store.go`. Reuses the Section 29 polymorphic reviews table with `entity_type=learning_track`. Routes wired in `internal/server/server.go:2403-2408`: POST requires auth + `reviews.create` RBAC, GET is public. Returns reviews + aggregate rating.

### AUDIT-BE-TASK-001 — Two Asynq tasks defined but never enqueued
- **Category:** Asynq Tasks
- **Severity:** High
- **Location:** internal/worker/tasks.go:90-91
- **Finding:** `affiliate:credit_author_commission` and `content:scan_affiliate_links` are defined with handlers but never enqueued anywhere in the codebase.
- **Action:** Implement enqueue logic in affiliate/content services (per Sections 55/56) or remove if feature not yet built.
- **Status:** **Already Implemented** —
  - `content:scan_affiliate_links`: Now enqueued on content publish via `enqueueAffiliateLinkScan()` in `internal/content/handlers.go:170`. The worker handler now respects the `affiliate.scan_on_publish` system setting (exits early if disabled).
  - `affiliate:credit_author_commission`: Now enqueued by the new `POST /admin/affiliate/commissions` endpoint. Added `InsertAffiliateContentCommission` sqlc query, `RecordAffiliateCommission` service method (looks up content author + provider's `author_commission_pct`, computes `author_share`, inserts the commission row, enqueues the credit task), and `RecordAffiliateCommission` handler. Route wired at `internal/server/server.go:2879`. The admin sets the author's revenue share percentage via the existing `PATCH /admin/affiliate/providers/{id}/author-commission` endpoint.

### AUDIT-BE-VALKEY-007 — `jc:university:instructor:{user_id}:profile` not implemented
- **Category:** Valkey Keys
- **Severity:** High
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** University instructor profile cache key documented but not implemented in code.
- **Action:** Implement in university service or remove from catalog.
- **Status:** **Already Implemented** — Added `cachedInstructorProfile`/`invalidateInstructorProfileCache` to `internal/university/service.go`. Read-through cache with 1hr TTL on `GetMyProfile`; invalidated on `UpdateMyProfile`, `ApproveInstructor`, `RejectInstructor`, `SuspendInstructor`, `ReinstateInstructor`. Valkey client wired via `Service.SetValkey()` in `internal/server/server.go:530`.

### AUDIT-BE-VALKEY-008 — `jc:university:track:{track_id}:enrolled` not implemented
- **Category:** Valkey Keys
- **Severity:** High
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** University track enrollment cache key documented but not implemented in code.
- **Action:** Implement in university service or remove from catalog.
- **Status:** **Already Implemented** — Added `cachedTrackEnrolled`/`incrementTrackEnrolledCache`/`invalidateTrackEnrolledCache` to `internal/university/service.go`. 10min TTL. Incremented in `ProcessEnrollment` alongside the DB counter. Read-through fallback to `track.TotalEnrolled` on cache miss.

### AUDIT-BE-VALKEY-009 — `jc:university:payout:{tenant_id}:{period}:{instructor_id}` not implemented
- **Category:** Valkey Keys
- **Severity:** High
- **Location:** catalog (valkey-keys-catalog.md)
- **Finding:** University payout cache key documented but not implemented in code.
- **Action:** Implement in university service or remove from catalog.
- **Status:** **Already Implemented** — Added `cachedPayoutAccumulator`/`invalidatePayoutAccumulatorCache` to `internal/university/service.go`. Read-through cache with 7-day safety TTL. Used in `CalculatePeriodPayouts` (replaces direct `SumOpenSplits` call). Invalidated after split insertion in `ProcessEnrollment` and after payout stamping.

### AUDIT-BE-SETTING-001 — `affiliate.author_commission_default` missing from Go settings catalog
- **Category:** System Settings
- **Severity:** High
- **Location:** internal/control/settings_catalog.go
- **Finding:** Setting documented in JEOS catalog but missing from the Go settings catalog (Control Panel UI cannot render it).
- **Action:** Add to internal/control/settings_catalog.go with proper metadata.
- **Status:** **Already Implemented** — Added to `internal/control/settings_catalog.go:107` as `{Type: "float", DefaultValue: "25", Description: "Default % of external affiliate commission shared with content author (when provider has no override)."}`.

### AUDIT-BE-SETTING-002 — `affiliate.unregistered_link_policy` missing from Go settings catalog
- **Category:** System Settings
- **Severity:** High
- **Location:** internal/control/settings_catalog.go
- **Finding:** Setting documented in JEOS catalog but missing from the Go settings catalog.
- **Action:** Add to internal/control/settings_catalog.go.
- **Status:** **Already Implemented** — Added to `internal/control/settings_catalog.go:108` as `{Type: "string", DefaultValue: "flag", Description: "How to handle unregistered affiliate links: flag (log only), block (prevent publish), strip (remove link)."}`.

### AUDIT-BE-SETTING-003 — `affiliate.scan_on_publish` missing from Go settings catalog
- **Category:** System Settings
- **Severity:** High
- **Location:** internal/control/settings_catalog.go
- **Finding:** Setting documented in JEOS catalog but missing from the Go settings catalog.
- **Action:** Add to internal/control/settings_catalog.go.
- **Status:** **Already Implemented** — Added to `internal/control/settings_catalog.go:109` as `{Type: "bool", DefaultValue: "true", Description: "Whether to scan content for unregistered affiliate links on publish."}`.

### AUDIT-BE-SETTING-004 — All 7 `university.*` settings missing from Go settings catalog
- **Category:** System Settings
- **Severity:** High
- **Location:** internal/control/settings_catalog.go
- **Finding:** `university.subdomain`, `university.custom_domain`, `university.creator_percentage_default`, `university.platform_percentage_default`, `university.payout_schedule`, `university.min_payout_kobo`, `university.pro_stream_rate_kobo` — all documented in JEOS catalog and seeded in migration 000169 but missing from the Go settings catalog.
- **Action:** Add all 7 university settings to internal/control/settings_catalog.go.
- **Status:** **Already Implemented** — All 7 university.* settings added to `internal/control/settings_catalog.go:111-117` with types, default values, and descriptions matching the JEOS catalog (lines 169-175).

### AUDIT-BE-SETTING-005 — `geo_personalization.enabled` missing from Go settings catalog
- **Category:** System Settings
- **Severity:** High
- **Location:** internal/control/settings_catalog.go
- **Finding:** Setting documented in JEOS catalog and seeded in migration 000169 but missing from the Go settings catalog.
- **Action:** Add to internal/control/settings_catalog.go.
- **Status:** **Already Implemented** — Added to `internal/control/settings_catalog.go:119` as `{Type: "bool", DefaultValue: "true", Description: "Master switch for geo-aware feed/search/discovery weighting."}`.

### AUDIT-BE-RBAC-005 — University permission nodes partially seeded
- **Category:** RBAC Permissions
- **Severity:** High
- **Location:** internal/server/server.go:2451-2456
- **Finding:** Code uses `university.instructor.approve`, `university.instructor.manage`, `university.course.approve`, `university.payout.process` — documented in permission-key-naming.md but only partially seeded in migration 000168.
- **Action:** Verify migration 000168 seeds all 12 university nodes from permission-key-naming.md.
- **Status:** **Already Implemented** — Verified. Migration 000168_seed_university_rbac_nodes.up.sql seeds all 12 university.* nodes: `university.instructor.apply`, `.approve`, `.manage`, `university.course.create`, `.submit`, `.approve`, `.feature`, `university.enrollment.view_own`, `university.review.create`, `.moderate`, `university.payout.view_own`, `.process`. The 6 admin-scope nodes are granted to super_admin_role; the 6 self-serve nodes are enforced at the handler via rbac.RequirePermission (super_admin bypasses via `*` wildcard). No action needed.

### AUDIT-BE-RBAC-MW-001 — POST /content/{id}/view lacks RBAC permission check
- **Category:** RBAC Middleware
- **Severity:** High
- **Location:** internal/server/server.go:1062
- **Finding:** POST /content/{id}/view has auth middleware but no `rbac.RequirePermission` check.
- **Action:** Add `rbac.RequirePermission(rbacResolver, "content.view")` or document as intentionally open to authenticated users.
- **Status:** **Already Implemented** — Documented as intentionally open to authenticated users. View recording is an inherent authenticated-user analytics action, not a permission-gated operation. No `content.view` RBAC node exists (only `admin.content.view` for the admin panel). The api-routes-catalog.md lists this route without a `[public]` marker, confirming auth-required but no per-permission gate. Comment added at `internal/server/server.go:1072-1077`.

### AUDIT-BE-RBAC-MW-002 — POST /content/{id}/report lacks RBAC permission check
- **Category:** RBAC Middleware
- **Severity:** High
- **Location:** internal/server/server.go:1064
- **Finding:** POST /content/{id}/report has auth middleware but no `rbac.RequirePermission` check.
- **Action:** Add `rbac.RequirePermission(rbacResolver, "moderation.report")`.
- **Status:** **Already Implemented** — Added `rbac.RequirePermission(rbacResolver, "moderation.report")` to POST /content/{id}/report. The `moderation.report` node is seeded in 000099:340 and already gates the sibling report endpoints (/spaces/{id}/report, /comments/{id}/report, /users/{id}/report, /dm/.../report). Now consistent.

### AUDIT-BE-RBAC-MW-003 — POST /content/{id}/share lacks RBAC permission check
- **Category:** RBAC Middleware
- **Severity:** High
- **Location:** internal/server/server.go:1116
- **Finding:** POST /content/{id}/share has auth middleware but no `rbac.RequirePermission` check.
- **Action:** Add `rbac.RequirePermission(rbacResolver, "content.share")`.
- **Status:** **Already Implemented** — False positive (already fixed before this audit pass). POST /content/{id}/share at `internal/server/server.go:1133` already has `rbac.RequirePermission(rbacResolver, "content.share")`. The `content.share` node is seeded in 000144:23.

### AUDIT-BE-RBAC-MW-004 — POST /content/render lacks RBAC permission check
- **Category:** RBAC Middleware
- **Severity:** High
- **Location:** internal/server/server.go:1098
- **Finding:** POST /content/render has auth middleware but no `rbac.RequirePermission` check.
- **Action:** Add `rbac.RequirePermission(rbacResolver, "content.render")`.
- **Status:** **Already Implemented** — Added `rbac.RequirePermission(rbacResolver, "content.render")` to POST /content/render. The `content.render` node is seeded in 000099:115 and 000109:99.

### AUDIT-BE-RBAC-MW-005 — POST /coupons/{id}/click lacks RBAC permission check
- **Category:** RBAC Middleware
- **Severity:** High
- **Location:** internal/server/server.go:2245
- **Finding:** POST /coupons/{id}/click has rate limiting but no auth or RBAC permission check.
- **Action:** Add auth + `rbac.RequirePermission(rbacResolver, "coupons.click")` or make explicitly public with tighter rate limiting.
- **Status:** **Already Implemented** — Documented as intentionally public. Per api-routes-catalog.md:706, this route is marked `[public, optional auth]`. It records a coupon click for analytics/attribution and must be callable by unauthenticated visitors. Rate limiting ("moderate" tier) is the only gate. No `coupons.click` RBAC node exists and none is needed — this is a public tracking endpoint. Comment added at `internal/server/server.go:2267-2274`.

### AUDIT-BE-TASK-008 — Hardcoded task string instead of constant
- **Category:** Asynq Tasks
- **Severity:** High (reclassified from Medium — DRY violation that could cause silent task drops)
- **Location:** internal/control/handlers.go:1536
- **Finding:** Security scan task is enqueued using hardcoded string `"security:scan"` instead of the constant `worker.TypeSecurityScan`.
- **Action:** Replace `asynq.NewTask("security:scan", payload)` with `asynq.NewTask(worker.TypeSecurityScan, payload)`.
- **Status:** **Already Implemented** — Replaced `asynq.NewTask("security:scan", payload)` with `asynq.NewTask(worker.TypeSecurityScan, payload)` at `internal/control/handlers.go:1539`. The `worker` package was already imported.

---

## MEDIUM FINDINGS (33)

### Routes (15 undocumented routes in code but not in catalog)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-ROUTE-001 | server.go:794 | `/api/v1/csrf` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-002 | server.go:808-809 | `/api/v1/auth/totp/verify-challenge` and `/api/v1/auth/totp/backup` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-003 | server.go:899 | `/api/v1/forms/{id}/ensure-field-group` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-004 | server.go:1175 | `/api/v1/me/push-tokens` (DELETE without :id) — catalog specifies `/{id}` | ✅ Resolved — catalog now documents BOTH `DELETE /me/push-tokens` (body-keyed, current V1 implementation per server.go:1201-1203 comment) AND `DELETE /me/push-tokens/{id}` (path-param variant reserved for P4 migration). api-routes-catalog.md:135-136 |
| AUDIT-BE-ROUTE-005 | server.go:1181 | `/api/v1/feed/for-you` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-006 | server.go:1282 | `/api/v1/media/serve/{bucket}/*` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-007 | server.go:1470 | `/api/v1/comparisons/auto-dimensions` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-008 | server.go:1547 | `/api/v1/profiles/invitations/accept` not in catalog | ✅ Resolved — `POST /profiles/invitations/accept` is documented in api-routes-catalog.md:528 and wired at server.go:2748 |
| AUDIT-BE-ROUTE-009 | server.go:1785-1789 | `/api/v1/admin/i18n/strings` (POST) and `/batch` not in catalog | ✅ Resolved — `POST /admin/i18n/strings` and `POST /admin/i18n/strings/batch` are documented in api-routes-catalog.md:1123-1124 and wired at server.go:1813,1815. Also covers `DELETE /admin/i18n/strings/{id}` (catalog:1125), `POST /admin/i18n/strings/{id}/translate` (catalog:1126), and `POST /admin/i18n/strings/ai-translate-batch` (catalog:1127) |
| AUDIT-BE-ROUTE-010 | server.go:1932-1943 | `/api/v1/public/*` prefix not documented in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-011 | server.go:1952 | `/api/v1/me/preferences/content-country` not in catalog (ADR-064) | ✅ Resolved — `PATCH /me/preferences/content-country` is documented in api-routes-catalog.md:463 (with ADR-064 reference at line 462) and wired at server.go:1980 |
| AUDIT-BE-ROUTE-012 | server.go:1974 | `/api/v1/me/organizations` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-013 | server.go:2014-2015 | `/api/v1/admin/users/{id}/hard` (DELETE) and `/profile-type` (PATCH) not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-014 | server.go:2995 | `/control/queue/{queue_name}/tasks` not in catalog | ✅ Resolved — added to catalog |
| AUDIT-BE-ROUTE-015 | server.go:3044-3047 | `/control/task-types` and `/control/api-scopes` not in catalog | ✅ Resolved — added to catalog |

**Action:** Add all 15 undocumented routes to the JEOS api-routes-catalog.md.

### Asynq Tasks (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-TASK-002 | enqueuer.go:TaskTypeCatalog() | `TypeAffiliateCreditAuthorCommission` and `TypeContentScanAffiliateLinks` missing from TaskTypeCatalog() — won't appear in Control Panel cron editor UI | ✅ Resolved — added to TaskTypeCatalog() and taskTier map |

### Valkey Keys (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-VALKEY-010 | catalog | Cache flush target patterns (`jc:cache:page:*`, `jc:cache:feed:*`, `jc:cache:i18n:*`, `jc:cache:search:*`, `jc:cache:analytics:*`) documented but not found in code | ✅ False positive — patterns are implemented in `internal/analytics/cache.go` |

### System Settings (11 catalog settings not read in code)

| ID | Setting Key | Finding | Status |
|----|-------------|---------|--------|
| AUDIT-BE-SETTING-006 | `feed.velocity.gravity` | Documented but not read in code | ✅ Resolved — wired into `HandleMaintenanceUpdateVelocityScores` (worker/handlers.go) |
| AUDIT-BE-SETTING-007 | `feed.max_followed_spaces_in_feed` | Documented but not read in code | ✅ Resolved — wired into `loadAffinity` (feed/service.go) |
| AUDIT-BE-SETTING-008 | `spaces.bubble_depth` | Documented but not read in code | ✅ Resolved — wired into bubble fan-out (worker/handlers.go) |
| AUDIT-BE-SETTING-009 | `spaces.max_pinned_posts` | Documented but not read in code | ✅ Resolved — wired into `PinContent` (content/store.go) |
| AUDIT-BE-SETTING-010 | `moderation.ai_auto_flag_threshold` | Documented but not read in code | ✅ Resolved (intentional forward-compat) — Setting is correctly seeded (migration 000099:40), cataloged in JEOS (system-settings-catalog.md:41) AND Go (settings_catalog.go:45), and surfaced in the admin UI (admin/settings.go:293). The only missing piece is the consumer: the AI moderation agent tools `moderation.flag_content` and `moderation.classify_severity` (Section 20 V1 tool catalog, line 15) are documented but not yet implemented — this is a V2 feature, not a defect. The setting is reserved so the admin UI renders the control before the classifier ships. Per project rule (don't remove useful forward-compatible features), the setting is retained. |
| AUDIT-BE-SETTING-011 | `ads.frequency_cap_per_day` | Documented but not read in code | ✅ Resolved — wired into `RenderSlot` (advertising/service.go) |
| AUDIT-BE-SETTING-012 | `ads.min_campaign_budget` | Documented but not read in code | ✅ Resolved — wired into `CreateCampaign` (advertising/service.go) |
| AUDIT-BE-SETTING-013 | `api.idempotency_ttl_hours` | Documented but not read in code (TODO in roadmap) | ✅ Resolved — wired into `IdempotencyMiddleware` (middleware/idempotency.go) |
| AUDIT-BE-SETTING-014 | `content.max_tags` | Documented but not read in code | ✅ Resolved — wired into `AddContentTag` handler (content/handlers.go) |
| AUDIT-BE-SETTING-015 | `payment.currency` | Documented but not read in code | ✅ Resolved — wired into `CreatePayment` (payments/service.go) |
| AUDIT-BE-SETTING-016 | `onboarding.promoted_spaces/users/tags` | 3 of 5 onboarding.* settings not read in code | ✅ False positive — all 3 are read in `onboarding/service.go:385-387` |

**Action:** Implement usage for each or remove from catalog if feature is not yet built.

### Migrations (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-MIG-003 | migrations/ | `000107_trending_snapshots.sql` and `000108_coupon_affiliate_system.sql` have `.down.sql` files but are missing `.up.sql` extension in filenames | ✅ Resolved — renamed to `.up.sql` + added `IF NOT EXISTS` to indexes |

**Action:** Rename to `000107_trending_snapshots.up.sql` and `000108_coupon_affiliate_system.up.sql`.

### RBAC Permissions (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-RBAC-006 | migrations/000109 | Seeds both underscore-scoped and dot-scoped variants of some nodes (e.g., `blog.edit_own` and `blog.edit.own`), creating redundancy | ✅ Resolved — migration 000135 cleans up dot-scoped variants |

**Action:** Review migration 000135 cleanup to ensure proper removal of legacy dot-scoped variants.

### Database Hygiene (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-DB-005 | internal/university/service.go:325-326 | N+1 query: loop over instructor profiles calls `SumOpenSplits` for each instructor individually | ✅ Resolved — added batch query `SumOpenSplitsByAllInstructors`, `CalculatePeriodPayouts` now fetches all sums in one query |

**Action:** Create batch query `SumOpenSplitsByInstructors` accepting multiple instructor IDs.

### Search/Meilisearch (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-SEARCH-002 | internal/search/indexer.go:453 | `relevant_countries` hardcoded to empty array instead of populated from geo targeting metadata | ✅ Resolved — content indexer now resolves country from `geo_space_id` via space hierarchy walk; learning tracks remain empty (no geo column in table — V1 limitation) |

**Action:** Populate from track geo targeting metadata or document V1 limitation.

### Environment (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-ENV-002 | internal/testdb/testdb.go:103 | `JC_TEST_DATABASE_URL` used in test code but not in JEOS env var catalog | ✅ Resolved — added to `.env.example` |

**Action:** Add to JEOS env var catalog for completeness (test-only).

---

## LOW FINDINGS (14)

### Asynq Tasks (6)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-TASK-003 | enqueuer.go:taskTier | `TypeAffiliateCreditAuthorCommission` and `TypeContentScanAffiliateLinks` missing from taskTier map (default to "default" tier) | ✅ Resolved — added to taskTier map |
| AUDIT-BE-TASK-004 | enqueuer.go:taskTier | `TypeUniversityNotifyInstructorApproved`, `TypeUniversityNotifyCourseApproved`, `TypeUniversitySendInstructorPayout` missing from taskTier map (default to "default" tier — correct but not explicit) | ✅ Resolved — added to taskTier map |
| AUDIT-BE-TASK-005 | enqueuer.go:taskTier | `TypeAffiliateCreditAuthorCommission` missing from taskTier (should be default tier per JEOS) | ✅ Resolved — added as "default" tier |
| AUDIT-BE-TASK-006 | enqueuer.go:taskTier | `TypeContentScanAffiliateLinks` missing from taskTier (should be low tier per JEOS) | ✅ Resolved — added as "low" tier |
| AUDIT-BE-TASK-007 | enqueuer.go:TaskTypeCatalog() | Missing catalog entries for 2 affiliate/content tasks | ✅ Resolved — added to TaskTypeCatalog() |
| AUDIT-BE-TASK-008 | internal/control/handlers.go:1536 | Hardcoded `"security:scan"` string instead of `worker.TypeSecurityScan` constant *(also listed as High due to DRY risk)* | ✅ Resolved — `asynq.NewTask(worker.TypeSecurityScan, payload)` at internal/control/handlers.go:1539 (with audit-trail comment at 1536-1538). Matches the High-severity entry above. |

### Valkey Keys (2)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-VALKEY-011 | internal/visitor/resolver.go:69 | `jc:visitor:ctx:{session_id}` uses 15-min TTL in code but catalog specifies 30-min TTL | ✅ Resolved — catalog updated to 15min to match code |
| AUDIT-BE-VALKEY-012 | internal/visitor/resolver.go:124,126 | Code uses `jc:visitor:ctx:user:{user_id}` and `jc:visitor:ctx:ip:{ip}` but catalog documents generic `jc:visitor:ctx:{session_id}` | ✅ Resolved — catalog updated with both specific key patterns |

### Migrations (2)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-MIG-001 | migrations/ | Migration sequence has intentional gaps at 000037 and 000043 (documented in MIGRATION-ORDER.md) — no action needed | ✅ No action needed |
| AUDIT-BE-MIG-002 | migrations/ | 71 .up.sql files; MIGRATION-ORDER.md claims 186 tables — clarify distinction between table count and migration file count | ✅ Resolved — added "File count vs table count" note to MIGRATION-ORDER.md:18 explaining that 186 = logical table count, ~127 = forward migration files (68 `*.up.sql` + 59 legacy `*.sql`), and a single migration may create multiple tables while others (ALTER/seed/index) create zero. The two numbers measure different things. |

### RBAC Permissions (2)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-RBAC-007 | migrations/000099 | Seeds `forum.create` but code uses `forum.thread.create` and `forum.reply.create` — dead seed | ✅ Resolved — migration 000170 removes dead seed; RBAC scope list updated |
| AUDIT-BE-RBAC-008 | migrations/000099 | Seeds `organizations.manage` but code uses `organizations.members.manage` and `organizations.invite` — dead seed | ✅ Resolved — migration 000170 removes dead seed |

### Search/Meilisearch (1)

| ID | Location | Finding | Status |
|----|----------|---------|--------|
| AUDIT-BE-SEARCH-001 | .jeos/sections/13-search.md | Section 13 does not document `knowledge_articles` and `learning_tracks` indexes | ✅ Resolved — Section 13 updated to list both indexes |

**Action:** Update Section 13 to reference these indexes.

### Security (1)

| ID | Location | Finding |
|----|----------|---------|
| AUDIT-BE-SEC-001 | Multiple handlers | All sampled handlers properly validate input after JSON decode — no issues found |

---

## CATALOG GAPS

### Undocumented in Code (catalog entries with no implementation)

**Valkey Keys (13 patterns not implemented):**
- `jc:llms_txt:{tenant_id}` (Critical)
- `jc:llms_full_txt:{tenant_id}` (Critical)
- `jc:robots_txt:{tenant_id}` (Critical)
- `jc:editor_lock:{content_type}:{content_id}` (Critical)
- `jc:ratelimit:forgot_password:{email}` (Critical)
- `jc:ratelimit:resend_verification:{email}` (Critical)
- `jc:university:instructor:{user_id}:profile` (High)
- `jc:university:track:{track_id}:enrolled` (High)
- `jc:university:payout:{tenant_id}:{period}:{instructor_id}` (High)
- `jc:cache:page:*` (Medium)
- `jc:cache:feed:*` (Medium)
- `jc:cache:i18n:*` (Medium)
- `jc:cache:search:*` (Medium)
- `jc:cache:analytics:*` (Medium)

**Asynq Tasks (13 V2-deferred — expected missing):**
- `university:fetch_logo`, `docs:generate_page`, `wallet:convert_points`, `wallet:process_refund`, `subscription:handle_renewal`, `subscription:handle_cancellation`, `payment:sync_subscription_status`, `data:cleanup_expired_exports`, `referral:process_qualifying_action`, `content:fan_out_comment`, `content:related_reindex`, `maintenance:prune_content_views`
- All marked "V2 Deferred" in Section 11 — absence is correct.

**Routes (1 missing):**
- `POST /api/v1/learn/tracks/{id}/reviews` and `GET /api/v1/learn/tracks/{id}/reviews` — learning track reviews

### Undocumented in Catalog (code with no catalog entry)

**Routes (15):** See AUDIT-BE-ROUTE-001 through 015 above.

**Asynq Tasks (0):** All 52 code tasks are documented in the catalog.

---

## CLEAN AREAS

The following checklist areas found **no issues**:

1. **Security — JSON validation:** All sampled handlers validate input after decode before reaching sqlc/external services.
2. **Security — MinIO references:** No MinIO references found. ADR-048 compliance confirmed (SeaweedFS used).
3. **Security — GDPR/Honcho erasure:** Honcho erasure is hard deletion with retry logic. Correctly implemented.
4. **Financial — Credits → Points reverse conversion:** No reverse conversion code found. One-way enforced.
5. **Financial — Points → Credits conversion:** Properly implemented as one-way irreversible with transaction safety, advisory locks, idempotency keys.
6. **Financial — Credits expiration:** No credits expiration logic found. Matches spec ("No expiration on either Points or Credits").
7. **Environment — JC_ prefix:** All 44 production env vars use JC_ prefix.
8. **Environment — Hardcoded credentials:** None found. All sensitive values loaded from env vars.
9. **Error tracking — GlitchTip:** Properly wired via `errpkg.CaptureException()` on all critical paths (payment webhooks, wallet conversion, GDPR tasks, Asynq panics).
10. **House rules — Coupon code in embeds:** No coupon_code stored in embeds.
11. **House rules — Honcho separation:** Honcho uses separate HTTP client, does not pass through main sqlc/migrate pipeline.
12. **RBAC middleware — Auth coverage:** All non-public Chi routes have auth middleware applied.
13. **SQLC coverage:** 58 query files, 1,038 queries, 0 orphaned query files.
14. **Meilisearch — tenant_id filterable:** All 7 indexes have `tenant_id` as a filterable attribute.
15. **Asynq — Orphaned enqueues:** 0 — all 46 enqueued tasks have matching handlers.
16. **Asynq — Missing handlers:** 0 — all 52 code tasks have handlers registered.

---

## RECOMMENDED ACTIONS (Priority Order)

### Immediate (Critical — fix before production)

1. **Seed missing RBAC permission nodes** (AUDIT-BE-RBAC-002, 003, 004): Create migration 000170 to seed `dm.view`, `spaces.moderator.assign`, `control.panel.access`. These will cause 403 errors in production.
2. **Reconcile `admin.feature_flags.manage` vs `features.manage`** (AUDIT-BE-RBAC-001): Update JEOS or code to match.
3. **Replace raw SQL in ban enforcement middleware** (AUDIT-BE-DB-001 through 004): Create 4 sqlc queries and replace inline SQL in server.go.
4. **Fix mutable balance column UPDATE** (AUDIT-BE-FIN-001): Remove or replace `internal/worker/affiliate_enhancements.go:41` with wallet ledger INSERT. Verify if code is reachable (column may not exist).
5. **Implement or deprecate 6 critical Valkey patterns** (AUDIT-BE-VALKEY-001 through 006): llms_txt, llms_full_txt, robots_txt, editor_lock, forgot_password rate limit, resend_verification rate limit.

### High Priority

6. **Implement learning track review endpoints** (AUDIT-BE-ROUTE-040): `POST/GET /api/v1/learn/tracks/{id}/reviews`.
7. **Add missing RBAC permission checks** (AUDIT-BE-RBAC-MW-001 through 005): Secure 5 write endpoints with `rbac.RequirePermission`.
8. **Add 11 settings to Go settings catalog** (AUDIT-BE-SETTING-001 through 005): 3 affiliate.*, 7 university.*, 1 geo_personalization.enabled.
9. **Implement or remove 3 university Valkey cache patterns** (AUDIT-BE-VALKEY-007 through 009).
10. **Implement or remove 2 unused Asynq tasks** (AUDIT-BE-TASK-001): `affiliate:credit_author_commission`, `content:scan_affiliate_links`.
11. **Fix hardcoded task string** (AUDIT-BE-TASK-008): Replace `"security:scan"` with `worker.TypeSecurityScan`.
12. **Verify migration 000168 seeds all 12 university permission nodes** (AUDIT-BE-RBAC-005).

### Medium Priority

13. **Document 15 undocumented routes** in JEOS api-routes-catalog.md.
14. **Implement or remove 11 stale catalog settings** (AUDIT-BE-SETTING-006 through 016).
15. **Fix N+1 query in university payout calculation** (AUDIT-BE-DB-005): Batch `SumOpenSplitsByInstructors`.
16. **Rename 2 migration files** to include `.up.sql` extension (AUDIT-BE-MIG-003).
17. **Complete geo weighting implementation** (AUDIT-BE-SEARCH-002): Populate `relevant_countries` from metadata.
18. **Add 2 tasks to TaskTypeCatalog()** (AUDIT-BE-TASK-002).
19. **Implement or remove 5 cache flush Valkey patterns** (AUDIT-BE-VALKEY-010).
20. **Clean up redundant dot-scoped permission nodes** (AUDIT-BE-RBAC-006).

### Low Priority

21. **Add missing tasks to taskTier map** (AUDIT-BE-TASK-003 through 006).
22. **Resolve visitor context TTL mismatch** (AUDIT-BE-VALKEY-011): 15min code vs 30min catalog.
23. **Update catalog for visitor key format** (AUDIT-BE-VALKEY-012).
24. **Update Section 13-search.md** to document `knowledge_articles` and `learning_tracks` indexes (AUDIT-BE-SEARCH-001).
25. **Remove dead permission seeds** `forum.create` and `organizations.manage` (AUDIT-BE-RBAC-007, 008).
26. **Add `JC_TEST_DATABASE_URL` to env var catalog** (AUDIT-BE-ENV-002).

---

## AUDIT METADATA

- **Auditor:** Devin (japa-backend-auditor skill)
- **Date:** 2026-07-26
- **JEOS reference files consulted:**
  - `.jeos/conventions/api-routes-catalog.md`
  - `.jeos/conventions/asynq-tasks.md`
  - `.jeos/conventions/valkey-keys-catalog.md`
  - `.jeos/conventions/system-settings-catalog.md`
  - `.jeos/conventions/permission-key-naming.md`
  - `.jeos/sections/01-foundation-tech-stack.md`
  - `.jeos/sections/03-rbac.md`
  - `.jeos/sections/11-job-queue-asynq.md`
  - `.jeos/sections/13-search.md`
  - `.jeos/sections/17-payment-core.md`
  - `.jeos/schemas/MIGRATION-ORDER.md`
- **Codebase scanned:** internal/server/, internal/worker/, internal/search/, internal/university/, internal/visitor/, internal/users/, internal/control/, internal/wallet/, internal/gdpr/, internal/honcho/, internal/config/, migrations/, internal/db/queries/
- **Build status at audit time:** `go build ./...` and `go vet ./...` both clean.
