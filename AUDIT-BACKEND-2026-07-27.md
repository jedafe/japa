# JapaConnect Backend Audit Report — Round 2

**Date:** 2026-07-27
**Scope:** Systematic audit of the remaining `internal/` folders NOT covered by `AUDIT-BACKEND-2026-07-26.md`, compared side-by-side with the JEOS specification catalogs.
**Auditor:** Devin (japa-backend-auditor + jc-backend-audit skills)
**Status:** Complete

---

## SCOPE & METHODOLOGY

### Already Audited (2026-07-26 — DO NOT re-audit)
`internal/server/`, `internal/worker/`, `internal/search/`, `internal/university/`, `internal/visitor/`, `internal/users/`, `internal/control/`, `internal/wallet/`, `internal/gdpr/`, `internal/honcho/`, `internal/config/`, `internal/db/queries/`, `migrations/`

### Audited in This Round (58 folders, 6 batches)
| Batch | Folders |
|-------|---------|
| 1 — Auth/Identity/Security | `auth`, `rbac`, `csrf`, `impersonation`, `clientip`, `geoip`, `tenant`, `middleware` |
| 2 — Content/Social | `content`, `comments`, `forum`, `follows`, `mentions`, `interactions`, `reviews`, `spaces`, `tags`, `bookmarkfolders` |
| 3 — Monetization/Financial | `advertising`, `affiliate`, `coupons`, `payments`, `referral`, `professional` |
| 4 — Learning/Docs/Blog/Comms | `learning`, `universities`, `blog`, `docs`, `newsletter`, `email`, `menus`, `redirects`, `rss` |
| 5 — Infrastructure/Realtime | `websocket`, `webhooks`, `valkey`, `i18n`, `seo`, `bots`, `publicapi`, `renderer`, `linkpreview`, `features`, `pagebuilder`, `customfields` |
| 6 — AI/Analytics/Misc | `ai`, `analytics`, `reports`, `tickets`, `spam`, `trending`, `feed`, `media`, `onboarding`, `mentorship`, `comparison`, `organizations`, `notifications`, `errors`, `db` (non-queries) |

### JEOS Reference Files Consulted
- `.jeos/JEOS.md` (index)
- `.jeos/sections/02,03,04,05,06,07,08,09,10,11,12,13,14,15,16,17,18,20,24,25,26,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,46,47,48,49,51,52,53,55,56.md`
- `.jeos/conventions/api-routes-catalog.md`, `asynq-tasks.md`, `valkey-keys-catalog.md`, `system-settings-catalog.md`, `permission-key-naming.md`, `feature-flag-naming.md`, `error-handling-go.md`

---

## SUMMARY

| Severity | Count |
|----------|-------|
| Critical | 6 |
| High | 18 |
| Medium | 27 |
| Low | 14 |
| **Total** | **65** |

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Security | 3 | 2 | 4 | 1 | 10 |
| Business Logic | 0 | 6 | 7 | 2 | 15 |
| Settings | 0 | 4 | 4 | 0 | 8 |
| Tasks (Asynq) | 1 | 4 | 1 | 2 | 8 |
| Valkey | 0 | 2 | 3 | 0 | 5 |
| Routes | 1 | 1 | 3 | 1 | 6 |
| Database | 0 | 0 | 2 | 0 | 2 |
| Monitoring | 0 | 0 | 0 | 2 | 2 |
| Standards | 0 | 0 | 1 | 2 | 3 |
| Financial | 1 | 0 | 0 | 0 | 1 |
| Search | 0 | 0 | 2 | 0 | 2 |
| RBAC | 0 | 1 | 1 | 0 | 2 |
| Compliance | 0 | 0 | 0 | 1 | 1 |

---

## CRITICAL FINDINGS (6)

### AUDIT-BE2-SEC-001 — Missing admin-on-admin impersonation safety check — **Already Implemented**
- **Category:** Security
- **Severity:** Critical
- **Location:** internal/impersonation/service.go:27-45
- **Finding:** `StartImpersonation` does not verify that the target user is NOT an admin-level user before allowing impersonation, violating Section 46 ("A user with admin.impersonate cannot impersonate another user who holds any admin-level role").
- **Action:** Add RBAC check verifying the target user has no admin capabilities before allowing impersonation.
- **Fix:** Added `rbac.RBAC.HasAdminCapability` (checks wildcard, super_admin group, and the 13 admin capability nodes from migration 000147). Wired into `impersonation.Service` via `SetRBAC` + `ErrTargetIsAdmin` sentinel; handler maps to HTTP 403 `cannot_impersonate_admin`. Also replaced the stub `IsAdmin` (AUDIT-BE2-SEC-010).

### AUDIT-BE2-SEC-002 — Impersonation middleware not wired — **Already Implemented**
- **Category:** Routes / Security
- **Severity:** Critical
- **Location:** internal/server/server.go (missing), internal/impersonation/middleware.go:16-51
- **Finding:** `ImpersonationMiddleware` exists and injects impersonated identity into request context, but is never applied in `server.go`. Impersonation state is stored in Valkey but never read by downstream handlers — impersonation is effectively non-functional.
- **Action:** Wire `ImpersonationMiddleware` in `server.go` after auth middleware on all authenticated route groups.
- **Fix:** Wired `impersonation.ImpersonationMiddleware(impersonationStore)` in `server.go` after `auth.OptionalAuth` + `bots.BotDetectionMiddleware` on the `/api/v1` route group.

### AUDIT-BE2-SEC-003 — Custom fields embed resolver missing SSRF protection — **Already Implemented**
- **Category:** Security
- **Severity:** Critical
- **Location:** internal/customfields/embed.go:447-475
- **Finding:** oEmbed calls in the platform embed resolver use an HTTP client without SSRF checks, allowing fetches of internal resources via redirect. This contrasts with `internal/linkpreview` which has full SSRF protection.
- **Action:** Route oEmbed HTTP calls through the same SSRF-protected client used by `internal/linkpreview/ssrf.go`.
- **Fix:** `NewEmbedResolver` now constructs an SSRF-safe `http.Client` that calls `linkpreview.ValidateRedirectURL` on every redirect hop (max 5). `fetchOEmbed` validates the initial URL via `linkpreview.ValidateURL` before sending. Test override constructor disables SSRF (httptest uses 127.0.0.1).

### AUDIT-BE2-SEC-004 — DM privacy check gap allows unwanted messages — **Already Implemented**
- **Category:** Security / Business Logic
- **Severity:** Critical
- **Location:** internal/websocket/dm.go:505-507
- **Finding:** `GetOrCreateConversationForInitiator` does not check the recipient's `dm_privacy` setting before creating conversations. Users with `dm_privacy = "nobody"` can receive unsolicited DMs, violating Section 48.
- **Action:** Add recipient `dm_privacy` + block-list check before conversation creation; reject with `403 dm_privacy_blocked` when set to `nobody`/`followers_only` and the initiator is not a follower.
- **Fix:** Added `DMStore.IsBlocked` + `DMStore.IsMutualFollow` (bidirectional) helpers and a new sqlc query `GetConversationByParticipants` to detect existing threads. `DMService.GetOrCreateConversation` now: (1) returns existing conversations (privacy doesn't retroactively close active threads), (2) checks block list bidirectionally, (3) enforces `nobody`/`followers_only`/`everyone` privacy for new conversations, (4) fails closed on unknown privacy values. Handler maps `ErrDMBlocked`/`ErrDMPrivacyNobody`/`ErrDMNotMutualFollower` to HTTP 403 with machine codes `dm_blocked`/`dm_privacy_blocked`/`dm_not_mutual_follower`.

### AUDIT-BE2-SEC-005 — Custom fields field-level RBAC not enforced — **Already Implemented**
- **Category:** Security / RBAC
- **Severity:** Critical
- **Location:** internal/customfields/handlers.go
- **Finding:** Handlers check tenant scoping but not field-level visibility/editability per user group, as required by Section 06. Permission bypass is possible — any authenticated tenant user can read/write fields intended for restricted groups.
- **Action:** Enforce field-level RBAC by consulting the field's `visibility_user_group` / `editable_user_group` columns before read/write operations.
- **Fix:** Added sqlc queries `CheckFieldPermissionForUser` (joins `custom_field_permissions` with `user_group_memberships`, returns `can_view`/`can_edit`) and `ListVisibleFieldIDsForUser`. Wired `rbac.RBAC` into `customfields.Handlers` via `SetRBAC`. `ListFieldValues` now filters values to fields the user can view (super-admins bypass). `SetFieldValue` now rejects with 403 `field_not_editable` when the user lacks `can_edit` on the field (super-admins bypass). Fail-closed when no permission rows exist.

### AUDIT-BE2-FIN-001 — Raw SQL UPDATE violates immutable ledger (affiliate author commission) — **Already Implemented**
- **Category:** Financial / Database
- **Severity:** Critical
- **Location:** internal/worker/affiliate_enhancements.go:41
- **Finding:** `HandleAffiliateCreditAuthorCommission` executes `UPDATE users SET credits_balance = credits_balance + $1 WHERE id = $2` — a mutable balance UPDATE that violates the immutable-ledger requirement (Section 28). The `users` table has no `credits_balance` column, so the code is also non-functional.
- **Action:** Replace with an append-only `wallet_ledger` INSERT inside a transaction holding `pg_advisory_xact_lock`. If the code path is unreachable, remove it. *(Also flagged as AUDIT-BE-FIN-001 in the 2026-07-26 audit — re-confirmed here from the affiliate-side perspective.)*
- **Fix:** Already implemented in a prior session. The handler now acquires `pg_advisory_xact_lock(hashtextextended('wallet:{tenant}:{user}', 0))` inside a tx, reads the latest balance via `qtx.GetLatestBalance`, appends a `wallet_ledger` row via `qtx.InsertLedgerEntry` with an idempotency key `affiliate_commission:{id}`, and marks the commission credited within the same tx. No `UPDATE users SET credits_balance` remains — the only reference to that string is in the audit comment describing the old behavior.

---

## HIGH FINDINGS (18)

### AUDIT-BE2-SEC-006 — TOTPRequiredForAdmin defined but never called — **Already Implemented**
- **Category:** Security / Business Logic
- **Severity:** High
- **Location:** internal/auth/settings.go:47-58
- **Finding:** `TOTPRequiredForAdmin` reads `auth.totp.required_for_admin` but is never invoked anywhere. Admin 2FA enforcement per Section 02 is not implemented.
- **Action:** Call `TOTPRequiredForAdmin` in auth middleware or admin-panel access checks; reject admin access with `403 totp_required` when the setting is enabled and the admin lacks an active TOTP session.
- **Fix:** Added `auth.RequireTOTPForAdmin(settings, rbac, totp)` middleware that: (1) only applies to `/api/v1/admin/*` paths, (2) is a no-op when the setting is disabled, (3) only enforces for admin-level users (via `rbac.HasAdminCapability`), (4) rejects with 403 `totp_required` when the user has no active TOTP enrollment. Added `TOTPEnrollmentChecker` interface and `TOTPStoreImpl.HasTOTPEnrolled` implementation. Wired the middleware in `server.go` after `ImpersonationMiddleware` on the `/api/v1` route group.

### AUDIT-BE2-SEC-007 — GeoIP middleware bypasses trusted proxy validation — **Already Implemented**
- **Category:** Security
- **Severity:** High
- **Location:** internal/geoip/middleware.go:9-27
- **Finding:** `realIP` directly reads `X-Forwarded-For` / `X-Real-IP` without verifying the peer is a trusted proxy, unlike `clientip.FromRequest`. Allows IP spoofing for geo resolution.
- **Fix:** Replaced the inline `realIP` helper with `clientip.FromRequest(r)`, which only honours forwarding headers when the immediate peer is a trusted proxy (JC_TRUSTED_PROXIES, defaults to loopback + RFC1918). Updated `TestRealIP` to use trusted-proxy peers (127.0.0.1) and added an "Untrusted peer ignores X-Forwarded-For" case.
- **Action:** Replace `geoip.realIP` with `clientip.FromRequest` so trusted-proxy validation runs before accepting forwarding headers.

### AUDIT-BE2-SEC-008 — Renderer Bun script path not validated — **Already Implemented**
- **Category:** Security
- **Severity:** High
- **Location:** internal/renderer/renderer.go:80
- **Finding:** The Bun script path is taken from admin config without validation. Comment says "trusted admin config" but no enforcement — path traversal risk if config is compromised.
- **Action:** Validate the script path with `filepath.Clean` + an allowlist root, and reject absolute paths or `..` traversal.
- **Fix:** `renderer.New` now returns `(*Renderer, error)` and validates the script path via `validateScriptPath`: rejects absolute paths, any `..` segment, non-`.js`/`.ts` extensions, and paths that escape the allowlist root (`JC_RENDERER_SCRIPT_ROOT` or `./scripts/tiptap-ssr`). The resolved absolute path is stored. Removed the `//nolint:gosec` suppression on `exec.CommandContext`. Updated `server.go` to handle the error (logs and disables SSR on misconfiguration). Added `TestNew_RejectsInvalidScriptPath` covering absolute, traversal, wrong-extension, empty, and executable cases.

### AUDIT-BE2-SEC-009 — Redirects open-redirect vulnerability — **Already Implemented**
- **Category:** Security
- **Severity:** High
- **Location:** internal/redirects/service.go
- **Finding:** `target_url` is not validated as a valid URL, nor checked against an allowlist. Open-redirect attacks are possible (ADR-056 requires validation).
- **Action:** Validate `target_url` is a valid absolute URL; optionally restrict to allowed domains via a system setting allowlist.
- **Fix:** Added `validateTargetURL` which only accepts (a) root-relative paths starting with a single `/` or (b) absolute `http`/`https` URLs with a host. Rejects protocol-relative URLs (`//evil.com`), dangerous schemes (`javascript:`, `data:`, `file:`), scheme-less URLs, and relative paths without a leading slash. Added `ErrInvalidTargetURL` sentinel and mapped it to 400 `invalid_target_url` in `handleServiceError`. Applied the check in both `CreateRedirect` and `UpdateRedirect`. Added `TestValidateTargetURL` (10 cases) and `TestCreateRedirectRejectsInvalidTargetURL`.

### AUDIT-BE2-BL-001 — Missing comment notification Asynq tasks — **Already Implemented**
- **Category:** Tasks / Business Logic
- **Severity:** High
- **Location:** internal/comments/handlers.go:246-252
- **Finding:** `CreateComment` does not enqueue `comment:notify_author`, `comment:notify_parent`, or `comment:notify_mention` tasks specified in Section 49. Comment authors and mentioned users never receive notifications.
- **Action:** Enqueue the three notification tasks after successful comment creation (fire-and-forget pattern).
- **Fix:** Per Section 11, `comment:notify_author/parent/mention` were consolidated into `notification:dispatch_batch`. Added `NotificationDispatcher` interface to `comments.Handlers` and `DispatchCommentNotifications` method on `comments.Service` that fires: (1) `comment_new` to the content author for top-level comments, (2) `comment_reply` to the parent comment author for replies, (3) `comment_mention` to each @mentioned user extracted from the TipTap JSON body via `mentions.ExtractMentions`. Self-notifications are skipped. Added `commentNotificationAdapter` in `server.go` adapting `notifications.Service.DispatchNotification` (returns `DispatchResult`) to the interface (returns `error`). Wired via `commentsHandlers.SetNotifier(...)`. Fire-and-forget per Rule 8.

### AUDIT-BE2-BL-002 — Missing mentions notification task — **Already Implemented**
- **Category:** Tasks / Business Logic
- **Severity:** High
- **Location:** internal/mentions/mentions.go
- **Finding:** Mentions are extracted and filtered but the `mentions:notify` task (Section 49) is never enqueued. Mentioned users receive no notification.
- **Action:** Enqueue `mentions:notify` after filtering, integrated into content/comment creation flows.
- **Fix:** Per Section 11, `mentions:notify` was consolidated into `notification:dispatch_batch`. Added `MentionNotifier` interface and `dispatchMentionNotifications` helper to `content.Handlers` that extracts @mentioned users from the content body via `mentions.ExtractMentions` and fires a `content_mention` notification to each (skipping self-mentions). Wired via `contentHandlers.SetMentionNotifier(commentNotificationAdapter{...})` in `server.go`. Fire-and-forget per Rule 8. Comment mention notifications are handled in BL-001's `DispatchCommentNotifications`.

### AUDIT-BE2-BL-003 — Review aggregate recalc task handler not registered — **Already Implemented (No Change Needed)**
- **Category:** Tasks / Business Logic
- **Severity:** High
- **Location:** internal/reviews/handlers.go:48-70
- **Finding:** Handler enqueues `content:recalc_review_aggregate` but the Asynq task handler is not registered in the worker server. Task will fail silently or land in dead-letter.
- **Action:** Register the review aggregate recalculation handler in `internal/worker/server.go` with the correct queue tier.
- **Fix:** Verified the handler IS registered: `worker.RegisterHandlers` (tasks.go:125) wires `mux.HandleFunc(TypeReviewAggregateRecalc, HandleReviewAggregateRecalc)`, and `HandleReviewAggregateRecalc` exists in handlers.go:1201. The audit finding was a false positive — no change needed.

### AUDIT-BE2-BL-004 — Missing Meilisearch index sync for spaces — **Already Implemented**
- **Category:** Search / Business Logic
- **Severity:** High
- **Location:** internal/spaces/handlers.go:625
- **Finding:** Space creation enqueues `space:auto_populate` but does not enqueue a Meilisearch index sync task. New spaces are not searchable until the next periodic sweep.
- **Action:** Enqueue `search:index_entity` with `entity_type=space` after space creation/update/delete.
- **Fix:** Added `Service.EnqueueSearchIndex(spaceID, tenantID, operation)` to `internal/spaces/spaces.go` that enqueues `search:index_entity` with `entity_type=space` and `operation` ("index" or "delete"). Wired it into `CreateSpace` (index), `UpdateSpace` (index), and `DeleteSpace` (delete) handlers. The worker's `HandleSearchIndexSync` already supports `entity_type=space` via `searchIndexer.IndexSpace` / `DeleteDocument("spaces", ...)`.

### AUDIT-BE2-BL-005 — Missing space bubble depth implementation — **Already Implemented**
- **Category:** Business Logic
- **Severity:** High
- **Location:** internal/spaces/
- **Finding:** Section 05 specifies posts bubble up the space hierarchy based on `spaces.bubble_depth`, but no bubbling logic exists in content fan-out.
- **Action:** Implement post bubbling in the content fan-out path, propagating posts to parent spaces up to the configured `spaces.bubble_depth`.
- **Fix:** (1) Rewrote the `bubble` branch of `HandleContentFanOutToFeeds` to use `GetSpaceParentSpaces` (the `space_parent_spaces` join table, supporting multi-parent hierarchies) instead of the single `space.ParentID` column. Uses BFS with a visited set to walk up to `spaces.bubble_depth` levels (default 2, from `system_settings`), fanning out to each parent space's members. (2) Added `enqueueFeedFanOut` to `content.Handlers` and call it from `CreateContent` when `status == published`. (3) Added fan-out enqueue to `HandleScheduledPublish` so scheduled posts fan out when they go live. Previously `FanOutPost` was defined but never called from any handler — the fan-out task was never enqueued.

### AUDIT-BE2-BL-006 — Missing content hash duplicate check on update — **Already Implemented**
- **Category:** Business Logic
- **Severity:** High
- **Location:** internal/content/handlers.go:594-596
- **Finding:** `UpdateContent` computes `content_hash` but does not check for duplicates against existing content before allowing the update. Duplicate content can be silently persisted.
- **Action:** Add duplicate-detection check (hash comparison against existing rows for the same tenant) before applying the update.
- **Fix:** Added `GetContentByHash` sqlc query (`internal/db/queries/contents.sql`) that finds a row with the same `(tenant_id, content_hash)`, excluding the row being updated. Added `ErrDuplicateContent` sentinel. In `Service.UpdateContent`, before the write, if a `content_hash` is supplied, calls `GetContentByHash` and returns `ErrDuplicateContent` on a hit. Mapped to 409 `duplicate_content` in `handleServiceError`. The unique index `idx_contents_tenant_content_hash` remains the ultimate guard.

### AUDIT-BE2-VALKEY-001 — Missing Valkey mute key patterns — **Already Implemented**
- **Category:** Valkey / Business Logic
- **Severity:** High
- **Location:** internal/interactions/handlers.go:169-380
- **Finding:** Space mute handlers do not use the required `jc:muted_spaces:{user_id}` pattern from Section 52. Mute state is not cached, forcing DB lookups on every request.
- **Action:** Implement Valkey SET with 10min TTL on mute/unmute, matching the catalog pattern.
- **Fix:** Added a `vk valkeycompat.Cmdable` field to `interactions.Service` with `SetValkey` setter. Defined `mutedUsersKey` (`jc:muted_users:{user_id}`) and `mutedSpacesKey` (`jc:muted_spaces:{user_id}`) helpers matching Section 52. `MuteUser`/`UnmuteUser` invalidate `jc:muted_users:{user_id}` after the DB write; `MuteSpace`/`UnmuteSpace` invalidate `jc:muted_spaces:{user_id}`. Invalidation is best-effort (logs and continues on error) — the 10min TTL bounds staleness. Wired `interactionsService.SetValkey(vkClient.Redis())` in `server.go`.

### AUDIT-BE2-VALKEY-002 — Missing i18n distributed Valkey caching — **Already Implemented**
- **Category:** Valkey / Business Logic
- **Severity:** High
- **Location:** internal/i18n/service.go:33-39
- **Finding:** i18n uses in-memory cache only. Section 18 specifies distributed Valkey caching with `jc:i18n:{tenant_id}:{locale}` pattern. Cache is not shared across instances — high cache miss rate under multi-instance deployment.
- **Action:** Add a Valkey cache layer keyed `jc:i18n:{tenant_id}:{locale}` with TTL matching the in-memory TTL; invalidate on string mutation.
- **Fix:** Added a two-layer cache to `i18n.Service`: L1 in-memory (5min TTL, existing) + L2 distributed Valkey (`jc:cache:i18n:{tenant_id}:{locale}`, 1h TTL per Section 18 / catalog). `GetTranslationsByLocale` checks L1 → L2 → DB; on L2 hit, L1 is backfilled. `cacheSet` writes to both layers (L2 write best-effort). `cacheInvalidate` deletes from both layers (L2 DEL best-effort). Added `SetValkey` setter and wired `i18nService.SetValkey(vkClient.Redis())` in `server.go`. Used `jc:cache:i18n:*` prefix per the valkey-keys-catalog (the audit's `jc:i18n:` prefix was not in the catalog).

### AUDIT-BE2-SETTING-001 — Hardcoded pricing plans violate Rule 3 — **Already Implemented**
- **Category:** Settings / Financial
- **Severity:** High
- **Location:** internal/payments/handlers.go:96-150
- **Finding:** Pricing plans (Free/Pro/Business with fixed NGN amounts) are hardcoded instead of loaded from `system_settings`. Violates Rule 3 and makes pricing inflexible.
- **Action:** Move pricing to `system_settings` keys (e.g., `pricing.plan.pro.monthly_amount`, `pricing.plan.pro.yearly_amount`) or a dedicated pricing config table.
- **Fix:** Added 5 new keys to both the JEOS catalog (`system-settings-catalog.md`) and the Go catalog (`internal/control/settings_catalog.go`): `pricing.plan.pro.monthly_amount` (500000), `pricing.plan.pro.yearly_amount` (5000000), `pricing.plan.business.monthly_amount` (1500000), `pricing.plan.business.yearly_amount` (15000000), `pricing.currency` (NGN). Added `SettingsProvider` interface and `SetSettings` to `payments.Handlers`. `ListPricingPlans` now reads all amounts and currency from settings with the catalog defaults as fallbacks. Added `GetStringOr` to `config.Settings`. Wired `paymentHandlers.SetSettings(settingsService)` in `server.go`.

### AUDIT-BE2-SETTING-002 — Affiliate link limit hardcoded — **Already Implemented**
- **Category:** Settings
- **Severity:** High
- **Location:** internal/affiliate/service.go:144
- **Finding:** Affiliate link limit is hardcoded as `if count >= 10` instead of reading `affiliate.max_links_per_user` (catalog default 10).
- **Action:** Replace the hardcoded `10` with a settings read.
- **Fix:** Added `SettingsProvider` interface and `SetSettings` to `affiliate.Service`. `CreateLink` now reads `affiliate.max_links_per_user` (default 10) from settings when wired. Wired `affiliateService.SetSettings(settingsService)` in `server.go`.

### AUDIT-BE2-SETTING-003 — Affiliate commission rates hardcoded — **Already Implemented**
- **Category:** Settings / Financial
- **Severity:** High
- **Location:** internal/affiliate/service.go:61
- **Finding:** Commission rates hardcoded as `CommissionRates{ProMonthly: 15, ProAnnual: 20, Course: 10}` instead of reading `affiliate.commission.pro_monthly`, `affiliate.commission.pro_annual`, `affiliate.commission.course` from the catalog.
- **Action:** Replace hardcoded values with settings reads.
- **Fix:** `GetProgramInfo` now reads `affiliate.commission.pro_monthly` (15), `affiliate.commission.pro_annual` (20), `affiliate.commission.course` (10), `affiliate.min_account_days` (30), and `affiliate.max_links_per_user` (10) from settings when wired, with catalog defaults as fallbacks. Uses the same `SettingsProvider` as SETTING-002.

### AUDIT-BE2-SETTING-004 — Missing affiliate settings in Go catalog — **Already Implemented (No Change Needed)**
- **Category:** Settings
- **Severity:** High
- **Location:** internal/control/settings_catalog.go
- **Finding:** `affiliate.author_commission_default`, `affiliate.unregistered_link_policy`, `affiliate.scan_on_publish` are documented in JEOS Section 55/56 and used in code but missing from the Go settings catalog (Control Panel UI cannot render them). *(Overlaps with 2026-07-26 AUDIT-BE-SETTING-001/002/003 — re-confirmed from affiliate-side audit.)*
- **Action:** Add all three to `internal/control/settings_catalog.go` with metadata.
- **Fix:** Verified all three keys are already present in `internal/control/settings_catalog.go` (lines 107-109), added in a prior session (AUDIT-BE-SETTING-001/002/003). No change needed.

### AUDIT-BE2-TASK-001 — Missing Asynq task tier assignments (affiliate/content) — **Already Implemented (No Change Needed)**
- **Category:** Tasks
- **Severity:** High
- **Location:** internal/worker/enqueuer.go:109-146
- **Finding:** `TypeAffiliateCreditAuthorCommission` and `TypeContentScanAffiliateLinks` are defined with handlers but missing from the `taskTier` map. They default to "default" tier instead of their intended tiers, risking silent queue misrouting. *(Overlaps with 2026-07-26 AUDIT-BE-TASK-003/005/006 — re-confirmed.)*
- **Action:** Add both task types to `taskTier` with correct tier assignments.
- **Fix:** Verified both task types are already in the `taskTier` map: `TypeAffiliateCreditAuthorCommission` → `QueueDefault` (line 135), `TypeContentScanAffiliateLinks` → `QueueLow` (line 162). Also present in `TaskTypeCatalog` (lines 215, 251). Fixed in a prior session — no change needed.

### AUDIT-BE2-TASK-002 — WebSocket ticket HTTP handler not exposed — **Already Implemented (with bug fix)**
- **Category:** Tasks / Routes
- **Severity:** High
- **Location:** internal/websocket/dm.go:38-44
- **Finding:** `CreateWSTicket` exists but no HTTP handler is exposed at `/api/v1/me/ws-ticket` (Section 10). Frontend cannot obtain WebSocket connection tickets.
- **Action:** Wire the HTTP handler in `server.go` and use the `jc:ws_ticket:{id}` Valkey key pattern per catalog.
- **Fix:** The handler IS already wired at `POST /api/v1/auth/ws-ticket` (server.go:1321) — the audit's claimed path `/api/v1/me/ws-ticket` was incorrect; the api-routes-catalog documents `POST /auth/ws-ticket`. However, found and fixed a critical bug: `CreateWSTicket` was `:exec` (no RETURNING), so the handler generated a separate random `ticketID` unrelated to the DB row. The client received a ticket ID that didn't exist in the DB, and the Valkey key was set with the wrong ID. Changed `CreateWSTicket` to `:one` with `RETURNING id, ...`, updated `Store.CreateWSTicket` to return `gen.CreateWSTicketRow`, and updated `GetWSTicket` handler to use the DB-generated `ticket.ID` for both the Valkey key and the client response. The `jc:ws_ticket:{id}` Valkey key pattern was already correct.

### AUDIT-BE2-RBAC-001 — Space moderation permission not dynamically resolved — **Already Implemented**
- **Category:** RBAC / Business Logic
- **Severity:** High
- **Location:** internal/rbac/rbac.go:182-221
- **Finding:** The resolver does not implement dynamic `spaces.moderate.assigned` resolution per Section 03. This permission should be auto-granted by checking the `space_moderators` table at runtime, not stored as a static grant.
- **Action:** Add `space_moderators` table lookup in `HasPermission` / `GetUserContext` to auto-grant `spaces.moderate.assigned` when the user is assigned as moderator for the target space.
- **Fix:** Added `HasSpacePermission(ctx, userID, tenantID, spaceID, permKey)` to `rbac.RBAC` — for `spaces.moderate.assigned`, it short-circuits on wildcard/`spaces.moderate` (all-spaces), then dynamically checks the `space_moderators` table via `GetSpaceModerator(spaceID, userID)`. The assignment record IS the grant (Section 03) — no static RBAC entry needed. Added `RequireSpacePermission` middleware that resolves the space ID from (1) `rbac.SpaceIDContextKey` set by `spaces.SpaceResolver` (for `{slug}` routes), (2) chi URL param `space_id`, (3) chi URL param `id`. Updated `spaces.SpaceResolver` to also set `rbac.SpaceIDContextKey` with the resolved space's UUID. Re-wired the two `spaces.moderate.assigned` routes (`POST /spaces/{slug}/moderators`, `DELETE /spaces/{slug}/moderators/{userId}`) from `RequirePermissionWithInvalidation` to `RequireSpacePermission` in `server.go`.

---

## MEDIUM FINDINGS (27)

### Security / Business Logic

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-SEC-010 | internal/impersonation/service.go:77-83 | `Service.IsAdmin` is a stub always returning true ("For now, all users with the impersonate permission are admins"). Bypasses actual RBAC. | Implement real RBAC check via the rbac resolver. — **Already Implemented**: `IsAdmin` now delegates to `rbac.HasAdminCapability` when wired; returns false defensively when no resolver is configured. |
| AUDIT-BE2-SEC-011 | internal/webhooks/service.go:15-16 | `MaxDeliveryAttempts = 5` hardcoded; should use `notifications.max_delivery_attempts` system setting (Rule 3). | Read from settings at runtime. |
| AUDIT-BE2-SEC-012 | internal/publicapi/middleware.go:129-138 | Public API rate limits hardcoded; should use system settings per Rule 3. | Move to settings. |
| AUDIT-BE2-SEC-013 | internal/linkpreview/fetcher.go:72 | HTTP timeout hardcoded to 10s; should be a system setting. | Read from settings. |

### Business Logic

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-BL-007 | internal/content/handlers.go | `content.max_tags` setting (default 5) not enforced in `AddContentTag`. | Add tag-count validation against the setting. |
| AUDIT-BE2-BL-008 | internal/content/handlers.go:532-605 | `UpdateContent` does not check `content.edit_window_minutes` before allowing edits. | Add edit-window validation. |
| AUDIT-BE2-BL-009 | internal/comments/handlers.go:255-294 | `UpdateComment` does not check `comments.edit_window_minutes`. | Add edit-window validation; set `is_edited` flag after window expires. |
| AUDIT-BE2-BL-010 | internal/follows/ | Section 31: following a space with `joining_perm='open'` should auto-join — not implemented. | Add auto-join in `ToggleFollow` when `entity_type='space'` and `joining_perm='open'`. |
| AUDIT-BE2-BL-011 | internal/bookmarkfolders/ | Folders CRUD exists but bookmarks cannot be assigned to folders — `folder_id` not persisted on bookmark create/update. | Wire `folder_id` through bookmark handlers. |
| AUDIT-BE2-BL-012 | internal/forum/ | Section 08: solved threads should emit QAPage JSON-LD for SEO — not implemented. | Add JSON-LD template rendering for solved threads. |
| AUDIT-BE2-BL-013 | internal/features/features.go:52-62 | `RolloutPercentage` field exists but `IsEnabled` does not use it — no gradual rollouts (Section 42). | Apply percentage-based rollout in `IsEnabled`. |
| AUDIT-BE2-BL-014 | internal/pagebuilder/service.go:284 | Sanitization rules hardcoded via `content.SanitizeHTML` — not configurable via settings. | Make sanitization rules system-setting driven. |

### Settings

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-SETTING-005 | internal/auth/password.go:65-85 | `PasswordPolicyFromSettings` only reads `min_length` and `require_uppercase`; hardcodes `require_number=true`, `require_symbol=false`, `max_length=128`, `breach_check=true`, `history_count=5` instead of reading catalog keys. | Add settings reads for all password policy keys. |
| AUDIT-BE2-SETTING-006 | internal/spaces/ | `spaces.bubble_depth` and `spaces.max_pinned_posts` not read in space handlers. *(Overlaps with 2026-07-26 AUDIT-BE-SETTING-008/009 — re-confirmed from spaces-side.)* | Add settings reads for validation. |
| AUDIT-BE2-SETTING-007 | internal/wallet/wallet.go:44 | `DefaultPointsToCreditsRate = 100` hardcoded; `wallet.points_to_credit_rate` setting exists in catalog. | Read from settings with default fallback. |
| AUDIT-BE2-SETTING-008 | internal/wallet/ (conversion logic) | `wallet.min_convert_points` (default 100) not read from settings. | Read from settings. |

### Tasks

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-TASK-003 | internal/worker/handlers.go:1884-2000 | `HandleRSSRegenerate` writes to local `feed.xml` via `os.WriteFile` — not multi-tenant safe; uses hardcoded SQL bypassing `internal/rss/rss.go` infrastructure. | Refactor to use `internal/rss` generators and store feeds in Valkey (`jc:rss:feed:*`). |
| AUDIT-BE2-TASK-004 | internal/worker/tasks.go, internal/ai/handlers.go | `docs:generate_page` task not in Asynq catalog; AI doc generation runs synchronously in `aiHandlers.HandleGenerate` instead of fire-and-forget. | Add task to catalog; enqueue from handler. |

### Valkey

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-VALKEY-003 | internal/docs/service.go | `jc:docs_nav:{audience}` (TTL 10min) and `jc:docs_page:{audience}:{slug}` (TTL 5min) documented in catalog but not implemented — DB hit on every doc page load. | Implement Valkey caching with invalidation on doc create/update/publish. |
| AUDIT-BE2-VALKEY-004 | internal/valkey/client.go | Package does not enforce `jc:` prefix — relies on developer discipline. High risk of Rule 4 violations. | Add a `MustKey(pattern, args...)` helper that prepends `jc:` and use it across callers. |

### Routes

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-ROUTE-001 | internal/server/server.go:982-987 vs catalog | Catalog specifies `/spaces/{id}/mute` but implementation uses `/spaces/{slug}/mute`. | Update catalog or add ID-based route. |
| AUDIT-BE2-ROUTE-002 | internal/content/handlers.go | Section 51 specifies `POST /api/v1/content/{id}/report` — not wired in content handlers. | Implement or wire to existing reports system. *(Note: 2026-07-26 audit flagged this route exists in server.go:1064 without RBAC — verify wiring vs. RBAC gap.)* |
| AUDIT-BE2-ROUTE-003 | internal/content/handlers.go | Section 49 specifies `POST/GET /api/v1/content/{id}/discussions` for blog-to-discussion linking — not implemented. | Implement thread link creation/listing handlers. |

### Search

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-SEARCH-001 | internal/docs/store.go, internal/search/indexer.go | Docs search uses PostgreSQL `ILIKE` instead of Meilisearch. No `doc_pages` index; `HandleSearchIndexSync` does not handle `doc_page` entity type. No `search:index_entity` enqueue on doc create/update. | Add `doc_page` to search index sync; create `IndexDocPage`; enqueue sync tasks. |
| AUDIT-BE2-SEARCH-002 | internal/content/handlers.go | `GET /content/{id}/related` is wired but handler may not use pgvector cosine similarity per Section 14. | Verify pgvector similarity is used; fix if not. |

### Database

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-DB-001 | internal/mentorship/service.go:45-186 | All mentorship DB ops use raw SQL via `s.db.Reader.Query` instead of sqlc-generated `gen.Queries` — inconsistent with reports/trending/organizations. | Create sqlc query files for mentorship; regenerate. |
| AUDIT-BE2-DB-002 | internal/mentorship/service.go | No Asynq task processor registered for mentorship — verify against Section 35 whether background tasks are required. | If required, implement processor and register in `server.go`. |

### Standards

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-STD-001 | internal/seo/ | Missing `robots.txt` handler (Section 26/43) and `llms.txt` generator (Section 43). Dynamic bot policies cannot be enforced. *(Overlaps with 2026-07-26 AUDIT-BE-VALKEY-001/002/003 — those keys belong here.)* | Implement `internal/seo/robots.go` and `internal/seo/llms.go` driven by the bot registry. |
| AUDIT-BE2-STD-002 | internal/auth/email_verification.go:1 | File contains only a package declaration — empty stub while verification logic lives in `handlers.go`. | Remove the empty file or move verification logic there. |

---

## LOW FINDINGS (14)

### Monitoring

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-MON-001 | internal/auth/handlers.go (multiple) | Auth handlers use `slog.Error` but do not capture to GlitchTip on critical failures (password reset token gen, TOTP decryption errors). | Add `glitchtip.CaptureException` for critical auth failures. |
| AUDIT-BE2-MON-002 | internal/middleware/*.go (error paths) | Middleware (rate limit, idempotency, ban enforcement) logs errors but does not capture to GlitchTip. | Add capture for infrastructure-failure error paths. |

### Business Logic

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-BL-015 | internal/tags/handlers.go | Catalog specifies `GET /api/v1/tags/{slug}/search` (autocomplete) — not implemented. | Implement tag search handler. |
| AUDIT-BE2-BL-016 | internal/university/handlers.go | `price_kobo` not validated as non-negative when set; no currency validation. | Add `price_kobo >= 0` and currency validation. |

### Tasks

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-TASK-005 | internal/worker/affiliate_enhancements.go:37-38 | Affiliate commission crediting relies on a DB `credited` flag, not Asynq idempotency. | Add task to `idempotentTasks` map for defense-in-depth. |
| AUDIT-BE2-TASK-006 | internal/trending/processor.go:83-89 | `ComputeSuggestions` is a no-op (intentional — computed on-demand). Verify alignment with Section 53. | Confirm on-demand computation is acceptable per spec. |

### Standards / Compliance

| ID | Location | Finding | Action |
|----|----------|---------|--------|
| AUDIT-BE2-STD-003 | internal/renderer/renderer.go:122-130 | Cache key uses only 8 bytes of SHA-256 — collision risk. | Use full hash or follow catalog pattern. |
| AUDIT-BE2-STD-004 | internal/linkpreview/fetcher.go:241-244 | Same 8-byte hash truncation issue as renderer. | Use full hash. |
| AUDIT-BE2-STD-005 | internal/linkpreview/fetcher.go | Fixed User-Agent "JapaConnect/1.0 (link preview)" — some sites block unknown UAs. | Add UA rotation / configurable UA. |
| AUDIT-BE2-STD-006 | internal/features/handlers.go:78,230,292,328 | `ClearCache()` called on every update — clears all tenants' caches, not just the affected one. | Scope cache clearing to affected tenant. |
| AUDIT-BE2-STD-007 | internal/pagebuilder/ | Missing widget rendering pattern (Section 16) and default block catalog seeding. | Implement widget renderer; seed default blocks. |
| AUDIT-BE2-STD-008 | internal/bots/ | Missing `registry.go` with default bot seed (Googlebot, Bingbot, etc.); no UA pattern validation; no integration with SEO robots/llms.txt. | Add default registry + validation + SEO integration. |
| AUDIT-BE2-STD-009 | internal/publicapi/ | Missing Swagger UI route (`/api/docs`) and developer self-service portal (Section 47). | Wire Swagger UI; implement developer key management. |
| AUDIT-BE2-STD-010 | internal/webhooks/ | Webhook delivery lacks idempotency protection — possible duplicate deliveries on retry. | Add idempotency key to delivery payloads. |
| AUDIT-BE2-STD-011 | internal/professional/handlers.go:133-134 | Hardcoded error code `upgrade_to_business_required` — acceptable as a code, but the business-requirement check should be configurable if policy is meant to be tunable. | Consider settings-driven policy toggle. |
| AUDIT-BE2-STD-012 | internal/i18n/handlers.go | No language pack import/export functionality (Section 18). | Implement bulk import/export endpoints. |

---

## CLEAN AREAS (No Issues Found)

The following folders/checks passed audit:

### Batch 1 — Auth/Identity/Security
- **internal/csrf** — Double-submit cookie pattern correctly implemented.
- **internal/clientip** — Trusted proxy validation with `JC_TRUSTED_PROXIES` env var.
- **internal/tenant** — Tenant resolution with host/slug matching and default fallback.
- **internal/middleware/ratelimit** — `jc:ratelimit:` prefix, tier config, graceful degradation.
- **internal/middleware/idempotency** — `jc:idempotency:` prefix, SET NX EX atomic pattern.
- **internal/middleware/ban_enforcement** — `jc:ban:` prefix, caching with expiry.
- **internal/middleware/operation_ban** — `jc:restrictions:` prefix, JSON structure.
- **internal/middleware/security_headers** — X-Content-Type-Options, X-Frame-Options, X-XSS-Protection, Referrer-Policy, CSP.
- **internal/middleware/internal_guard** — X-Internal-Token + loopback IP validation.
- **internal/rbac/valkey** — `jc:perm:{user_id}` prefix, 5min TTL, invalidation.
- **internal/auth/bruteforce** — Reads `security.max_login_attempts` and `security.fail_window_minutes`.
- **internal/auth/jwt** — Key rotation with kid header, JTI revocation via `jc:revoked_jti:`.
- **internal/auth/tokens** — crypto/rand for token generation.
- **internal/auth/totp** — AES-256-GCM encryption with `JC_TOTP_ENCRYPTION_KEY`.
- **internal/auth/backup_codes** — SHA-256 hashing, single-use consumption.
- **internal/auth/breach** — HIBP k-anonymity API with rate limiting + caching.
- **internal/auth/oauth** — State tokens, redirect handling, account merging.
- **internal/impersonation/store** — `jc:impersonate:{admin_id}` prefix, 30min TTL.
- **internal/impersonation/handlers** — Audit logging on all actions (Rule 10).
- **internal/geoip/handlers/update/reader** — Mounted at `/api/v1/geo/*`, MaxMind atomic swap.

### Batch 2 — Content/Social
- **internal/comments** (partial) — Nested depth enforcement (max 3), vote upsert with recalc, pin/unpin, soft delete.
- **internal/forum** — Thread/reply CRUD, solved marking, voting, anonymous posting, bookmarks.
- **internal/follows** (partial) — Polymorphic follow, mutual follow detection, bulk status queries.
- **internal/reviews** (partial) — One-per-user enforcement, aggregate recalc on CUD, moderation status.
- **internal/tags** (partial) — CRUD with admin routes, tag follow, moderator assignment.
- **internal/mentions** (partial) — TipTap JSON extraction, filtering for blocks/erased users/private spaces.
- **internal/interactions** (partial) — User block/unblock, mute/unmute validation.
- **internal/spaces** (partial) — CRUD with permission presets, membership, moderator assignment with perm cache invalidation, parent hierarchy.

### Batch 3 — Monetization/Financial
- Idempotency middleware on `POST /payments/intent` and `POST /me/wallet/convert` (Rule 9).
- Webhook signature verification for Paystack, Stripe, NOWPayments.
- Wallet ledger uses advisory locks for balance mutations.
- Credit payment integration delegates to wallet service.

### Batch 6 — AI/Analytics/Misc
- **internal/ai** — Complete handler wiring, Asynq processor registered, sqlc queries, permission gates.
- **internal/analytics** — Handler wiring, Asynq processor, cache flusher, security scan integration.
- **internal/reports** — Handler wiring, sqlc, auto-hide threshold from settings, perm cache invalidation on ban.
- **internal/tickets** — Handler wiring, Asynq enqueuer, permission gates.
- **internal/spam** — Handler wiring, Asynq processor, multi-layer pipeline with graceful degradation, `jc:spam:` keys.
- **internal/trending** — Handler wiring, Asynq processor, velocity scoring.
- **internal/feed** — Handler wiring, hot-reloadable settings, fan-out enqueuer.
- **internal/media** — Handler wiring, ClamAV scan, image/video processors, quarantine workflow.
- **internal/onboarding** — Handler wiring, settings integration for promoted content.
- **internal/organizations** — Handler wiring, sqlc, permission gates, claim ownership flow.
- **internal/notifications** — Handler wiring, batch dispatcher, push delivery.
- **internal/errors** — GlitchTip initialized for server + worker, Asynq error forwarding.

---

## CROSS-REFERENCES TO 2026-07-26 AUDIT

Several findings in this round re-confirm or expand on findings from the previous audit (viewed from a different folder's perspective):

| This Round | 2026-07-26 | Note |
|------------|------------|------|
| AUDIT-BE2-FIN-001 | AUDIT-BE-FIN-001 | Same raw-SQL ledger violation; re-confirmed from affiliate side. |
| AUDIT-BE2-SETTING-004 | AUDIT-BE-SETTING-001/002/003 | Same missing affiliate settings; re-confirmed. |
| AUDIT-BE2-TASK-001 | AUDIT-BE-TASK-003/005/006 | Same missing task tier entries; re-confirmed. |
| AUDIT-BE2-SETTING-006 | AUDIT-BE-SETTING-008/009 | Same unread `spaces.bubble_depth` / `spaces.max_pinned_posts`; re-confirmed from spaces side. |
| AUDIT-BE2-STD-001 | AUDIT-BE-VALKEY-001/002/003 | The unimplemented `jc:llms_txt`, `jc:llms_full_txt`, `jc:robots_txt` keys belong in `internal/seo/` — confirmed missing here. |
| AUDIT-BE2-ROUTE-002 | AUDIT-BE-RBAC-MW-002 | `POST /content/{id}/report` — 2026-07-26 flagged missing RBAC; this round flags it may not be wired in content handlers. Reconcile: route exists in server.go but handler implementation may be incomplete. |

---

## RECOMMENDED ACTIONS (Priority Order)

### Immediate (Critical — fix before production)

1. **AUDIT-BE2-SEC-001/002/005/004** — Impersonation safety + wiring, custom fields SSRF + RBAC, DM privacy. All are exploitable security gaps.
2. **AUDIT-BE2-SEC-003** — Add SSRF protection to custom fields embed resolver.
3. **AUDIT-BE2-FIN-001** — Remove or replace the raw-SQL `UPDATE users SET credits_balance` in `affiliate_enhancements.go:41` with a wallet ledger INSERT.

### High Priority

4. **AUDIT-BE2-SEC-006/007/008/009** — Wire `TOTPRequiredForAdmin`, fix GeoIP proxy validation, validate renderer script path, fix redirects open-redirect.
5. **AUDIT-BE2-BL-001/002/003** — Register/enqueue comment, mentions, and review-recalc Asynq tasks.
6. **AUDIT-BE2-BL-004/005/006** — Space Meilisearch sync, bubble depth, content hash duplicate check.
7. **AUDIT-BE2-VALKEY-001/002** — Mute key patterns + i18n distributed caching.
8. **AUDIT-BE2-SETTING-001/002/003/004** — Move hardcoded pricing, affiliate limits/rates, and missing catalog entries to settings.
9. **AUDIT-BE2-TASK-001/002** — Fix task tier map; wire WebSocket ticket handler.
10. **AUDIT-BE2-RBAC-001** — Dynamic `spaces.moderate.assigned` resolution.

### Medium Priority

11. **AUDIT-BE2-SEC-010 through 013** — Impersonation IsAdmin stub, webhook/publicapi/linkpreview hardcoded values.
12. **AUDIT-BE2-BL-007 through 014** — Edit windows, auto-join, bookmark folders, JSON-LD, feature rollout, sanitization config.
13. **AUDIT-BE2-SETTING-005 through 008** — Password policy completeness, wallet conversion settings.
14. **AUDIT-BE2-TASK-003/004** — RSS regenerate refactor, docs:generate_page task.
15. **AUDIT-BE2-VALKEY-003/004** — Docs caching, Valkey prefix enforcement helper.
16. **AUDIT-BE2-ROUTE-001/002/003** — Space mute path, content report, content discussions.
17. **AUDIT-BE2-SEARCH-001/002** — Docs Meilisearch index, related-content pgvector.
18. **AUDIT-BE2-DB-001/002** — Mentorship raw SQL → sqlc, mentorship Asynq verification.
19. **AUDIT-BE2-STD-001/002** — SEO robots/llms.txt, empty email_verification.go.

### Low Priority

20. **AUDIT-BE2-MON-001/002** — GlitchTip capture in auth/middleware error paths.
21. **AUDIT-BE2-BL-015/016** — Tag search route, learning track price validation.
22. **AUDIT-BE2-TASK-005/006** — Affiliate idempotency, trending no-op verification.
23. **AUDIT-BE2-STD-003 through 012** — Hash truncation, UA rotation, feature cache scoping, widget renderer, bot registry, Swagger UI, webhook idempotency, professional error code, i18n import/export.

---

## AUDIT METADATA

- **Auditor:** Devin (japa-backend-auditor + jc-backend-audit skills)
- **Date:** 2026-07-27
- **Method:** 6 parallel read-only subagents, each auditing a logical batch of `internal/` folders against the relevant JEOS sections and convention catalogs.
- **Folders audited:** 58 (all `internal/` folders not covered by AUDIT-BACKEND-2026-07-26.md)
- **JEOS sections consulted:** 45+ sections and 7 convention files
- **Combined with 2026-07-26 audit:** Both reports together cover 100% of `internal/` + `migrations/` + `cmd/`.
