# Design: User-Initiated Verification Requests

**Date:** 2026-10-25
**Status:** Approved design
**Scope:** Let frontend users request a verification badge for (a) an organization
they own/administer and (b) their own profile. Requests carry evidence via the
Form Builder, queue for admin review, and require a separate admin grant step.

## Background

- JEOS §7 (ADR-029) spec'd `verified` + `verification_type` badges on both
  organizations and individual profiles, granted admin-only. Org badges are
  implemented (`POST/DELETE /admin/organizations/{id}/verify`, gated by
  `admin.moderation`). User-profile badges were spec'd but never shipped:
  `users` has no `verified`/`verification_type` columns and no grant route.
- The claim-ownership flow (`/claim-ownership`) is the proven precedent:
  Form Builder evidence form → `POST /forms/{id}/submissions` → domain request
  row → admin review queue → outcome notification. Verification requests mirror
  it. Per §7, claiming and verification remain separate trust signals; this
  feature does not couple them.
- Decisions captured with the requester: Form Builder evidence intake;
  two-step grant (approving a request does not grant the badge — admin runs the
  verify endpoint separately); requester suggests a badge type that admin may
  override; org requests accepted from owner AND `admin`-role members.

## Data model

New table `verification_requests` (mirrors `ownership_claims`):

```sql
CREATE TABLE verification_requests (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requester_user_id  uuid NOT NULL REFERENCES users(id),
  entity_type        text NOT NULL CHECK (entity_type IN ('organization','user')),
  organization_id    uuid REFERENCES organizations(id),   -- NULL for entity_type='user'
  requested_type     text,                                 -- requester-suggested badge type
  form_submission_id uuid NOT NULL REFERENCES form_submissions(id),
  status             text NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','approved','rejected','withdrawn')),
  reviewed_by        uuid REFERENCES users(id),
  resolution_note    text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  resolved_at        timestamptz
);
-- One pending request per org; one pending request per user profile:
CREATE UNIQUE INDEX idx_vreq_pending_org
  ON verification_requests (organization_id)
  WHERE status = 'pending' AND entity_type = 'organization';
CREATE UNIQUE INDEX idx_vreq_pending_user
  ON verification_requests (requester_user_id)
  WHERE status = 'pending' AND entity_type = 'user';
```

`users` table gains `verified boolean NOT NULL DEFAULT false` and
`verification_type text` (same value set as orgs:
`business|embassy|notable|immigration_lawyer|professional_body`).

## API routes

Mirroring `/claim-ownership` shapes:

```
POST   /api/v1/verification-requests          {entity_type, organization_id?, requested_type?, form_submission_id}   [verification.request]
GET    /api/v1/me/verification-requests       requester's own requests, newest first                                 [verification.request]
GET    /api/v1/verification-requests/{id}     own request detail                                                     [verification.request]
DELETE /api/v1/verification-requests/{id}     withdraw a pending request (requester only)                            [verification.request]
GET    /api/v1/verification-requests/types    allowed badge-type values (Rule 16 — no hardcoded enum on frontend)    [public]
GET    /api/v1/admin/verification-requests    review queue ?status=&entity_type=                                     [admin.moderation]
PATCH  /api/v1/admin/verification-requests/{id}  {status: approved|rejected, resolution_note}                        [admin.moderation]
POST   /api/v1/admin/users/{id}/verify        {verification_type} — grant profile badge                              [admin.moderation]
DELETE /api/v1/admin/users/{id}/verify        revoke profile badge                                                   [admin.moderation]
```

New RBAC node `verification.request` seeded default-on for member groups (same
grant set as `organizations.claim`). Review and both grant routes reuse
`admin.moderation`, consistent with org verify.

## Request flow

1. **Entry points (main app):**
   - Org dashboard `/dashboard/organizations/[id]` — "Request verification"
     action visible to owner + `admin`-role members while `org.verified` is false.
   - User settings — "Request verification" action while `me.verified` is false.
   - Both land on `/verification-request?org={slug}` or `?self=1`.
2. **Request page** (`/verification-request`, auth-gated): loads
   `GET /forms/{slug}/embed` — slug from `system_settings`
   `verification.form_slug` (default `verification-request`) — renders the
   dynamic fields plus a suggested-type select fed by
   `GET /verification-requests/types`. Submits
   `POST /forms/{id}/submissions` (paid `submission_fee_credits` supported —
   credits or Paystack/Stripe gateway redirect, same as claim-ownership), then
   `POST /verification-requests` linking the `form_submission_id`.
3. **Eligibility (server-enforced on POST):**
   - `entity_type=organization`: `organization_id` required; requester is
     `owner_user_id` or an `organization_members` row with role `owner|admin`;
     org not already `verified`; no pending request exists for that org.
   - `entity_type=user`: `organization_id` must be null; request is always for
     the caller's own profile; caller not already `verified`; no pending
     request exists for the caller.
   - `requested_type` (optional) validated against the allowed set.
4. **Tracking:** `/verification-request` with no query params lists "My
   verification requests" from `GET /me/verification-requests` — status chips,
   resolution note, withdraw button on `pending`, `AppealButton`
   (entity type `verification_request`) on `rejected` (§22 precedent).
   Appealable subject types are a registered enum in
   `internal/appeals/service.go` (e.g. `org_claim`), so `verification_request`
   must be registered there with a subject resolver that exposes the request
   (and linked submission) to staff tooling — same pattern as
   `SubjectOrgClaim`.

## Review flow (two-step)

1. Admin queue at `GET /admin/verification-requests` with `?status=` +
   `?entity_type=` filters. Admin panel page `/verification-requests` follows
   the `/organisations/claims` precedent: DataTable + view modal rendering the
   linked `form_submissions.field_values` + approve/reject modals with
   `resolution_note`.
2. `PATCH /admin/verification-requests/{id}` records the decision only
   (approved ≠ verified). Reject sets `resolution_note`; approve may leave it
   empty. Withdrawn requests are terminal and unreviewable (same guard as
   claims).
3. **Grant** is a separate admin action: on approved rows the queue offers
   "Grant badge" → type picker prefilled with `requested_type`, overridable →
   calls `POST /admin/organizations/{id}/verify` (existing) or
   `POST /admin/users/{id}/verify` (new). Revoke via the corresponding DELETE.
4. Notifications: review outcome enqueues `email:send` (BUG-018 pattern) and
   emits `verification_request.approved` / `verification_request.rejected`
   webhook events. Badge grants emit `organization.verified` (existing) and new
   `user.verified` (§12 catalog addition).

## Badge display

`verified` + `verification_type` added to the public user profile DTO so
`/u/[username]` can render the badge (orgs already render theirs). Admin
`/users` page gains verify/unverify row actions matching the orgs page.

## JEOS updates (spec before code — repo rule)

- `sections/07-organization-profiles-claim-ownership.md`: new "Verification
  Requests" subsection — schema, routes, eligibility, two-step grant.
- `conventions/api-routes-catalog.md`: the 9 routes above.
- `conventions/system-settings-catalog.md`: `verification.form_slug`.
- `sections/03-rbac.md`: `verification.request` node.
- §22 moderation-appeals catalog: `verification_request` appealable subject.
- §12 webhook catalog: `user.verified`, `verification_request.approved`,
  `verification_request.rejected`.

## Error handling

Standard Rule 5 error envelope. Expected codes: `already_verified`,
`pending_request_exists`, `not_org_admin`, `request_not_found`,
`request_not_pending`, `invalid_requested_type`, `invalid_status`,
`insufficient_credits` / `payment_intent_failed` (paid forms).

## Testing

Handler tests mirroring `internal/organizations/handlers_test.go`: submit
(org + user), eligibility rejections (non-admin member, already verified,
duplicate pending), withdraw, review transitions (approve/reject, withdrawn
unreviewable), admin user verify/unverify grant + webhook emission.
