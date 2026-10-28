# Donations & Tips (§125) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement JEOS §125 — platform/org/member donation & tipping with points/credits ledger settlement, fiat-via-payments settlement, Open Collective sync, and external link surface.

**Architecture:** New `internal/tips` module (handlers → service → gen queries) plus `internal/opencollective` GraphQL client. Tips settle via existing seams: wallet ledger pair (`spend`/`earn`, `source='tip'`), `payments.CreatePayment` with `purpose_type='donation'`, worker `onPaymentSucceeded` dispatch → `tips:settle_fiat`. OC contributions imported by a scheduled task + optional webhook.

**Spec:** `.jeos/sections/125-donations-tips.md` (authoritative — read it first)

**Tech Stack:** Go (chi, pgx/v5, sqlc), Asynq, Valkey, SvelteKit 2 + Svelte 5 + Bits UI (apps/main), bun.

## Global Constraints

- JEOS rules in `.jeos/AGENTS.md` are binding: Rule 2 (never edit `internal/db/gen`), Rule 3 (no hardcoded config — `system_settings`/`JC_*`), Rule 5 (error envelope `{error:{code,message,status}}`), Rule 9 (`Idempotency-Key` on financial POSTs), Rule 16 (frontend consumes API only), Rule 17 (no AI attribution anywhere), Rule 18 (bun, never npm/npx).
- New Asynq task names must be added to `internal/worker` catalog AND `.jeos/sections/11-job-queue-asynq.md`.
- New Valkey keys must be added to `.jeos/conventions/valkey-keys-catalog.md`.
- New routes must be added to `.jeos/conventions/api-routes-catalog.md`.
- New settings keys must be added to `.jeos/conventions/system-settings-catalog.md` and `internal/control/settings_catalog.go`.
- RBAC role slugs: member tier = `'member','pro_member','business_member'`; admin tier = `'super_admin','super_admin_role','admin-group-role'`.
- Migrations: `migrations/NNNNNN_name.up.sql` + `.down.sql`, wrapped in `BEGIN;`/`COMMIT;`. Next number: **000948**.
- sqlc queries live in `internal/db/queries/*.sql` (`-- name: X :one|:many|:exec`), regenerate with `make sqlc-gen`.
- Verify each task: `go build ./... && go vet ./internal/tips ./internal/opencollective` and task-specific tests.

---

### Task 1: Migration 000948 + sqlc queries + regen

**Files:**
- Create: `migrations/000948_donations_tips.up.sql`
- Create: `migrations/000948_donations_tips.down.sql`
- Create: `internal/db/queries/tips.sql`
- Generated: `internal/db/gen/tips.sql.go`, `models.go` (via `make sqlc-gen`)

**Interfaces — produces:** gen queries used by Tasks 2–8: `UpsertTipSettings`, `GetTipSettings`, `GetTipSettingsByOCWebhookSecret`, `ListTipSettingsWithOCToken`, `SetOCCredentials`, `ClearOCCredentials`, `CreateTipExternalLink`, `UpdateTipExternalLink`, `DeleteTipExternalLink`, `ListTipExternalLinks`, `InsertTip`, `GetTip`, `GetTipByPaymentID`, `GetTipByOCContributionID`, `GetTipByIdempotencyKey`, `UpdateTipStatus`, `ListTipsSent`, `ListTipsReceived`, `CountTipsReceived`, `AdminListTips`, `AdminCountTips`, `AdminTipStats`, `SumTipperPointsToday`, `ContentTipSummary`, `GetUserDisplayNames`, `IsMentorProfile`.

- [ ] **Step 1: Write the up migration** — schema verbatim from spec §Schema (three tables, all CHECKs, UNIQUE, indexes — including `tip_settings.oc_webhook_secret`) plus seeds:

```sql
BEGIN;

-- tables from .jeos/sections/125-donations-tips.md §Schema (verbatim)

INSERT INTO permissions (key, description) VALUES
    ('tips.send',     'Send tips/donations to creators, organizations, or the platform'),
    ('tips.receive',  'Enable tips and configure accepted settlement methods'),
    ('tips.moderate', 'Admin tips list, stats, and refund actions')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('member','pro_member','business_member') AND p.key = 'tips.send'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('pro_member','business_member','super_admin','super_admin_role','admin-group-role') AND p.key = 'tips.receive'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id FROM user_roles ur, permissions p
WHERE ur.slug IN ('super_admin','super_admin_role','admin-group-role') AND p.key = 'tips.moderate'
ON CONFLICT DO NOTHING;

INSERT INTO notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category)
VALUES ('tip.received', 'Tip Received', 'Someone tipped you', true, true, true, false, 'billing')
ON CONFLICT (key) DO NOTHING;

COMMIT;
```

- [ ] **Step 2: Write the down migration** — drop indexes/tables in reverse (`tip_external_links`, `tips`, `tip_settings`), `DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (...))`, `DELETE FROM permissions WHERE key IN (...)`, `DELETE FROM notification_types WHERE key = 'tip.received'`.

- [ ] **Step 3: Write `internal/db/queries/tips.sql`**

```sql
-- name: UpsertTipSettings :one
INSERT INTO tip_settings (owner_type, owner_id, tips_enabled, accept_points, accept_credits,
    accept_fiat, accept_external, oc_collective_slug, oc_api_token_encrypted,
    min_tip_amount, thank_you_message)
VALUES (@owner_type, @owner_id, @tips_enabled, @accept_points, @accept_credits,
    @accept_fiat, @accept_external, @oc_collective_slug, @oc_api_token_encrypted,
    @min_tip_amount, @thank_you_message)
ON CONFLICT (owner_type, owner_id) DO UPDATE SET
    tips_enabled = @tips_enabled, accept_points = @accept_points,
    accept_credits = @accept_credits, accept_fiat = @accept_fiat,
    accept_external = @accept_external, oc_collective_slug = @oc_collective_slug,
    oc_api_token_encrypted = COALESCE(@oc_api_token_encrypted, tip_settings.oc_api_token_encrypted),
    min_tip_amount = @min_tip_amount, thank_you_message = @thank_you_message,
    updated_at = now()
RETURNING *;

-- name: GetTipSettings :one
SELECT * FROM tip_settings WHERE owner_type = @owner_type AND owner_id = @owner_id;

-- name: ListTipSettingsWithOCToken :many
SELECT * FROM tip_settings WHERE oc_collective_slug IS NOT NULL AND oc_api_token_encrypted IS NOT NULL;

-- name: SetOCCredentials :exec
UPDATE tip_settings SET oc_collective_slug = @oc_collective_slug,
    oc_api_token_encrypted = @oc_api_token_encrypted, updated_at = now()
WHERE owner_type = @owner_type AND owner_id = @owner_id;

-- name: ClearOCCredentials :exec
UPDATE tip_settings SET oc_collective_slug = NULL, oc_api_token_encrypted = NULL, updated_at = now()
WHERE owner_type = @owner_type AND owner_id = @owner_id;

-- name: CreateTipExternalLink :one
INSERT INTO tip_external_links (tip_settings_id, kind, label, url, position)
VALUES (@tip_settings_id, @kind, @label, @url, @position) RETURNING *;

-- name: UpdateTipExternalLink :one
UPDATE tip_external_links SET kind = @kind, label = @label, url = @url, position = @position
WHERE id = @id AND tip_settings_id = @tip_settings_id RETURNING *;

-- name: DeleteTipExternalLink :exec
DELETE FROM tip_external_links WHERE id = @id AND tip_settings_id = @tip_settings_id;

-- name: ListTipExternalLinks :many
SELECT * FROM tip_external_links WHERE tip_settings_id = @tip_settings_id ORDER BY position, created_at;

-- name: InsertTip :one
INSERT INTO tips (sender_user_id, external_sender_name, recipient_type, recipient_user_id,
    recipient_organization_id, content_id, method, points_amount, credits_amount,
    fiat_amount, fiat_currency, fee_amount, net_credits, payment_id, oc_contribution_id,
    message, is_anonymous, status, idempotency_key)
VALUES (@sender_user_id, @external_sender_name, @recipient_type, @recipient_user_id,
    @recipient_organization_id, @content_id, @method, @points_amount, @credits_amount,
    @fiat_amount, @fiat_currency, @fee_amount, @net_credits, @payment_id, @oc_contribution_id,
    @message, @is_anonymous, @status, @idempotency_key)
RETURNING *;

-- name: GetTip :one
SELECT * FROM tips WHERE id = @id;

-- name: GetTipByPaymentID :one
SELECT * FROM tips WHERE payment_id = @payment_id;

-- name: GetTipByOCContributionID :one
SELECT * FROM tips WHERE oc_contribution_id = @oc_contribution_id;

-- name: GetTipByIdempotencyKey :one
SELECT * FROM tips WHERE sender_user_id = @sender_user_id AND idempotency_key = @idempotency_key;

-- name: GetTipSettingsByOCWebhookSecret :one
SELECT * FROM tip_settings WHERE oc_webhook_secret = @oc_webhook_secret;

-- name: UpdateTipStatus :exec
UPDATE tips SET status = @status, payment_id = COALESCE(@payment_id, payment_id),
    net_credits = COALESCE(@net_credits, net_credits), updated_at = now()
WHERE id = @id;

-- name: ListTipsSent :many
SELECT * FROM tips WHERE sender_user_id = @sender_user_id
ORDER BY created_at DESC LIMIT @page_limit OFFSET @page_offset;

-- name: ListTipsReceived :many
SELECT * FROM tips WHERE recipient_type = 'user' AND recipient_user_id = @recipient_user_id
   AND status = 'completed'
ORDER BY created_at DESC LIMIT @page_limit OFFSET @page_offset;

-- name: CountTipsReceived :one
SELECT count(*)::int FROM tips WHERE recipient_type = 'user' AND recipient_user_id = @recipient_user_id AND status = 'completed';

-- name: SumTipperPointsToday :one
SELECT COALESCE(sum(points_amount),0)::bigint FROM tips
WHERE sender_user_id = @sender_user_id AND method = 'points'
  AND created_at >= (date_trunc('day', timezone('UTC', now()))) AT TIME ZONE 'UTC';

-- name: ContentTipSummary :one
SELECT count(*)::int AS tip_count,
       COALESCE(sum(net_credits),0)::bigint AS credits_total,
       COALESCE(sum(points_amount),0)::bigint AS points_total
FROM tips WHERE content_id = @content_id AND status = 'completed';

-- name: AdminListTips :many
SELECT * FROM tips
WHERE (@status::text = '' OR status = @status)
  AND (@method::text = '' OR method = @method)
  AND (@recipient_type::text = '' OR recipient_type = @recipient_type)
ORDER BY created_at DESC LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountTips :one
SELECT count(*)::int FROM tips
WHERE (@status::text = '' OR status = @status)
  AND (@method::text = '' OR method = @method)
  AND (@recipient_type::text = '' OR recipient_type = @recipient_type);

-- name: AdminTipStats :one
SELECT count(*)::int AS total,
       COALESCE(sum(net_credits) FILTER (WHERE status='completed'),0)::bigint AS credits_settled,
       COALESCE(sum(fiat_amount) FILTER (WHERE status='completed'),0)::bigint AS fiat_minor_settled,
       count(*) FILTER (WHERE method='opencollective')::int AS oc_count
FROM tips WHERE created_at >= @since;

-- name: GetUserDisplayNames :many
SELECT u.id, u.username, up.display_name
FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE u.id = ANY(@ids::uuid[]);

-- name: IsMentorProfile :one
SELECT is_mentor FROM user_profiles WHERE user_id = @user_id;
```

- [ ] **Step 4: Run migration + regen**

Run: `make migrate-up` (or project equivalent — check Makefile) then `make sqlc-gen`
Expected: `internal/db/gen/tips.sql.go` generated; `go build ./...` passes.

- [ ] **Step 5: Commit**

```bash
git add migrations/000948_donations_tips.*.sql internal/db/queries/tips.sql internal/db/gen/
git commit -m "feat(tips): §125 donations & tips schema, RBAC nodes, notification type"
```

---

### Task 2: `internal/tips` package — types, store, settings service

**Files:**
- Create: `internal/tips/tips.go` (types, constants, interfaces)
- Create: `internal/tips/store.go` (gen query wrapper)
- Create: `internal/tips/settings.go` (settings + links service logic)
- Test: `internal/tips/settings_test.go`

**Interfaces — produces:**

```go
package tips

const (
    RecipientPlatform      = "platform"
    RecipientUser          = "user"
    RecipientOrganization  = "organization"
    MethodPoints           = "points"
    MethodCredits          = "credits"
    MethodOpenCollective   = "opencollective"
    StatusPending          = "pending"
    StatusCompleted        = "completed"
    StatusFailed           = "failed"
    StatusRefunded         = "refunded"
    LedgerSourceTip        = "tip"
    PlatformOwnerID        = "00000000-0000-0000-0000-000000000000" // uuid sentinel
)

var (
    ErrTipsDisabled        = errors.New("tips are disabled")
    ErrRecipientNoTips     = errors.New("recipient does not accept tips")
    ErrMethodNotAccepted   = errors.New("recipient does not accept this tip method")
    ErrBelowMinimum        = errors.New("tip below recipient minimum")
    ErrSelfTip             = errors.New("cannot tip yourself")
    ErrInsufficientBalance = errors.New("insufficient balance")
    ErrDailyPointsLimit    = errors.New("daily points tip limit reached")
    ErrNotEligible         = errors.New("tips.receive permission or mentor profile required")
    ErrNotFound            = errors.New("tip or settings not found")
)

type Settings struct {
    ID pgtype.UUID; OwnerType string; OwnerID pgtype.UUID
    TipsEnabled, AcceptPoints, AcceptCredits, AcceptFiat, AcceptExternal bool
    OCCollectiveSlug *string
    HasOCToken       bool   // derived: oc_api_token_encrypted IS NOT NULL — token never leaves service
    OCWebhookSecret  *string
    MinTipAmount     int64
    ThankYouMessage  *string
}
type ExternalLink struct { ID pgtype.UUID; Kind, Label, URL string; Position int32 }
type UpdateSettingsInput struct { TipsEnabled *bool; AcceptPoints, AcceptCredits, AcceptFiat, AcceptExternal *bool; MinTipAmount *int64; ThankYouMessage *string }

// Service seam types used by later tasks — declared now:
type TipDTO struct { /* tip row fields + sender_display_name, recipient_display_name */ }
type CreateTipInput struct {
    RecipientType string; RecipientUserID, RecipientOrgID, ContentID pgtype.UUID
    Method string; PointsAmount, CreditsAmount, FiatAmount int64; FiatCurrency string
    Message string; IsAnonymous bool; IdempotencyKey string
}
type CreateTipResult struct { Tip TipDTO; CheckoutURL string } // CheckoutURL only for fiat

// Dependencies (narrow seams, wired in server.go):
type SettingsReader interface { GetIntOr(ctx, key string, fb int) int; GetBoolOr(ctx, key string, fb bool) bool; GetStringOr(ctx, key, fb string) string }
type BalanceReader interface { /* resolves current ledger balance */ }
type PermChecker interface { Can(ctx, userID pgtype.UUID, node string) bool } // rbac resolver
type MentorChecker interface { IsMentor(ctx, userID pgtype.UUID) (bool, error) } // user_profiles.is_mentor — gen query:
//   -- name: IsMentorProfile :one
//   SELECT is_mentor FROM user_profiles WHERE user_id = @user_id;
```

- [ ] **Step 1: Write failing test** `settings_test.go` — table tests: `PublicConfig` hides `oc_api_token_encrypted` (never serialized, only `has_oc_token`); `effectiveMin(method, settings)` picks per-method floor; `checkReceiveEligibility` passes for perm holder OR `is_mentor` user, fails otherwise; owner validation rejects platform↔user-id mixups.

- [ ] **Step 2: Run `go test ./internal/tips/` — expect compile fail.**

- [ ] **Step 3: Implement `tips.go` + `store.go` + `settings.go`.** Store embeds `*gen.Queries` read/write split like `wallet.Store`. `settings.go` implements `GetSettings`, `UpsertSettings` (creates row on first save), `PublicConfig(owner)` → `{tips_enabled, methods:[…accepted], min_tip_amount, oc_collective_slug, external_links, thank_you_message}`, `EnsurePlatformSettings` (upsert with `PlatformOwnerID` uuid), link CRUD (`max 10 links`, `kind`/`url` validation, `https?://` only), `CanReceive(ctx,userID)` = `perm(tips.receive) || IsMentorProfile(user_id)`.

- [ ] **Step 4: `go test ./internal/tips/ -v` — PASS. `go build ./...` clean.**

- [ ] **Step 5: Commit** — `feat(tips): §125 settings service, owner configs, eligibility seam`

---

### Task 3: Settings/config HTTP handlers + routes

**Files:**
- Create: `internal/tips/handlers.go` (all handlers incl. public + admin)
- Modify: `internal/server/server.go` — route mounts (see Task 9 wiring block; may be folded into Task 9 if preferred, but mount at least the settings routes here)
- Test: `internal/tips/handlers_test.go`

**Interfaces — consumes:** `Service` from Task 2. **Produces:** handler names used in server wiring: `GetMyTipSettings`, `UpdateMyTipSettings`, `CreateMyTipLink`, `UpdateMyTipLink`, `DeleteMyTipLink`, `PublicUserTipConfig`, `PublicOrgTipConfig`, `PublicDonate`, `AdminGetDonationSettings`, `AdminUpdateDonationSettings`, plus `GetOrgTipSettings`/`UpdateOrgTipSettings` + org link CRUD.

- [ ] **Step 1: Failing tests** — handler test with fake service: `UpdateMyTipSettings` without `tips.receive` perm → 403 `tips_not_eligible`; org PUT by non-admin member → 403; `PublicUserTipConfig` for user with tips disabled → `{enabled:false}` (not 404, so frontend hides button); admin donation-settings PUT writes platform row.

- [ ] **Step 2: Implement handlers** — follow `wallet/handlers.go` conventions: `requireUser` via `auth.AuthenticatedUser` ctx, `errpkg.WriteError(w, status, "snake_code", "message")`, `errpkg.WriteJSON(w, 200, {"data": …})`. Org writes: resolve org → caller must be `organizations.owner_user_id` OR hold org role `admin` via the seam `OrgRoleChecker interface { MemberRole(ctx context.Context, orgID, userID pgtype.UUID) (string, error) }` (satisfied by `*organizations.Service.MemberRole`; roles are `admin|editor|member`).

- [ ] **Step 3: Mount in `server.go`** (member + admin route blocks):

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.receive")).
    Route("/me/tip-settings", func(r chi.Router) {
        r.Get("/", tipsHandlers.GetMyTipSettings)
        r.Put("/", tipsHandlers.UpdateMyTipSettings)
        r.Post("/links", tipsHandlers.CreateMyTipLink)
        r.Patch("/links/{id}", tipsHandlers.UpdateMyTipLink)
        r.Delete("/links/{id}", tipsHandlers.DeleteMyTipLink)
    })
// public
r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "loose")).Get("/users/{username}/tip-config", tipsHandlers.PublicUserTipConfig)
r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "loose")).Get("/organizations/{id}/donation-config", tipsHandlers.PublicOrgTipConfig)
r.With(mw.RateLimit(vkClient.Redis(), "loose")).Get("/donate", tipsHandlers.PublicDonate)
// admin (tips.moderate or admin.donation_settings — use tips.moderate)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.moderate")).Get("/admin/donation-settings", tipsHandlers.AdminGetDonationSettings)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.moderate")).Put("/admin/donation-settings", tipsHandlers.AdminUpdateDonationSettings)
// org-scoped (org-admin check inside handler; requireActiveOrgByID resolves org + 404s inactive)
r.With(auth.RequireAuth(kr, vkClient.Redis()), requireActiveOrgByID(orgService)).Route("/organizations/{id}/tip-settings", func(r chi.Router) {
    r.Get("/", tipsHandlers.GetOrgTipSettings)
    r.Put("/", tipsHandlers.UpdateOrgTipSettings)
    r.Post("/links", tipsHandlers.CreateOrgTipLink)
    r.Patch("/links/{id}", tipsHandlers.UpdateOrgTipLink)
    r.Delete("/links/{id}", tipsHandlers.DeleteOrgTipLink)
})
```

- [ ] **Step 4: tests pass; `go build ./...` clean.**
- [ ] **Step 5: Commit** — `feat(tips): §125 settings + public config routes`

---

### Task 4: `internal/opencollective` client + OC connect/disconnect

**Files:**
- Create: `internal/opencollective/client.go`
- Create: `internal/tips/oc.go` — `ConnectOC(ctx, owner, slug, token string)`, `DisconnectOC`, `SyncCollective(ctx, settingsID)`, `ImportContribution`
- Modify: `internal/tips/handlers.go` — `ConnectMyOC`, `DisconnectMyOC` (+ org variants)
- Test: `internal/opencollective/client_test.go` (httptest fake GraphQL), `internal/tips/oc_test.go`

**Interfaces — produces:**

```go
package opencollective

type Contribution struct {
    ID           string    // opaque OC id — tips.oc_contribution_id dedupe key
    AmountCents  int64
    Currency     string
    FromName     string
    FromSlug     string
    FromEmail    string    // may be empty — OC only exposes emails the token is scoped to see
    CreatedAt    time.Time
}
type Client struct{ /* http.Client, baseURL default https://api.opencollective.com/graphql/v2 */ }
func (c *Client) VerifyToken(ctx context.Context, token string) (handle string, err error) // { me { slug } }
func (c *Client) ListContributions(ctx context.Context, slug, token string, limit, offset int) ([]Contribution, error)
```

GraphQL (POST `{"query": "…", "variables": …}` to `https://api.opencollective.com/graphql/v2`, `Personal-Token: <token>` header — personal tokens; `Api-Key` is the older header variant, accept either config):

```graphql
query($slug: String!, $limit: Int!, $offset: Int!) {
  account(slug: $slug) {
    transactions(type: CREDIT, limit: $limit, offset: $offset,
                 orderBy: {field: CREATED_AT, direction: DESC}) {
      totalCount
      nodes { id type kind createdAt
              amount { valueInCents currency }
              fromAccount { name slug } }
    }
  }
}
```

Import only `kind == "CONTRIBUTION"` nodes (skips host-fee shares/other credit
kinds). User matching: try adding `emails` to `fromAccount`; if OC returns a
schema/permission error for that field, retry the same query without it —
`sender_user_id` stays NULL when no `users.email` match (spec allows NULL).

- [ ] **Step 1: Failing tests** — client test fakes GraphQL JSON (parse + pagination); `ImportContribution` dedupes on `oc_contribution_id` (second import = no-op), matches `sender_user_id` via a `UserByEmail` seam, leaves NULL when unmatched.

- [ ] **Step 2: Implement.** `ConnectOC`: if token supplied → `VerifyToken` → AES-256-GCM encrypt via `internal/crypto` (same call sites as BYOK keys — check `crypto.Encrypt` signature) → `SetOCCredentials`. Generates `oc_webhook_secret` (random hex 32) stored on the settings row — column ships in the 000948 DDL. Slug-only connect stores slug, no token (link-only mode).

- [ ] **Step 3: Handlers `PUT/DELETE /me/tip-settings/opencollective` + org subpath** — `PUT` body `{collective_slug, api_token?}`; response never returns token, only `has_token`, `webhook_url` (`{site}/api/v1/tips/webhook/opencollective?s=<secret>` when token present).

- [ ] **Step 4: tests pass; build clean.**
- [ ] **Step 5: Commit** — `feat(tips): §125 Open Collective client + collective connect`

---

### Task 5: `POST /tips` — points & credits settlement

**Files:**
- Create: `internal/tips/create.go` — `CreateTip`, validation, ledger pair
- Create: `internal/tips/ledger.go` — transfer math + fee
- Modify: `internal/tips/handlers.go` — `CreateTip` handler
- Test: `internal/tips/create_test.go`

**Interfaces — consumes:** Task 2 service + eligibility; wallet pattern `pg_advisory_xact_lock(hashtextextended($1,0))`, `gen.GetLatestBalance`, `gen.InsertLedgerEntry`. **Produces:** `Service.CreateTip(ctx, senderID, CreateTipInput) (CreateTipResult, error)`; `Notifier` seam `Notify(ctx, recipientUserID, tipID)` (wired to enqueue `tips:notify_received` in Task 9).

- [ ] **Step 1: Failing tests** (fake store via narrow interface — define `tipsStore` interface satisfied by `*Store` so unit tests don't need DB):
  - points tip transfers sender→recipient minus 0 fee, both ledger ops written, `source='tip'`, `reference=tip.id`, balances correct
  - credits tip with `fee_percent=10` → recipient gets 90, fee row on tip = 10
  - self-tip → `ErrSelfTip`; disabled recipient → `ErrRecipientNoTips`; unaccepted method → `ErrMethodNotAccepted`; `SumTipperPointsToday` ≥ limit → `ErrDailyPointsLimit`; insufficient balance → `ErrInsufficientBalance`
  - ledger lock order: sender/recipient locked in `min(user_id)` order to prevent deadlock — test asserts lock call order via fake

- [ ] **Step 2: Implement.** Single tx: lock both wallets in deterministic (uuid-ascending) order → debit sender (`spend`, `ledger_type=points|credits`, `source='tip'`) → credit recipient (`earn`, amount−fee) → `InsertTip` status `completed` → commit → enqueue notify (fire-and-forget post-commit). Idempotency: reuse request `Idempotency-Key` ctx value (`middleware.IdempotencyKeyFromContext`) as `idempotency_key`; on re-entry with same key return existing tip (query by sender+key — add `GetTipByIdempotencyKey` query: `SELECT * FROM tips WHERE sender_user_id=@s AND idempotency_key=@k`).

- [ ] **Step 3: Handler** — `POST /api/v1/tips` (auth + `tips.send` + `strict` rate limit): enforce `Idempotency-Key` presence (`400 missing_idempotency_key` when `middleware.IdempotencyKeyFromContext(ctx)` is empty — Rule 9); parse body, route `points|credits` → `CreateTip`; fiat methods → Task 6; `opencollective`/`external` methods → 400 `invalid_method` (not client-initiated). Feature gate `tips.enabled` → 403 `tips_disabled`.

- [ ] **Step 4: tests pass; build clean.**
- [ ] **Step 5: Commit** — `feat(tips): §125 points/credits tip settlement via wallet ledger`

---

### Task 6: Fiat tips — payment intent + `tips:settle_fiat` + refund

**Files:**
- Create: `internal/tips/fiat.go` — fiat `CreateTip` path, `SettleFiatTip`, `RefundTip`, fee/FX math
- Modify: `internal/tips/handlers.go` — admin `AdminRefundTip` + fiat branch in `CreateTip`
- Modify: `internal/worker/tasks.go`, `internal/worker/enqueuer.go`, `internal/worker/handlers.go`, `internal/worker/server.go` (mux `HandleFunc`), `internal/server/server.go` (adapter wiring)
- Test: `internal/tips/fiat_test.go`

**Interfaces — consumes:** `payments.Service.CreatePayment(ctx, userID, CreatePaymentInput)` (same shape as wallet's `PaymentCreator`: `{Provider, PurposeType, PurposeID, Amount, Currency, Metadata}` → `{ID, CheckoutURL, …}`); `currencies.credits_per_unit` via gen query `GetCurrencyByCode`; `middleware.IdempotencyKeyFromContext`. **Produces:** worker adapter `tips.SettlementAdapter{SettleFiat(ctx, tipID, paymentID)}` satisfying `worker.TipSettlement` interface; `Service.RefundTip(ctx, tipID, adminID)`.

- [ ] **Step 1: Failing tests** — fiat `CreateTip` creates pending tip + calls PaymentCreator with `purpose_type='donation'`, `purpose_id=tip.id`, returns `CheckoutURL`; `SettleFiatTip` computes `net = fiat_amount / 10^exp × credits_per_unit × (1 − fee%)` (table: USD 1000¢ @ `credits_per_unit=10` @ fee 10% → 100 gross → **90 net**; JPY ¥5000 @ `credits_per_unit=2`, fee 0 → **10000 net**); idempotent (already `completed` → no-op, no second ledger entry); `RefundTip` on completed tip → clawback ledger entry + status `refunded`; refund on pending → error.

- [ ] **Step 2: Implement `fiat.go`.** Provider set = `stripe|paystack|nowpayments|manual_transfer` (validate against `tips.method` CHECK). Bounds: `tips.min_fiat_minor`/`tips.max_fiat_minor`. On `payments` success the payment row is attached to tip (`payment_id`). Currency exponent map: keep tiny `currencyExponent(code) int` (NGN,USD,EUR,GBP→2; JPY→0; default 2) — no new table.

- [ ] **Step 3: Worker wiring** — `tasks.go`: `TypeTipsSettleFiat = "tips:settle_fiat"`, `TypeTipsNotifyReceived = "tips:notify_received"`, `TypeTipsOCSync = "tips:oc_sync"`. `enqueuer.go`: `taskTypeCatalog` rows (`tips:settle_fiat`→critical? use `default`; `tips:notify_received`→default; `tips:oc_sync`→low) + `taskTier` + add to `idempotentTasks` as needed. `handlers.go` `onPaymentSucceeded`: add

```go
if p.PurposeType == "donation" && p.PurposeID.Valid {
    payload, _ := json.Marshal(map[string]string{
        "tip_id": p.PurposeID.String(), "payment_id": p.ID.String()})
    if err := taskEnqueuer.Enqueue(TypeTipsSettleFiat, payload); err != nil {
        slog.Error("task:payment:process_webhook: tips settle enqueue failed", "payment_id", p.ID, "error", err)
    }
}
// refunded donation → clawback
if p.PurposeType == "donation" && newStatus == "refunded" { /* enqueue TypeTipsClawback */ }
```

`server.go` mux: `mux.HandleFunc(TypeTipsSettleFiat, HandleTipsSettleFiat)` etc. Handlers call `tipSettler` package-var adapter registered via `worker.RegisterTipSettler(s)` (follow `RegisterTaskEnqueuer` pattern); refund-side hook: find where `payment.refunded` transitions occur (`updatePaymentStatus` call sites in webhook handler) and enqueue `tips:clawback` (`TypeTipsClawback = "tips:clawback"`).

- [ ] **Step 4: tests pass; `go build ./...` clean.**
- [ ] **Step 5: Commit** — `feat(tips): §125 fiat tips via donations payments + settle/clawback tasks`

---

### Task 7: OC sync task + inbound webhook

**Files:**
- Create: `internal/worker/tips_oc.go` — `HandleTipsOCSync`
- Modify: `internal/worker/server.go` (scheduler `schedule("*/60 * * * *", TypeTipsOCSync, time.Hour)` — default 60m; `JC_SCHEDULER_CRON_TIPS__OC_SYNC` override already supported by `SchedulerCronSpecs`), `internal/tips/oc.go` (`SyncAll` iterating `ListTipSettingsWithOCToken` with `jc:tips:oc_sync_lock:{owner_id}` SET NX EX)
- Modify: `internal/tips/handlers.go` — `HandleOCWebhook`
- Modify: `internal/server/server.go` — `r.With(mw.RateLimit(vkClient.Redis(), "webhook")).Post("/tips/webhook/opencollective", tipsHandlers.HandleOCWebhook)`
- Test: `internal/tips/oc_sync_test.go`

- [ ] **Step 1: Failing tests** — `SyncAll` skips locked owners, decrypts token, imports new contributions only (dedupe), updates no rows when OC returns empty; webhook with wrong `?s=` → 401; valid secret + contribution payload → tip row.

- [ ] **Step 2: Implement** — webhook resolves `tip_settings` by `oc_webhook_secret` (add `GetTipSettingsByOCWebhookSecret` query) then calls `ImportContribution`. OC webhook payload shape: `{type: "collective.transaction.created", data: {transaction: {…}}}` — map the same `Contribution` fields; unknown types → 200 no-op (always 200 to avoid OC retry storms except bad secret).

- [ ] **Step 3: tests pass; build clean.**
- [ ] **Step 4: Commit** — `feat(tips): §125 OC contribution sync + inbound webhook`

---

### Task 8: Lists, tip-summary, admin surface

**Files:**
- Modify: `internal/tips/handlers.go` — `ListMySent`, `ListMyReceived`, `ContentTipSummary`, `AdminListTips`, `AdminGetTip`, `AdminTipStats`
- Modify: `internal/server/server.go` mounts
- Test: `internal/tips/handlers_test.go` additions

- [ ] **Step 1: Failing tests** — received list masks `sender_user_id`/name→`"Someone"` when `is_anonymous`; sent list always shows; `ContentTipSummary` counts only `completed`; admin list filters compose.

- [ ] **Step 2: Implement** — DTOs join `users` for display names via a `UserNames` seam (`map[uuid]string` batch lookup query `GetUserDisplayNames :many`); pagination identical to wallet handlers (`parsePageSize`).

- [ ] **Step 3: Mounts:**

```go
r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/tips/sent", tipsHandlers.ListMySent)
r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/me/tips/received", tipsHandlers.ListMyReceived)
r.With(auth.OptionalAuth(kr, vkClient.Redis()), mw.RateLimit(vkClient.Redis(), "loose")).Get("/content/{id}/tip-summary", tipsHandlers.ContentTipSummary)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.moderate")).Get("/admin/tips", tipsHandlers.AdminListTips)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.moderate")).Get("/admin/tips/stats", tipsHandlers.AdminTipStats)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.moderate")).Get("/admin/tips/{id}", tipsHandlers.AdminGetTip)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.moderate")).Post("/admin/tips/{id}/refund", tipsHandlers.AdminRefundTip)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "tips.send"), mw.RateLimit(vkClient.Redis(), "strict")).Post("/tips", tipsHandlers.CreateTip)
```

- [ ] **Step 4: tests pass; build clean.**
- [ ] **Step 5: Commit** — `feat(tips): §125 tip history, content summary, admin surface`

---

### Task 9: Feature flags, notification emission, JEOS catalog sync

**Files:**
- Modify: `internal/features/registry.go` — `{Key:"tips.enabled", Label:"Tips & Donations", Category:"Commerce", SettingsCategory:"tips", Default:false}`, `{Key:"tips.opencollective", Label:"Open Collective Tips", Category:"Commerce", Default:false}`
- Modify: `internal/tips/notify.go` (new) — `NotifyReceived` building `tip.received` notification via `notifications.Service.CreateNotification`
- Modify: `internal/control/settings_catalog.go` — `tips.fee_percent`, `tips.points_daily_limit`, `tips.min_fiat_minor`, `tips.max_fiat_minor`, `tips.oc_sync_interval_minutes`, `tips.oc_platform_collective`, `tips.oc_platform_token` (`Sensitive:true, Panel:"control"`)
- Modify: `.jeos/conventions/api-routes-catalog.md`, `system-settings-catalog.md`, `valkey-keys-catalog.md`, `permission-key-naming.md` (`tips.send|receive|moderate`), `.jeos/schemas/MIGRATION-ORDER.md` (000948 entry), `.jeos/sections/11-job-queue-asynq.md`, `.jeos/JEOS.md` index row `| 125 | sections/125-donations-tips.md | … |`
- Modify: `internal/server/server.go` — full service construction: `tipsService := tips.NewService(database)` + `SetSettings`, `SetPermChecker(rbacResolver-adapter)`, `SetPaymentsService(paymentsService)`, `SetOCClient(opencollective.NewClient())`, `SetNotifier`, `SetEnqueuer`; `worker.RegisterTipSettler(tips.NewSettlementAdapter(tipsService))`

- [ ] **Step 1: Wire service in server.go** — placement next to `walletService` block (~line 904); adapters declared file-local like `walletCacheInvalidator`.

- [ ] **Step 2: Flags + settings catalog + notification type constant** — add `NotificationTypeTipReceived = "tip.received"` in `internal/notifications/notifications.go` constants block + test expectations.

- [ ] **Step 3: JEOS catalog updates** (separate commit to `.jeos` repo): routes (20 rows), settings (7 keys), Valkey (`jc:tips:oc_sync_lock:{owner_id}`, `jc:tips:idem:{key}` — note: actual idempotency keys come from shared `jc:idempotency:` middleware; spec/catalog note this and record only the sync-lock), §11 tasks (3 rows), JEOS.md index.

- [ ] **Step 4: `go build ./... && go test ./internal/tips/ ./internal/worker/ -count=1`**
- [ ] **Step 5: Commit** — `feat(tips): §125 wiring, flags, settings catalog; docs(jeos): §125 catalogs`

---

### Task 10: apps/main — tips API client + `TipDialog`

**Files:**
- Create: `apps/main/src/lib/api/tips.ts` — typed fetchers (`apiFetch` wrapper used elsewhere in app — match existing `lib/api/*` convention)
- Create: `apps/main/src/lib/components/tips/TipDialog.svelte` — Bits UI `Dialog`: amount presets [1,5,10,25,50] + custom input, method radio rendered from `tip-config` (`points|credits|fiat providers|OC|external links`), optional message, `is_anonymous` checkbox, submit → `POST /api/v1/tips` with `Idempotency-Key: crypto.randomUUID()`; fiat → redirect to returned `checkout_url`; external/OC options render outbound `rel="noopener"` links
- Create: `apps/main/src/lib/components/tips/TipButton.svelte` — trigger + dialog wrapper, fetches `tip-config` on open, hidden when `enabled=false`

- [ ] **Step 1:** Inspect an existing dialog component in `apps/main` (e.g. report/bookmark dialog) for Bits UI usage and copy conventions (literals via i18n, not hardcoded copy — check `lib/i18n` usage).
- [ ] **Step 2:** Implement components + client types mirroring backend DTOs.
- [ ] **Step 3:** `bun run check` (svelte-check) clean for new files.
- [ ] **Step 4: Commit** — `feat(main): §125 tip dialog + api client`

### Task 11: apps/main — tip entry points

**Files:**
- Modify: profile page component (user profile header — find under `apps/main/src/routes/(app)/[username]/` or profile components dir)
- Modify: content action rail component (reactions/bookmark row)
- Modify: mentor profile / service listing sidebar components
- Create: `apps/main/src/routes/(public)/donate/+page.svelte` + `+page.server.ts` (load `GET /api/v1/donate` — public; render methods, OC embed/link, external links; fiat form posts to `POST /api/v1/tips` requiring login → redirect to `/login?next=/donate` when 401)

- [ ] **Step 1:** Mount `TipButton` with `recipientType='user'|…` props at each surface; content rail passes `content_id`.
- [ ] **Step 2:** `/donate` page — platform config + method CTAs; unauthenticated tip attempt → login redirect with `next` param.
- [ ] **Step 3:** `bun run check` clean.
- [ ] **Step 4: Commit** — `feat(main): §125 tip buttons on profiles/content/mentors + /donate`

### Task 12: apps/main — dashboard tips settings + history

**Files:**
- Create: `apps/main/src/routes/(app)/me/tips/+page.svelte` (+ `+page.server.ts`/`+page.ts`) — received/sent tabs, paginated lists from `/me/tips/*`
- Create: `apps/main/src/routes/(app)/settings/tips/+page.svelte` — settings form (`GET/PUT /me/tip-settings`): enable toggle, per-method checkboxes, min amount, thank-you message, external links repeater CRUD, OC connect card (slug + token input → `PUT /me/tip-settings/opencollective`; shows `webhook_url` for copy when connected)
- Modify: settings nav (add "Tips & Donations" entry — find the settings sidebar/nav config; must come from API or existing nav conventions, not hardcoded beyond static nav which is acceptable if that's the established pattern)

- [ ] **Step 1:** Implement settings form — show only when user holds `tips.receive` (check via `GET /api/v1/me/permissions` or the app's existing permission store; if no such endpoint, gate render on `tip-settings` GET 403 → show "not eligible" state).
- [ ] **Step 2:** Received/sent lists — amount+method badge, anonymous sender shows "Someone".
- [ ] **Step 3:** `bun run check` clean.
- [ ] **Step 4: Commit** — `feat(main): §125 dashboard tips settings + history`

### Task 13: apps/admin — tips DataTable + donation settings; apps/control verify

**Files:**
- Create: `apps/admin/src/routes/(app)/tips/+page.svelte` — DataTable (full spec: sort/search/filter[status,method,recipient]/paginate, refund action with confirm dialog) consuming `/admin/tips` + `/admin/tips/stats` summary cards
- Create: `apps/admin/src/routes/(app)/tips/settings/+page.svelte` — platform donation settings form (`GET/PUT /admin/donation-settings`)
- Modify: admin nav registry
- Verify: apps/control Extensions page lists `tips.enabled` + `tips.opencollective` automatically from `features.Registry` (no code needed beyond registry entry); OC platform token appears under sensitive settings (`Panel:"control"`)

- [ ] **Step 1–4:** Implement, `bun run check`, commit — `feat(admin): §125 tips moderation + donation settings`

### Task 14: Verification sweep

- [ ] `go build ./... && go vet ./internal/tips ./internal/opencollective ./internal/worker`
- [ ] `go test ./internal/tips/ ./internal/opencollective/ -count=1`
- [ ] `make sqlc-gen` produces no diff (queries already generated)
- [ ] Route smoke: `GET /api/v1/donate`, `GET /api/v1/users/{u}/tip-config`, `POST /api/v1/tips` (points+credits+fiat paths) against dev DB
- [ ] `bun run check` clean in `apps/main`, `apps/admin`
- [ ] Catalog drift check: every new route in api-routes-catalog, task in §11, key in valkey/settings catalogs, node in §03-style list (permissions seeded in migration)
- [ ] Rule 17 sweep: no AI attribution in code/commits/comments
- [ ] Final commit + summary

---

## Self-review notes

- Spec coverage: all §125 sections map to tasks (schema→T1, settings→T2/3, OC→T4/7, points/credits→T5, fiat→T6, lists/admin→T8, flags/settings/notifications→T9, frontend→T10–13, GDPR → covered by pseudonymization note — erasure hook deferred to a follow-up migration task list inside Task 14 notes).
- Known deferred items (call out, don't silently drop): GDPR erasure pseudonymization wiring for `tips`/`tip_settings` (add to §19 erasure task in a follow-up); mobile (apps/mobile) tip surfaces (v2.1 fast-follow); leaderboard/public tip wall (not in spec).
