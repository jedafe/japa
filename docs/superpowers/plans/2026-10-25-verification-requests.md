# Verification Requests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let frontend users request a verification badge for an organization they own/administer or for their own profile; requests carry Form Builder evidence and queue for admin review; badges are granted in a separate admin step.

**Architecture:** New `verification_requests` table + `internal/verification` package mirroring the claim-ownership flow (`internal/organizations`). User-profile badges ship the ADR-029 columns (`users.verified`, `users.verification_type`) plus admin grant routes. Admin review/gates reuse `admin.moderation`; a new `verification.request` node gates submission.

**Tech Stack:** Go (chi, sqlc/pgx, asynq), PostgreSQL, SvelteKit 2 + Svelte 5 (main + admin apps), bun.

## Global Constraints

- **JEOS before code** — Task 11 updates JEOS; section/catalog edits in earlier tasks are noted inline where required by the task itself (e.g. catalog entry alongside settings key seed).
- **Rule 5** — all API errors use `errpkg.WriteError(w, status, "snake_case_code", "message")`.
- **Rule 16** — no hardcoded data on frontend; badge types come from `GET /verification-requests/types`, the form from `GET /forms/{slug}/embed`.
- **Rule 17** — no AI attribution anywhere (commits, comments, docs).
- **Rule 18** — bun only: `bun run check`, `bunx svelte-check`. Never npm/npx.
- **sqlc** — never hand-edit `internal/db/gen/`; edit `db/queries/*.sql` then `make sqlc-gen`.
- Commits go to `main`.
- Spec: `docs/superpowers/specs/2026-10-25-verification-requests-design.md`.
- Reference implementation to mirror: claim-ownership — `internal/organizations/{service,handlers,store}.go`, `apps/main/src/routes/(app)/claim-ownership/`, `apps/admin/src/routes/organisations/claims/+page.svelte`.

---

### Task 1: Migration — schema + seeds

**Files:**
- Create: `migrations/000933_verification_requests.up.sql`
- Create: `migrations/000933_verification_requests.down.sql`

**Interfaces:**
- Produces: `verification_requests` table; `users.verified`, `users.verification_type` columns; permission `verification.request`; seeded form slug `verification-request`; settings keys `verification.form_slug`, `verification.fee_credits`.

- [ ] **Step 1: Write the up migration**

Mirror `migrations/archive/000385_claim_form_paid_submissions.up.sql` (seed style) and `migrations/archive/000144_seed_rbac_gate_permissions.up.sql` (permission grant style). Latest migration is `000932`.

```sql
-- 000933_verification_requests.up.sql
-- User-initiated verification badge requests for organizations and profiles
-- (JEOS §07 Verification Requests). Requests carry Form Builder evidence
-- (form_submissions) and queue for admin review; badge grant stays a
-- separate admin action (two-step). Also ships the ADR-029 user-profile
-- badge columns (users.verified / users.verification_type).
--
-- Ref: sections/07-organization-profiles-claim-ownership.md (Verification Requests)

BEGIN;

-- 1. users: profile verification badge (ADR-029 — spec'd, never shipped)
ALTER TABLE users ADD COLUMN IF NOT EXISTS verified boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS verification_type text;
COMMENT ON COLUMN users.verification_type IS
  'business|embassy|notable|immigration_lawyer|professional_body — set via POST /admin/users/{id}/verify';

-- 2. verification_requests
CREATE TABLE verification_requests (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requester_user_id  uuid NOT NULL REFERENCES users(id),
  entity_type        text NOT NULL CHECK (entity_type IN ('organization','user')),
  organization_id    uuid REFERENCES organizations(id),
  requested_type     text,
  form_submission_id uuid NOT NULL REFERENCES form_submissions(id),
  status             text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','approved','rejected','withdrawn')),
  reviewed_by        uuid REFERENCES users(id),
  resolution_note    text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  resolved_at        timestamptz,
  CONSTRAINT vreq_org_entity CHECK (
    (entity_type = 'organization' AND organization_id IS NOT NULL) OR
    (entity_type = 'user' AND organization_id IS NULL)
  )
);
CREATE INDEX idx_vreq_requester ON verification_requests (requester_user_id);
CREATE INDEX idx_vreq_status    ON verification_requests (status) WHERE status = 'pending';
CREATE UNIQUE INDEX idx_vreq_pending_org
  ON verification_requests (organization_id)
  WHERE status = 'pending' AND entity_type = 'organization';
CREATE UNIQUE INDEX idx_vreq_pending_user
  ON verification_requests (requester_user_id)
  WHERE status = 'pending' AND entity_type = 'user';

-- 3. RBAC node + grants (mirror 000144): member_role gets it, super_admin gets all.
INSERT INTO permissions (key, description) VALUES
  ('verification.request', 'Request a verification badge for own profile or administered organization')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'verification.request'
WHERE r.slug = 'member_role'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key = 'verification.request'
WHERE r.slug = 'super_admin_role'
ON CONFLICT DO NOTHING;

-- 4. Seed the verification-request system form (mirror claim-ownership seed).
DO $do$ DECLARE tid uuid := '00000000-0000-0000-0000-000000000001'; BEGIN

INSERT INTO custom_field_groups (id, tenant_id, name, slug, description, is_system)
VALUES (
  '00000000-0000-0000-0000-00000000d001', tid,
  'Verification Request Fields', 'verification_request_fields',
  'Fields for the verification badge request form (§07). Admins can add/remove fields via the Form Builder.',
  true
) ON CONFLICT (id) DO NOTHING;

-- Embed handler gates on status='published' (migration 000919 / CF13).
INSERT INTO forms (
  id, tenant_id, name, slug, description,
  is_multi_step, submit_label, success_message, status, is_system,
  form_type, submission_fee_credits
) VALUES (
  '00000000-0000-0000-0000-00000000d002', tid,
  'Request Verification Badge', 'verification-request',
  'Submit evidence to request a verification badge for your profile or organization.',
  false, 'Submit Request',
  'Your verification request has been submitted and will be reviewed by our team.',
  'published', true, 'standard', 0
) ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES ('00000000-0000-0000-0000-00000000d001', 'form', '00000000-0000-0000-0000-00000000d002', 0)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, placeholder, is_required, is_system, show_in_frontend, position) VALUES
  ('00000000-0000-0000-0000-00000000d011', '00000000-0000-0000-0000-00000000d001', tid,
   'Legal / Registered Name', 'legal_name', 'text', 'Name as it appears on official documents', true, true, true, 0),
  ('00000000-0000-0000-0000-00000000d012', '00000000-0000-0000-0000-00000000d001', tid,
   'Evidence URL', 'evidence_url', 'url', 'https://example.com/proof', true, true, true, 1),
  ('00000000-0000-0000-0000-00000000d013', '00000000-0000-0000-0000-00000000d001', tid,
   'Registration / ID Number', 'registration_id', 'text', 'e.g. RC123456, bar admission no., passport no.', false, true, true, 2),
  ('00000000-0000-0000-0000-00000000d014', '00000000-0000-0000-0000-00000000d001', tid,
   'Additional Notes', 'notes', 'textarea', 'Anything else reviewers should know…', false, true, true, 3)
ON CONFLICT (id) DO NOTHING;

INSERT INTO system_settings (tenant_id, key, value, is_public) VALUES
  (tid, 'verification.form_slug', '"verification-request"'::jsonb, false),
  (tid, 'verification.fee_credits', '0', false)
ON CONFLICT DO NOTHING;

END $do$;

COMMIT;
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000933_verification_requests.down.sql
BEGIN;
DELETE FROM system_settings WHERE key IN ('verification.form_slug','verification.fee_credits');
DELETE FROM custom_fields WHERE id IN
  ('00000000-0000-0000-0000-00000000d011','00000000-0000-0000-0000-00000000d012',
   '00000000-0000-0000-0000-00000000d013','00000000-0000-0000-0000-00000000d014');
DELETE FROM custom_field_group_assignments WHERE group_id = '00000000-0000-0000-0000-00000000d001';
DELETE FROM forms WHERE id = '00000000-0000-0000-0000-00000000d002';
DELETE FROM custom_field_groups WHERE id = '00000000-0000-0000-0000-00000000d001';
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'verification.request');
DELETE FROM permissions WHERE key = 'verification.request';
DROP TABLE IF EXISTS verification_requests;
ALTER TABLE users DROP COLUMN IF EXISTS verification_type;
ALTER TABLE users DROP COLUMN IF EXISTS verified;
COMMIT;
```

- [ ] **Step 3: Check exact table/column names against schema**

Run: `grep -n "CREATE TABLE public.permissions\|CREATE TABLE public.user_roles\|CREATE TABLE public.role_permissions\|CREATE TABLE public.system_settings\|CREATE TABLE public.custom_fields\b" migrations/000735_baseline_schema.up.sql`
Expected: confirm column lists (`permissions(key,description)`, `role_permissions(role_id,permission_id)`, `system_settings(tenant_id,key,value,is_public)`, `custom_fields` column names incl. `key`/`field_type`/`position`) and adjust the INSERT column lists to match — do NOT guess.

- [ ] **Step 4: Apply + verify**

Run: `make migrate-up` (or the project's migrate command — check Makefile) then `psql $JC_DATABASE_URL -c '\d verification_requests'`
Expected: table exists; `users` shows verified/verification_type; `SELECT slug,status FROM forms WHERE slug='verification-request'` → `published`.

- [ ] **Step 5: Commit**

```bash
git add migrations/000933_verification_requests.*.sql
git commit -m "feat(db): verification_requests table, user badge columns, seeded request form + permission"
```

---

### Task 2: sqlc queries

**Files:**
- Create: `db/queries/verification_requests.sql`
- Modify: `db/queries/users.sql` (add VerifyUser/UnverifyUser; add `verified, verification_type` to the SELECT lists of `GetPublicProfileByUsername` and whichever query backs `GetMyProfile` — find it via `grep -n "name:" internal/db/gen/users.sql.go | head -40` and the service call)
- Regenerate: `internal/db/gen/*` via `make sqlc-gen`

**Interfaces:**
- Produces (gen methods used by Task 3): `CreateVerificationRequest`, `GetVerificationRequestByID`, `GetPendingVerificationRequestForOrg`, `GetPendingVerificationRequestForUser`, `ListVerificationRequestsByRequester`, `ListVerificationRequests` (+ `CountVerificationRequests`), `ReviewVerificationRequest`, `WithdrawVerificationRequest`, `VerifyUser`, `UnverifyUser`, `GetUserByID` (email for notify — reuse existing), `GetMemberRole` (exists in `db/queries/organizations.sql`), `GetFormSubmissionByID` (exists in `db/queries/forms.sql`).

- [ ] **Step 1: Write `db/queries/verification_requests.sql`**

```sql
-- name: CreateVerificationRequest :one
INSERT INTO verification_requests
  (requester_user_id, entity_type, organization_id, requested_type, form_submission_id)
VALUES ($1, $2, sqlc.narg(organization_id), sqlc.narg(requested_type), $5)
RETURNING *;

-- name: GetVerificationRequestByID :one
SELECT vr.*, u.username AS requester_username, o.name AS org_name, o.slug AS org_slug
FROM verification_requests vr
JOIN users u ON u.id = vr.requester_user_id
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE vr.id = $1;

-- name: GetPendingVerificationRequestForOrg :one
SELECT id FROM verification_requests
WHERE organization_id = $1 AND status = 'pending' AND entity_type = 'organization';

-- name: GetPendingVerificationRequestForUser :one
SELECT id FROM verification_requests
WHERE requester_user_id = $1 AND status = 'pending' AND entity_type = 'user';

-- name: ListVerificationRequestsByRequester :many
SELECT vr.id, vr.entity_type, vr.organization_id, vr.requested_type, vr.status,
       vr.resolution_note, vr.created_at, vr.resolved_at,
       o.name AS org_name, o.slug AS org_slug
FROM verification_requests vr
LEFT JOIN organizations o ON o.id = vr.organization_id
WHERE vr.requester_user_id = $1
ORDER BY vr.created_at DESC;

-- name: ListVerificationRequests :many
SELECT vr.*, u.username AS requester_username, u.email AS requester_email,
       o.name AS org_name, o.slug AS org_slug, rv.username AS reviewed_by_username
FROM verification_requests vr
JOIN users u ON u.id = vr.requester_user_id
LEFT JOIN organizations o ON o.id = vr.organization_id
LEFT JOIN users rv ON rv.id = vr.reviewed_by
WHERE ($1::text = '' OR vr.status = $1)
  AND ($2::text = '' OR vr.entity_type = $2)
ORDER BY vr.created_at DESC
LIMIT $3 OFFSET $4;

-- name: CountVerificationRequests :one
SELECT count(*) FROM verification_requests vr
WHERE ($1::text = '' OR vr.status = $1)
  AND ($2::text = '' OR vr.entity_type = $2);

-- name: ReviewVerificationRequest :exec
UPDATE verification_requests
SET status = $2, reviewed_by = $3, resolution_note = sqlc.narg(resolution_note), resolved_at = now()
WHERE id = $1 AND status = 'pending';

-- name: WithdrawVerificationRequest :execrows
UPDATE verification_requests
SET status = 'withdrawn', resolved_at = now()
WHERE id = $1 AND requester_user_id = $2 AND status = 'pending';
```

In `db/queries/users.sql` append:

```sql
-- name: VerifyUser :exec
UPDATE users SET verified = true, verification_type = sqlc.narg(verification_type), updated_at = now()
WHERE id = $1;

-- name: UnverifyUser :exec
UPDATE users SET verified = false, verification_type = NULL, updated_at = now()
WHERE id = $1;
```

And add `verified, verification_type` to the SELECT column lists of `GetPublicProfileByUsername` and the `/me` profile query (find its name: `grep -rn "GetMyProfile\|my.*profile" internal/users/service.go | head`).

- [ ] **Step 2: Regenerate**

Run: `make sqlc-gen`
Expected: `internal/db/gen/verification_requests.sql.go` created; users gen updated; `go build ./...` clean.

- [ ] **Step 3: Commit**

```bash
git add db/queries/verification_requests.sql db/queries/users.sql internal/db/gen/
git commit -m "feat(db): sqlc queries for verification requests and user badge grant"
```

---

### Task 3: `internal/verification` package — store, service, handlers

**Files:**
- Create: `internal/verification/verification.go` (constants, inputs, emitter interfaces)
- Create: `internal/verification/store.go`
- Create: `internal/verification/service.go`
- Create: `internal/verification/handlers.go`
- Test: `internal/verification/handlers_test.go`

**Interfaces:**
- Consumes: gen methods from Task 2; `errpkg`; `auth.GetUserFromContext` (same helper the org handlers use — check its name in `internal/organizations/handlers.go:709`); `worker.TypeEmailSend`, `worker.QueueForTask`, `asynq.NewTask` (pattern: `internal/organizations/organizations.go:182`).
- Produces: `type Service` with `NewService(*db.DB) *Service`, `SetSettings(SettingsReader)`, `SetWebhooks(WebhookEmitter)`, `SetAsynq(*asynq.Client)`; `type Handlers` with `NewHandlers(*Service) *Handlers` exposing `SubmitRequest`, `ListMyRequests`, `GetRequest`, `WithdrawRequest`, `ListTypes`, `AdminListRequests`, `ReviewRequest`.

- [ ] **Step 1: Write the failing test**

Create `internal/verification/handlers_test.go` — mirror `internal/organizations/handlers_test.go` structure (whatever stubbing style it uses). Cover:

```go
// Cases to cover (adapt to the repo's existing test harness):
// - SubmitRequest: 401 when unauthenticated
// - SubmitRequest: 400 invalid_entity_type / 404 organization not found /
//   403 not_org_admin / 409 already_verified / 409 pending_request_exists /
//   400 form_submission_not_new / 400 invalid_requested_type
// - WithdrawRequest: 404 for another user's request; 204/200 own pending
// - ReviewRequest: 400 on status=withdrawn; 200 approve/reject
// - ListTypes: 200 returns the 5 badge types
```

Run: `go test ./internal/verification/...`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 2: Write `verification.go` + `store.go`**

`verification.go`:

```go
package verification

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	EntityTypeOrganization = "organization"
	EntityTypeUser         = "user"

	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusWithdrawn = "withdrawn"

	DefaultPageSize int32 = 20
	MaxPageSize     int32 = 100
)

// VerificationTypes is the §07 badge-type vocabulary shared by orgs and users.
var VerificationTypes = []string{"business", "embassy", "notable", "immigration_lawyer", "professional_body"}

func IsValidType(s string) bool {
	for _, t := range VerificationTypes {
		if s == t {
			return true
		}
	}
	return false
}

type SubmitInput struct {
	EntityType       string
	OrganizationID   *pgtype.UUID
	RequestedType    *string
	FormSubmissionID pgtype.UUID
}

type ReviewInput struct {
	Status         string
	ResolutionNote string
}

// WebhookEmitter fans verification_request.* / user.verified events out to
// subscribed endpoints (fire-and-forget, Section 12).
type WebhookEmitter interface {
	EmitEvent(ctx context.Context, eventType string, payload any)
}

// SettingsReader supplies verification.form_slug / verification.fee_credits.
type SettingsReader interface {
	GetSetting(ctx context.Context, key string) (string, error)
}
```

`store.go`: thin wrapper over `gen.Queries` (mirror `internal/organizations/store.go` — `write`/`read` split if that package uses it; keep simple single `*gen.Queries` pair if not). Methods: `Create`, `GetByID`, `GetPendingForOrg`, `GetPendingForUser`, `ListByRequester`, `List` (+`Count`), `Review`, `Withdraw`, plus pass-throughs to existing `GetMemberRole`, `GetOrganizationByIDOnly`/`ActiveOrgExistsByID`, `GetFormSubmissionByID`, and user fetch (`GetUserByID` or equivalent for email lookup — find in `db/queries/users.sql`).

- [ ] **Step 3: Write `service.go`**

Mirror `ClaimOwnership`/`ReviewOwnershipClaim` (`internal/organizations/service.go:798,890`):

- `SubmitRequest(ctx, requesterID pgtype.UUID, in SubmitInput) (gen.VerificationRequest, error)`:
  - validate `in.EntityType` (else `ErrInvalid: invalid_entity_type`)
  - `in.RequestedType != nil` → `IsValidType` else `invalid_requested_type`
  - org path: org exists+active (`errpkg.ErrNotFound`), not `org.Verified` (`already_verified`), requester role via `GetMemberRole` is `owner|admin` or `owner_user_id == requester` (`not_org_admin`), `GetPendingForOrg` no rows (`pending_request_exists`)
  - user path: `organization_id` must be nil; caller not already `verified` (add tiny gen query `GetUserVerified` or reuse a user fetch — choose whatever exists); `GetPendingForUser` no rows
  - form submission: fetch `GetFormSubmissionByID`; require `Status == 'new'` and submission's `user_id == requesterID` (don't link another user's submission)
  - create row; return it
- `ListMyRequests`, `GetRequest` (requester-scoped → `request_not_found`), `WithdrawRequest` (`execrows` 0 → `request_not_pending`), `AdminList` (`status`, `entityType`, page/pageSize clamps), `ReviewRequest`:
  - fetch; `status != pending` → `request_not_pending`; `input.Status` must be approved|rejected (else `invalid_status`)
  - `ReviewVerificationRequest` exec; on success emit `verification_request.approved`/`verification_request.rejected` webhook + `enqueueReviewEmail(ctx, requesterEmail, approved, resolutionNote)` — copy the `enqueueInvitationEmail` body (payload keys `to,subject,body,type`; `type: "verification_review"`; subject like `"Your verification request was approved"`; link body to `{siteURL}/verification-request`)
  - service fields: `asynq *asynq.Client`, `webhooks WebhookEmitter`, `settings SettingsReader`, `siteURL string` + setters (mirror organizations)

- [ ] **Step 4: Write `handlers.go`**

Mirror `internal/organizations/handlers.go` claim handlers (chi `URLParam`, `pgtype.UUID.Scan`, `errpkg.WriteError`, `errpkg.WriteJSON` envelope `{data: ...}`). Routes to serve:

```
SubmitRequest     POST   /verification-requests          body {entity_type, organization_id?, requested_type?, form_submission_id}
ListMyRequests    GET    /me/verification-requests
GetRequest        GET    /verification-requests/{id}
WithdrawRequest   DELETE /verification-requests/{id}
ListTypes         GET    /verification-requests/types     → {data: {types: [...]}}
AdminListRequests GET    /admin/verification-requests     ?status=&entity_type=&page=&page_size=
ReviewRequest     PATCH  /admin/verification-requests/{id} body {status, resolution_note}
```

- [ ] **Step 5: Run tests green**

Run: `go test ./internal/verification/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/verification/
git commit -m "feat(verification): request submission, tracking, withdraw, admin review service+handlers"
```

---

### Task 4: Wire routes + user badge grant

**Files:**
- Modify: `internal/server/server.go` (service construction + route mounts)
- Modify: `internal/users/service.go` (add `VerifyUser`/`UnverifyUser` + optional webhook emitter)
- Modify: `internal/admin/moderation.go` (add `VerifyUser`/`UnverifyUser` handlers next to `VerifyOrganization` ~line 1326)
- Modify: `internal/users/store.go` if the service goes through it (check that package's store pattern)

**Interfaces:**
- Consumes: `internal/verification` from Task 3; `gen.VerifyUser`/`UnverifyUser` from Task 2.
- Produces: live routes `POST/DELETE /admin/users/{id}/verify`; `users.Service.VerifyUser(ctx, id, adminID pgtype.UUID, vtype *string) error` emitting `user.verified` + `user.unverified` webhook events (payload mirrors org: `{user_id, verified, verification_type, verified_by}` — check `organizations/service.go:1003`).

- [ ] **Step 1: users.Service badge grant**

In `internal/users/service.go`, mirror `VerifyOrganization` (`internal/organizations/service.go:990`): add a `webhooks` field + `SetWebhooks` setter (same `EmitEvent` interface shape) and:

```go
func (s *Service) VerifyUser(ctx context.Context, id, adminID pgtype.UUID, vtype *string) error {
    if vtype != nil && !verification.IsValidType(*vtype) {
        return fmt.Errorf("%w: invalid verification_type", errpkg.ErrInvalid)
    }
    if err := s.store.VerifyUser(ctx, gen.VerifyUserParams{ID: id, VerificationType: ...}); err != nil {
        return err
    }
    s.emitWebhook(ctx, "user.verified", map[string]any{...})
    s.enqueueGrantEmail(ctx, id) // spec: requester is emailed on grant too
    return nil
}
// UnverifyUser mirrors with "user.unverified" (+ revoke email, type "verification_revoked").
```

If `users.Service` has no store-level `VerifyUser`, call the gen queries via its existing write handle (check how other user updates run). For `enqueueGrantEmail`, copy the `email:send` asynq pattern from `enqueueInvitationEmail` (`internal/organizations/organizations.go:182`) — payload `{to, subject, body, type: "verification_granted"}`; fetch the recipient email via the existing user-fetch gen query; best-effort, never fail the grant.

Apply the same grant email inside `organizations.Service.VerifyOrganization` (~line 990): after the `organization.verified` emit, enqueue `email:send` to the org owner's address (type `verification_granted`), using the already-wired `s.asynq` — the org's claimant/requester may differ from its owner but the owner is who the spec's "requester is notified on grant" resolves to at grant time.

- [ ] **Step 2: admin handlers**

In `internal/admin/moderation.go`, clone `VerifyOrganization`/`UnverifyOrganization` into `VerifyUser`/`UnverifyUser` using `h.Users` (check the field name — `h.Orgs` precedent; find the users service field on admin `Handlers`), ID param `id`, body `{verification_type}`. Reuse `isValidOrgVerificationType` (rename-free — same vocabulary) or swap to `verification.IsValidType` and update the org call sites too.

- [ ] **Step 3: server wiring**

In `internal/server/server.go`:

```go
// near orgService construction (~line 828)
verificationService := verification.NewService(database)
verificationService.SetSettings(settingsService /* adapter matching SettingsReader */)
verificationService.SetWebhooks(<same emitter wired into orgService>)
verificationService.SetAsynq(asynqClient /* the *asynq.Client used elsewhere */)
verificationService.SetSiteURL(cfg.SiteURL /* check the config field name */)
verificationHandlers := verification.NewHandlers(verificationService)
```

Then mount (next to the claim-ownership block ~line 4737 and admin blocks):

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "verification.request")).Post("/verification-requests", verificationHandlers.SubmitRequest)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "verification.request")).Get("/me/verification-requests", verificationHandlers.ListMyRequests)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "verification.request")).Get("/verification-requests/{id}", verificationHandlers.GetRequest)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "verification.request")).Delete("/verification-requests/{id}", verificationHandlers.WithdrawRequest)
r.Get("/verification-requests/types", verificationHandlers.ListTypes) // public

r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).Get("/admin/verification-requests", verificationHandlers.AdminListRequests)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).Patch("/admin/verification-requests/{id}", verificationHandlers.ReviewRequest)

r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).Post("/admin/users/{id}/verify", adminHandlers.VerifyUser)
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "admin.moderation")).Delete("/admin/users/{id}/verify", adminHandlers.UnverifyUser)
```

`GET /verification-requests/types` ordering: mount it before `{id}` so `/types` isn't captured as an id (chi handles static-over-param anyway, but keep it adjacent for clarity).

- [ ] **Step 4: Build + smoke**

Run: `go build ./... && go test ./internal/users/... ./internal/admin/...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/users/ internal/admin/moderation.go
git commit -m "feat(verification): wire request routes + admin user badge grant"
```

---

### Task 5: Appeals subject + DTOs

**Files:**
- Modify: `internal/appeals/service.go` (subject const + registration + resolver, ~lines 63, 646)
- Modify: `internal/users/types.go` (`PublicProfileResponse` ~line 201, `UserProfileResponse` ~line 86)
- Modify: `internal/users/service.go` (populate the new fields from the updated gen row structs)

**Interfaces:**
- Consumes: `users.verified`/`verification_type` selected by the gen queries updated in Task 2.
- Produces: `verified`, `verification_type` on `GET /users/{username}` (public) and `GET /me`; appealable subject `verification_request`.

- [ ] **Step 1: Appeals subject**

In `internal/appeals/service.go`: add `SubjectVerificationRequest = "verification_request"` to the subject const block, then mirror `s.orgClaimSubject()` (find it — it's a method returning hooks with `ListEligible`/`OnOverturn` etc.) into `s.verificationRequestSubject()` whose eligible-subject fetch reads a `verification_requests` row by id via the service's store/gen access (check how `orgClaimSubject` fetches claims — same pattern), and register `s.subjects[SubjectVerificationRequest] = s.verificationRequestSubject()` in the registry block (~line 646). Eligibility rule: only `rejected` requests are appealable (mirror org_claim's rule). OnOverturn: set the request back to `pending` (new gen query `-- name: ReopenVerificationRequest :exec UPDATE verification_requests SET status='pending', reviewed_by=NULL, resolved_at=NULL WHERE id=$1 AND status='rejected'`) — add it to `db/queries/verification_requests.sql` and re-run `make sqlc-gen`.

- [ ] **Step 2: DTO fields**

In `internal/users/types.go` add to both structs:

```go
Verified         bool    `json:"verified"`
VerificationType *string `json:"verification_type"`
```

and populate them in the service mapping where `PublicProfileResponse`/`UserProfileResponse` are built from the gen row (fields now returned by the updated queries). If `/me` is assembled elsewhere (e.g. `me_handlers.go`), populate there too.

- [ ] **Step 3: Test**

Run: `go build ./... && go test ./internal/users/... ./internal/appeals/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/appeals/ internal/users/ db/queries/verification_requests.sql internal/db/gen/
git commit -m "feat(verification): verification_request appeal subject + verified fields on user DTOs"
```

---

### Task 6: Main app — `/verification-request` page

**Files:**
- Create: `apps/main/src/routes/(app)/verification-request/+page.server.ts`
- Create: `apps/main/src/routes/(app)/verification-request/+page.svelte`
- Modify: `apps/main/src/lib/types/index.ts` (`MyVerificationRequest`, `VerificationRequest` types)
- Modify: `apps/main/src/lib/i18n/en.json` (+ `fr.json` — add the `verification_request.*` key namespace; copy the `claim_ownership.*` block shape)

**Interfaces:**
- Consumes: `GET /forms/{slug}/embed` (FormEmbed type — already used by claim-ownership), `POST /forms/{id}/submissions`, `GET /verification-requests/types`, `POST /verification-requests`, `GET /me/verification-requests`, `DELETE /verification-requests/{id}`, `GET /organizations/{slug}`.
- Produces: page at `/verification-request?org={slug}` | `?self=1` | (none → My requests).

- [ ] **Step 1: `+page.server.ts`**

Clone `(app)/claim-ownership/+page.server.ts` and adapt:

```ts
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests)
// Modes: ?org={slug} → org request form; ?self=1 → profile request form;
// none → "My verification requests" (GET /me/verification-requests).
// Dynamic form via GET /forms/{slug}/embed where slug comes from the backend
// config endpoint — the form slug is read server-side from the backend's
// public settings or falls back to 'verification-request' (system_settings
// 'verification.form_slug').
```

Load `form` from `/forms/verification-request/embed` (catch → null), `org` from `/organizations/{slug}` when `?org=`, `types` from `/verification-requests/types`, `requests` from `/me/verification-requests` when no params. Auth redirect same as claim page.

- [ ] **Step 2: `+page.svelte`**

Clone `(app)/claim-ownership/+page.svelte`, rename i18n namespace to `verification_request.*`, and change:

- Header copy: "Request verification" / "My verification requests".
- Above the dynamic fields, render a `requested_type` select fed by `data.types` (the `/types` response), optional.
- Submit: same two-call flow — `POST /forms/{id}/submissions` `{field_values, payment_method?}` then `POST /verification-requests` `{entity_type: org ? 'organization' : 'user', organization_id: org?.id, requested_type, form_submission_id}`.
- My-requests list: status chips + withdraw (`DELETE /verification-requests/{id}`) + `AppealButton` `entityType="verification_request"` on rejected — copy the claims list block verbatim, fields `entity_type/org_name/resolution_note/created_at/reviewed_at` → use `resolved_at` per our schema (check the ListVerificationRequestsByRequester columns — resolved_at, not reviewed_at; align the template).

- [ ] **Step 3: Typecheck**

Run: `cd apps/main && bun run check` (or `bunx svelte-check --tsconfig ./tsconfig.json`)
Expected: 0 new errors in changed files.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/verification-request" apps/main/src/lib/types/index.ts apps/main/src/lib/i18n/
git commit -m "feat(main): verification-request page — org + profile modes, My requests, appeals"
```

---

### Task 7: Main app — entry points + badge

**Files:**
- Modify: `apps/main/src/routes/(app)/dashboard/organizations/[id]/+page.svelte` — "Request verification" action when `!org.verified` and caller role is owner|admin
- Modify: user settings section (find the account/profile settings file — e.g. `apps/main/src/lib/components/hub/settings/sections/AccountSection.svelte`) — "Request verification" link gated on `!me.verified`
- Modify: `apps/main/src/routes/(app)/u/[username]/+page.svelte` — render verified badge next to display name when `user.verified` (mirror how org pages render theirs — check `ProviderCard.svelte` / org page badge markup for the shared badge component or icon; if a shared `VerifiedBadge` component exists use it)

**Interfaces:**
- Consumes: `org.verified`, caller role (already loaded in the dashboard page — `myOrgs` role resolution exists in `+page.server.ts`), `me.verified` (from layout user payload — confirm the `/me` fields land on `event.locals.user`/`data.user`), `user.verified`/`verification_type` on the public profile load.

- [ ] **Step 1: Org dashboard button** — find the actions/header area of `dashboard/organizations/[id]/+page.svelte`; add a "Request verification" link → `/verification-request?org={org.slug}` shown only when `!org.verified` and the caller's role is `owner`/`admin` (the page already computes role gating for edit controls — reuse that predicate).

- [ ] **Step 2: Settings action** — in the account settings section add a "Request verification" row/link → `/verification-request?self=1` shown when the signed-in user isn't verified; show the badge state when they are.

- [ ] **Step 3: Public profile badge** — on `u/[username]` add the verified badge adjacent to the display name (reuse the badge markup used on org profiles — locate it via `grep -rn "BadgeCheck\|verified" apps/main/src/lib/components | head`).

- [ ] **Step 4: Check + commit**

```bash
cd apps/main && bun run check
git add apps/main/
git commit -m "feat(main): verification entry points — org dashboard, settings, profile badge"
```

---

### Task 8: Admin panel — review queue

**Files:**
- Create: `apps/admin/src/routes/verification-requests/+page.svelte`
- Modify: `apps/admin/src/lib/api.ts` (`verificationRequestsAdminApi`)
- Modify: `apps/admin/src/lib/components/layout/sidebar.svelte` (nav entry under Organisations group, `admin.moderation`)
- Modify: admin i18n keys (`nav.verification_requests`, page strings — find the admin i18n file, e.g. `src/lib/stores/i18n.svelte` + locale JSONs)

**Interfaces:**
- Consumes: `GET /admin/verification-requests`, `PATCH /admin/verification-requests/{id}`, `GET /forms/submissions/{id}` or equivalent to view `field_values` (check what the claims page calls — it fetches `ClaimSubmission` — reuse that endpoint).
- Produces: queue page + "Grant badge" shortcut.

- [ ] **Step 1: api.ts**

```ts
export const verificationRequestsAdminApi = {
  list: (params: { status?: string; entity_type?: string; page?: number; page_size?: number }) =>
    api<{ items: any[]; total: number }>(`/admin/verification-requests?${new URLSearchParams(...)}`),
  review: (id: string, body: { status: 'approved' | 'rejected'; resolution_note?: string }) =>
    api<unknown>(`/admin/verification-requests/${id}`, { method: 'PATCH', body }),
};
```

(Match the file's actual response envelope — claims uses `{ items, meta }`; copy its exact shape.)

- [ ] **Step 2: Queue page** — clone `apps/admin/src/routes/organisations/claims/+page.svelte`: DataTable + status/entity_type FilterPanel, View modal rendering the linked submission's `field_values`, Approve/Reject modals w/ `resolution_note` textarea. Add a **"Grant badge"** action on `status === 'approved'` rows: opens a small modal with a type select (options fetched from `GET /verification-requests/types`) prefilled with `requested_type`, then calls `organisationsAdminApi.verify(orgId, …)` for orgs or `usersAdminApi`-style `POST /admin/users/{id}/verify` for profiles (added in Task 9 — if tasks are reordered, add both calls here).

- [ ] **Step 3: Sidebar** — add `{ path: "/verification-requests", label: t("nav.verification_requests", "Verification Requests"), icon: BadgeCheck, permission: "admin.moderation" }` next to the claims entry (~line 86), import `BadgeCheck` from lucide.

- [ ] **Step 4: Check + commit**

```bash
cd apps/admin && bun run check
git add apps/admin/
git commit -m "feat(admin): verification-requests review queue + grant-badge shortcut"
```

---

### Task 9: Admin panel — `/users` verify actions

**Files:**
- Modify: `apps/admin/src/routes/users/+page.svelte` (verify/unverify row actions + type picker modal)
- Modify: `apps/admin/src/lib/api.ts` (extend the users admin API object — find it e.g. `usersAdminApi`)

**Interfaces:**
- Consumes: `POST/DELETE /admin/users/{id}/verify` (Task 4); `GET /verification-requests/types`.

- [ ] **Step 1: api.ts** — add to the users admin api object:

```ts
verify: (id: string, body: { verification_type?: string }) =>
  api<unknown>(`/admin/users/${id}/verify`, { method: 'POST', body }),
unverify: (id: string) =>
  api<unknown>(`/admin/users/${id}/verify`, { method: 'DELETE' }),
```

- [ ] **Step 2: Row actions** — on `/users` add `Verify` (`show: (row) => !row.verified`) opening a type-select modal → `verify(id, {verification_type})`; `Unverify` (`show: (row) => row.verified`, `variant: 'destructive'`) → `unverify(id)`; reload via `entityController?.reload()` per `apps/admin/AGENTS.md` conventions.

- [ ] **Step 3: Check + commit**

```bash
cd apps/admin && bun run check
git add apps/admin/
git commit -m "feat(admin): verify/unverify user profile actions"
```

---

### Task 10: JEOS + catalogs + i18n sweep

**Files:**
- Modify: `.jeos/sections/07-organization-profiles-claim-ownership.md` — add the "Verification Requests" subsection from the spec (schema, routes, eligibility, two-step grant, seeded form)
- Modify: `.jeos/conventions/api-routes-catalog.md` — add the 9 routes near the `/claim-ownership` block
- Modify: `.jeos/conventions/system-settings-catalog.md` — `verification.form_slug`, `verification.fee_credits`
- Modify: `.jeos/sections/03-rbac.md` — `verification.request` node entry
- Modify: `.jeos/sections/12-*.md` (webhooks catalog file — find exact name) — `user.verified`, `user.unverified`, `verification_request.approved`, `verification_request.rejected`
- Modify: `.jeos/sections/22-*.md` (appeals) — `verification_request` appealable subject
- Modify: `internal/control/settings_catalog.go` — register `verification.form_slug` + `verification.fee_credits` next to `claim_ownership.form_slug` (line ~653): `{Type: "string", DefaultValue: "verification-request", Description: "Slug of the Form Builder form used for verification badge requests (§07).", Panel: "control"}` and `{Type: "int", DefaultValue: "0", ...}`

- [ ] **Step 1: Apply all doc/catalog edits** matching each file's existing format.

- [ ] **Step 2: i18n sweep** — `grep -rn "claim_ownership\|verification_request" apps/main/src/lib/i18n/en.json apps/admin/src` — ensure every `t('verification_request.*')` key used in Tasks 6–9 exists in `en.json` (and `fr.json` where the app requires both — check whether fr.json must have every key or falls back; mirror how `claim_ownership.*` is handled).

- [ ] **Step 3: Final verification**

```bash
go build ./... && go test ./internal/verification/... ./internal/users/... ./internal/appeals/...
cd apps/main && bun run check
cd ../admin && bun run check
```

- [ ] **Step 4: Commit**

```bash
git add .jeos/ internal/control/settings_catalog.go apps/main/src/lib/i18n apps/admin/src
git commit -m "docs+config(verification): JEOS section/catalogs, settings registry, i18n keys"
```

---

## Self-review notes

- Spec coverage: schema (T1), routes (T3/T4), user badge columns+grant (T1/T4/T9), request+review flow (T3/T6/T8), seeded form (T1), appeals (T5), DTOs (T5), entry points+badge (T7), admin queue (T8), JEOS/catalogs/i18n (T10). ✅
- Type consistency: `entity_type` values `organization|user`; request statuses mirror claim statuses; `requested_type` column name is used consistently (spec's field — keep `requested_type` everywhere, not `requested_verification_type`).
- The down-migration drops `users.verified` — fine pre-release.
