# Services + Mentorship Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) or inline execution to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify the marketplace surface — strip the custom chrome off `/services`, rebuild `/services/[slug]` inside the unified app shell, merge mentorship into the service layer (mentorship = `listing_kind='mentorship'`, mentorship requests = `service_bookings`), and integrate Service Bookings + Mentorship requests as tabs inside `/messages`.

**Architecture:** `service_listings` (contents + `service_listing_meta`) is the single source of truth for everything an Agent/Agency/mentor lists — including mentorship. `service_bookings` is the single source of truth for every consumer→provider intent — inquiries, paid bookings, and mentorship requests. Mentors keep their `user_profiles` capability columns (capacity, areas, bio, embedding); flipping `is_mentor` upserts a shadow `kind='mentorship'` listing so discovery, detail pages, booking, admin review, GDPR and SEO ride the existing service pipeline. The `/mentorship/*` and `/mentors` API endpoints become thin adapters over servicedir (same response shapes — external consumers keep working). Frontend: `/messages` becomes the unified inbox — tabs `Messages | Requests | Bookings | Mentorship` implemented as routes (`/messages`, `/messages/requests`, `/messages/bookings`, `/messages/mentorship`) matching the existing route-per-tab pattern.

**Decisions locked with the user:**
1. `/services` list page keeps ONLY the `LeftNav` sidebar — `MarketplaceHeader`/`MarketplaceFooter` removed; content gets left/right margins. Accepted tradeoff: mobile loses nav chrome except the in-page breadcrumb.
2. Mentorship merge is FULL backend — shadow listings + requests become `service_bookings`; `mentorship_connections` is migrated then dropped.
3. `/messages` gets new tabs — `Messages | Requests | Bookings | Mentorship`.
4. Old surfaces redirect: `/me/service-bookings` → `/messages/bookings`, `/mentors` → `/services?kind=mentorship`, etc.

**Tech Stack:** Go (chi, sqlc/pgx, golang-migrate, Asynq) · SvelteKit 2 + Svelte 5 Runes + Bits UI + Tailwind v4 · bun only (Rule 18)

## Global Constraints

- JEOS updated BEFORE code (AGENTS.md). Phase A edits `.jeos/` first.
- Never hand-edit `internal/db/gen/*` — edit `db/queries/*.sql`, run `make sqlc-gen`.
- Commits go to `main`. No AI attribution anywhere (Rule 17).
- Error envelope `{ "error": { "code": "snake_case" } }` (Rule 5).
- No new Asynq task names needed — reuse `service:booking_*`, `mentor:generate_embedding`, `mentor:update_meilisearch` (Rule 7 satisfied; if a new name is ever introduced, add it to `sections/11-job-queue-asynq.md` first).
- `content_hash` = `hex(sha256(normalized_body))` per Rule 12 — see migration note.
- Svelte 5 strict Runes; all data from API (Rule 16); `$t()` for user-facing strings (en.json + fr.json).
- Next free migration number: **000824** (up/down pair).

## Verified schema facts (checked against migrations — do not guess)

- `contents`: `id uuid PK gen_random_uuid()`, `author_id uuid NOT NULL`, `content_type text NOT NULL DEFAULT 'discussion'`, `title text NOT NULL`, `slug text NOT NULL` (global UNIQUE `idx_contents_slug`), `body text NOT NULL DEFAULT ''`, `body_html`, `status` CHECK in (`draft`,`in_review`,`scheduled`,`published`,`archived`), `content_hash text` (UNIQUE `idx_contents_content_hash WHERE content_hash IS NOT NULL`), `organization_id uuid NULL`, `featured_image_id`, `geo_space_id`, `deleted_at`, timestamps. Listing content_type = `'service_listing'` (`internal/content/slug.go` `TypeServiceListing`).
- `service_listing_meta`: `id, content_id NOT NULL, org_id NULL, service_category NOT NULL, destination_space_id, city_space_id, is_remote, website_url, booking_url, price_range, is_verified, verification_type, claim_status, claimed_by_org_id, origin_communities, listing_kind CHECK('service','package'), purchase_mode CHECK('inquiry','external','paid') DEFAULT 'inquiry', price_amount/currency/per, available_from/until, capacity`.
- `service_categories` (migration 000803): `(id, slug UNIQUE, label, description, icon, accent_color, applies_to CHECK('service','package','both') DEFAULT 'both', position, is_active)`. Seed rows use `applies_to`, not `sort_order`.
- `service_bookings` (000767): `org_id uuid NOT NULL REFERENCES organizations(id)` today; statuses `pending|confirmed|fulfilled|cancelled|declined|disputed|refunded`; `intent IN ('inquiry','booking')`; already carries `note`, `preferred_dates`, `org_response_note`, `expires_at`, `confirmed_at`, `fulfilled_at`, `cancelled_at`, `cancel_reason`, `anonymized_at`.
- `mentorship_connections` (000349): `id, tenant_id, mentor_id, mentee_id, status(pending|active|declined|completed|cancelled), goals, destination_space_id, responded_at, completed_at, cancelled_at, cancel_reason`, UNIQUE(mentor_id, mentee_id, status); `trg_mentee_count` maintains `user_profiles.active_mentee_count`.
- `user_profiles` mentor columns: `is_mentor, mentor_capacity, active_mentee_count, mentor_areas, mentor_bio, mentor_since, mentor_embedding vector(1536)`; `display_name` is on `user_profiles` (not `users`); `users.username` exists.
- Existing servicedir seams: `s.checker.IsEnabled(ctx,"<flag>")` for feature flags (`PackagesEnabled` is the model); `enqueueStatusChangedAudience` (audience `""`=booker, `"org"`=org members); `ListServiceBookingsByUser`/`ByOrg` sqlc queries already join `sm.listing_kind` and booker user/profile — mirror them for provider side.

## Status mapping (mentorship_connections → service_bookings)

| connection status | booking status |
|---|---|
| pending | pending |
| active | confirmed |
| declined | declined |
| completed | fulfilled |
| cancelled | cancelled |

Field map: `mentee_id`→`booker_user_id` · `mentor_id`→`provider_user_id` · `goals`→`note` · `destination_space_id`→`destination_space_id` · `responded_at`→`confirmed_at` (active/completed) AND `meta->>'responded_at'` (declined — bookings have no declined_at) · `completed_at`→`fulfilled_at` · `cancelled_at`/`cancel_reason`→same · `meta = {"source":"mentorship_connection","connection_id":<id>,"tenant_id":<id>}` · `created_at`/`updated_at` preserved.

| Booking status | Service meaning | Mentorship meaning |
|---|---|---|
| pending | awaiting provider | request awaiting mentor |
| confirmed | accepted | connection active (DM auto-opens) |
| fulfilled | delivered | mentorship completed |
| declined / cancelled | same | same |
| disputed / refunded | fee disputes | n/a (mentorship is inquiry-mode) |

---

## Phase A — JEOS spec updates (before any code)

### Task 1: Update section files + route catalog

**Files:**
- `.jeos/sections/111-service-packages-bookings.md` — document `provider_user_id`, `destination_space_id`, `meta` on `service_bookings`; `?role=provider` + `?kind=` on `GET /me/service-bookings`; `POST /me/service-bookings/{id}/respond`; mentorship-kind semantics (no expiry, no fee, inquiry-only).
- `.jeos/sections/102-mentorship-matching.md` — `mentorship_connections` is migrated to `service_bookings`; mentor profile → shadow `kind='mentorship'` listing model; keep the match-scoring section (still served via compat `/mentors`).
- `.jeos/sections/114-services-marketplace.md` — `listing_kind` gains `mentorship`; `/services` chrome decision (LeftNav only); detail page inside AppShell; `/messages` tab model.
- `.jeos/conventions/api-routes-catalog.md` — add/amend:
  - `GET /api/v1/me/service-bookings?role=booker|provider&kind=&status=&intent=` (amended)
  - `POST /api/v1/me/service-bookings/{id}/respond` — provider transition `{action: accept|decline|complete, note?}` → confirmed|declined|fulfilled; route perm `mentorship.respond`; user-provider bookings only (org bookings keep the org route).
  - `GET /api/v1/services?kind=mentorship` (amended kind filter)
  - Mark `/api/v1/mentors*`, `/api/v1/mentorship/*`, `/api/v1/me/mentorship/*`, `/api/v1/admin/mentorship/connections` as **compat adapters** over servicedir.
  - Frontend route table: `/messages/bookings`, `/messages/mentorship`, redirects.

- [ ] **Step 1:** Make the edits — concise amendment blocks dated `2026-10-09`; don't rewrite whole sections.
- [ ] **Step 2:** Commit: `docs: amend JEOS for mentorship→service unification`.

---

## Phase B — Backend: schema + servicedir

### Task 2: Migration `000824_mentorship_service_unification`

**Files:**
- Create: `migrations/000824_mentorship_service_unification.up.sql`
- Create: `migrations/000824_mentorship_service_unification.down.sql`

**Interfaces produced:**
- `service_listing_meta.listing_kind` allows `'mentorship'`; `service_categories.applies_to` allows `'mentorship'`
- `service_bookings`: `org_id` NULLable, `+ provider_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL`, `+ destination_space_id uuid NULL REFERENCES spaces(id) ON DELETE SET NULL`, `+ meta jsonb NOT NULL DEFAULT '{}'`; CHECK exactly one of org_id/provider_user_id — OR `anonymized_at IS NOT NULL`, so GDPR anonymize/hard-delete of a provider user cannot trip the CHECK on a scrubbed row
- `user_profiles.mentor_listing_content_id uuid NULL REFERENCES contents(id) ON DELETE SET NULL`
- Shadow listing per `is_mentor=true` user (contents + meta)
- `mentorship_connections` → bookings; table dropped; mentee-count trigger re-pointed
- `menu_items` 'Mentors' row repointed to `/services?kind=mentorship`

- [ ] **Step 1:** Write the up migration:

```sql
-- 000824_mentorship_service_unification.up.sql
-- Mentorship merges into the service layer. Mentorship offerings are
-- service_listing_meta rows with listing_kind='mentorship'; mentorship
-- requests are service_bookings (intent='inquiry').
-- Ref: docs/superpowers/plans/2026-10-09-services-mentorship-unification.md

BEGIN;

-- 1. New listing kind + category applies_to value.
ALTER TABLE service_listing_meta
    DROP CONSTRAINT service_listing_meta_listing_kind_check,
    ADD CONSTRAINT service_listing_meta_listing_kind_check
        CHECK (listing_kind IN ('service','package','mentorship'));

ALTER TABLE service_categories
    DROP CONSTRAINT service_categories_applies_to_check,
    ADD CONSTRAINT service_categories_applies_to_check
        CHECK (applies_to IN ('service','package','mentorship','both'));

-- 2. Bookings gain a user-owned provider dimension (mentorship listings
--    are authored by the mentor user, not an org) plus request context.
ALTER TABLE service_bookings
    ALTER COLUMN org_id DROP NOT NULL,
    ADD COLUMN provider_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN destination_space_id uuid REFERENCES spaces(id) ON DELETE SET NULL,
    ADD COLUMN meta jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT service_bookings_provider_check
        CHECK ((org_id IS NOT NULL) <> (provider_user_id IS NOT NULL)
               OR anonymized_at IS NOT NULL);

CREATE INDEX idx_svc_bookings_provider_user
    ON service_bookings (provider_user_id, status);

-- 3. Mentor → shadow listing link (upsert target for PATCH
--    /me/mentor-profile; NULL until the mentor opts in).
ALTER TABLE user_profiles
    ADD COLUMN mentor_listing_content_id uuid REFERENCES contents(id) ON DELETE SET NULL;

-- 4. Seed the mentorship taxonomy category. applies_to='mentorship'
--    keeps it out of the service/package pickers.
INSERT INTO service_categories (slug, label, description, icon, accent_color, applies_to, position)
VALUES ('mentorship', 'Mentorship', 'Guidance from members who already made the move', 'graduation-cap', 'violet', 'mentorship', 115)
ON CONFLICT (slug) DO NOTHING;

-- 5. Shadow listings for every existing mentor.
--    content_hash: sha256 hex of the normalized bio PLUS a per-user
--    distinguisher ('|mentor:<user_id>') — empty/duplicate bios would
--    otherwise collide on idx_contents_content_hash (Rule 12 parity:
--    the app path uses content.ComputeContentHash; the distinguisher
--    matches ComputeContentHashWithDistinguisher semantics).
INSERT INTO contents (author_id, content_type, title, slug, body, body_html, status, content_hash)
SELECT
    up.user_id,
    'service_listing',
    COALESCE(NULLIF(up.display_name, ''), u.username) || ' — Mentorship',
    'mentor-' || u.username,
    COALESCE(up.mentor_bio, ''),
    '',
    'published',
    encode(sha256(convert_to(
        lower(regexp_replace(btrim(COALESCE(up.mentor_bio,'')), '\s+', ' ', 'g'))
        || '|mentor:' || up.user_id::text,
        'UTF8')), 'hex')
FROM user_profiles up
JOIN users u ON u.id = up.user_id
WHERE up.is_mentor = true
ON CONFLICT (slug) DO NOTHING;

INSERT INTO service_listing_meta (content_id, org_id, service_category, is_remote, listing_kind, purchase_mode)
SELECT c.id, NULL, 'mentorship', true, 'mentorship', 'inquiry'
FROM contents c
WHERE c.slug LIKE 'mentor-%' AND c.content_type = 'service_listing'
  AND NOT EXISTS (SELECT 1 FROM service_listing_meta m WHERE m.content_id = c.id);

UPDATE user_profiles up
SET mentor_listing_content_id = c.id
FROM users u, contents c
WHERE u.id = up.user_id
  AND c.slug = 'mentor-' || u.username
  AND c.content_type = 'service_listing';

-- 6. Migrate connections → bookings (mentor resolved via shadow listing).
INSERT INTO service_bookings (
    listing_meta_id, booker_user_id, provider_user_id,
    intent, party_size, note, destination_space_id, status, purchase_mode,
    confirmed_at, fulfilled_at, cancelled_at, cancel_reason, meta,
    created_at, updated_at
)
SELECT
    m.id,
    mc.mentee_id,
    mc.mentor_id,
    'inquiry',
    1,
    mc.goals,
    mc.destination_space_id,
    CASE mc.status
        WHEN 'active' THEN 'confirmed'
        WHEN 'completed' THEN 'fulfilled'
        ELSE mc.status END,
    'inquiry',
    CASE WHEN mc.status IN ('active','completed') THEN mc.responded_at END,
    CASE WHEN mc.status = 'completed' THEN mc.completed_at END,
    mc.cancelled_at,
    mc.cancel_reason,
    jsonb_build_object(
        'source','mentorship_connection',
        'connection_id', mc.id,
        'tenant_id', mc.tenant_id,
        'responded_at', mc.responded_at),
    mc.created_at,
    mc.updated_at
FROM mentorship_connections mc
JOIN user_profiles up ON up.user_id = mc.mentor_id
JOIN service_listing_meta m ON m.content_id = up.mentor_listing_content_id;

-- 7. Mentee-count trigger moves to service_bookings (confirmed
--    mentorship-kind bookings only). Fires for both NEW and OLD provider
--    so a provider reassignment can't strand the count.
CREATE OR REPLACE FUNCTION trg_update_active_mentee_count()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE user_profiles up
    SET active_mentee_count = (
        SELECT COUNT(*)::smallint
        FROM service_bookings sb
        JOIN service_listing_meta m ON m.id = sb.listing_meta_id
        WHERE sb.provider_user_id = up.user_id
          AND m.listing_kind = 'mentorship'
          AND sb.status = 'confirmed'
    )
    WHERE up.user_id = COALESCE(NEW.provider_user_id, OLD.provider_user_id)
       OR up.user_id = OLD.provider_user_id;
    RETURN NULL;
END; $$;

DROP TRIGGER IF EXISTS trg_mentee_count ON mentorship_connections;
DROP TABLE mentorship_connections;

CREATE TRIGGER trg_service_booking_mentee_count
AFTER INSERT OR UPDATE OF status, provider_user_id ON service_bookings
FOR EACH ROW EXECUTE FUNCTION trg_update_active_mentee_count();

-- 8. Nav seed repoint — Mentors item now lands on the mentorship kind
--    tab of the service directory.
UPDATE menu_items SET url = '/services?kind=mentorship'
WHERE url = '/mentors';

COMMIT;
```

**Down migration** — full reverse, no stubs: recreate `mentorship_connections` (same columns/indexes/UNIQUE), backfill FROM service_bookings WHERE `meta->>'source'='mentorship_connection'` (reverse status map, restore `responded_at` from meta), restore `trg_mentee_count` on it, drop the new trigger/columns/constraints/index, restore the two CHECKs, delete the seeded category only if unreferenced (`countListingsByCategory` equivalent), remove `mentor_listing_content_id`, delete shadow listings (`contents` + meta WHERE meta->kind='mentorship' and slug LIKE 'mentor-%'), revert the menu_items row.

- [ ] **Step 2:** `go run ./cmd/migrate/` (up). Verify: `service_bookings` count == old connections count; every `is_mentor` user has `mentor_listing_content_id`; `SELECT proname FROM pg_proc WHERE proname='trg_update_active_mentee_count'`; down-then-up round trip on a scratch DB.
- [ ] **Step 3:** Commit.

> ⚠️ Executor: before running, diff every identifier in the INSERT SELECTs against `migrations/000735_baseline_schema.up.sql` (`contents` NOT NULL columns beyond the listed set — e.g. `visibility`/`word_count` defaults) and against `000803` for `service_categories`. Adjust to the real schema; do not weaken NOT NULL columns silently.

### Task 3: sqlc queries — provider dimension + filters

**Files:**
- Modify: `internal/db/queries/services.sql`
- Modify: `internal/db/queries/gdpr.sql` (see Task 7 note — may be deferred to Phase C)
- Regen: `make sqlc-gen`

- [ ] **Step 1:** `CreateServiceBooking` gains `provider_user_id`, `destination_space_id`, `meta` (`sqlc.narg`).
- [ ] **Step 2:** `GetServiceBooking`, `GetServiceBookingDetail`, `ListServiceBookingsByUser`, `ListServiceBookingsByOrg` SELECT the new columns; detail/list joins add `LEFT JOIN users pu ON pu.id = sb.provider_user_id` + `LEFT JOIN user_profiles pup ON pup.user_id = pu.id` for `provider_username/provider_display_name/provider_avatar`, and `LEFT JOIN spaces dsp ON dsp.id = sb.destination_space_id` for `destination_space_name`. `ListServiceBookingsByUser` and the new provider query accept `sqlc.narg(kind)` filtering on `sm.listing_kind`.
- [ ] **Step 3:** New queries:
  - `ListServiceBookingsByProviderUser :many` + `CountServiceBookingsByProviderUser :one` — `sb.provider_user_id = @provider_user_id` + optional kind/status/intent filters, same joins as ByUser.
  - `GetMentorshipListingByAuthor :one` — meta JOIN contents WHERE `c.author_id=@author_id AND m.listing_kind='mentorship' AND c.deleted_at IS NULL` (the mentor's shadow listing).
  - `GetOpenMentorshipBooking :one` — pending OR confirmed booking for `(booker_user_id, listing_meta_id)` (dedupe, replaces `GetActiveOrPendingConnection`).
- [ ] **Step 4:** `make sqlc-gen` → `go build ./...` → commit.

### Task 4: servicedir — mentorship kind support

**Files:**
- `internal/servicedir/store.go` — `ListingKindMentorship = "mentorship"`; `IsValidListingKind` accepts it; `categoryAppliesTo` map in `service.go` gains `mentorship` (admin taxonomy CRUD accepts the new value).
- `internal/servicedir/service.go`
- `internal/servicedir/service_bookings.go`, `internal/servicedir/handlers_bookings.go`
- `internal/server/server.go`

- [ ] **Step 1 (tests first):** extend `internal/servicedir/service_bookings_test.go` + `service_content_test.go`:
  - `kind=mentorship` lists when `mentorship.enabled` on, 404 when off — even when `service_packages.enabled` is off.
  - Booking on user-owned mentorship listing succeeds (`provider_user_id` = author, `org_id` NULL, `expires_at` NULL); duplicate open request → error; capacity-full mentor → error.
  - `role=provider` list returns only the caller's provider bookings; `respond` 403s for strangers, works for provider user; `GetBooking` allows provider.
- [ ] **Step 2:** `go test ./internal/servicedir/ -v` → FAIL.
- [ ] **Step 3:** Implement:
  - `Service.MentorshipEnabled(ctx)` — `s.checker.IsEnabled(ctx,"mentorship.enabled")` fallback settings (same shape as `PackagesEnabled`).
  - `List()` — at service.go ~1767: when `!PackagesEnabled` the pin becomes `kind IN ('service','mentorship')` (mentorship rows still surface in the unfiltered list); `kind='mentorship'` request → require `MentorshipEnabled` (404 off). Package pin logic unchanged otherwise.
  - `CreateBooking()` — for `listing_kind='mentorship'`: skip the `PackagesEnabled` gate (require `MentorshipEnabled`), skip the org-required branch, set `provider_user_id = listing.AuthorID` (add `AuthorID` to the booking-path listing fetch if absent), force `intent='inquiry'`, `party_size=1`, `expires_at=NULL` (mentorship requests are not SLA-bound — keeps them out of `HandleServiceBookingExpirePending`), dedupe via `GetOpenMentorshipBooking` → `ErrDuplicateRequest`, capacity check (`active_mentee_count >= mentor_capacity` → `ErrMentorAtCapacity`), accept `goals` + `destination_space_id` input fields (goals → `note`; also store `goals` verbatim in `meta`). Reject `intent='booking'`/tiers/coupons on mentorship kind. Requires `booker != provider` (no self-requests).
  - `BookingDTO` gains `ProviderUserID/ProviderUsername/ProviderDisplayName/ProviderAvatar/DestinationSpaceID/DestinationSpaceName/Goals/ConnectionStatus` (Goals read from `meta->>'goals'` fallback `note`); `GetBooking` auth adds `row.ProviderUserID == viewerID` arm and guards `IsOrgMember` behind `row.OrgID.Valid`.
  - `HandleMyBookings` — new params `role=booker|provider` (default booker → `ListServiceBookingsByProviderUser`), `kind=` passthrough.
  - New `POST /me/service-bookings/{id}/respond` — body `{action:'accept'|'decline'|'complete', note?}`; service `RespondToBooking(ctx, providerID, bookingID, action, note)`: requires `sb.provider_user_id = providerID` (org-owned bookings are NOT served — they have the org route); transitions pending→confirmed|declined, confirmed→fulfilled; decline note → `org_response_note`. On `accept` of a mentorship-kind booking → DM auto-open (Task 5) + enqueue `mentor:update_meilisearch` for the provider.
  - Route wiring in `server.go` under the existing `me/service-bookings` block: `r.With(auth.RequireAuth, rbac.RequirePermission("mentorship.respond")).Post("/me/service-bookings/{id}/respond", ...)` — the node exists (used on `/me/mentorship/incoming/{id}/accept`); org bookings keep `service.booking.manage` via the org route.
- [ ] **Step 4:** Tests green → `go build ./...` → commit.

### Task 5: Worker notifications + DM auto-open for mentorship bookings

**Files:**
- `internal/worker/service_bookings.go` — `HandleServiceBookingCreated`/`notifyOrgMembers`/status-changed: when `!bk.OrgID.Valid && bk.ProviderUserID.Valid`, notify the provider user directly (`ActorType "user"`; `ActorID` = the action initiator — booker on created/disputed, provider on the booker-facing status arm), notification `action_url = "/messages/mentorship?b="+id` (booker arm uses `/messages/bookings?b=`).
- `internal/servicedir/service.go` — `SetDMCreator` seam (interface `CreateConversation(ctx, a, b pgtype.UUID, opts)`) injected in `server.go` from `wsDMStore` — the same object mentorshipService already receives at ~line 3891.
- `internal/servicedir/service_bookings.go` — `RespondToBooking` accept path for mentorship kind: open DM provider↔booker, enqueue `mentor:update_meilisearch` (payload `{user_id}` — same shape the mentorship service uses).

- [ ] **Step 1:** Provider-arm notification unit test (worker package has `testdb` helpers — follow `service_bookings_test.go` patterns there if present; otherwise cover at servicedir level).
- [ ] **Step 2:** Implement; `go build ./...`; `go test ./internal/worker/ ./internal/servicedir/`; commit.

---

## Phase C — Backend: mentorship compat layer

### Task 6: mentorship.Service delegates to servicedir

**Files:**
- `internal/mentorship/service.go`, `handlers.go`; `internal/server/mentorship_adapters.go`; `internal/server/server.go` (inject servicedir into mentorship service — add a narrow interface seam `type BookingBackend interface{...}` rather than importing the concrete type if layering demands it).

**Interface contract:** all existing response DTOs (`MentorSummary`, `Connection`, `MentorProfile`, `Program*`, `Match*`) stay byte-compatible — `agent_tool_writers.go` calls `mentorship` methods via reflection (signatures must not change).

- [ ] **Step 1:** `UpdateMentorProfile` — after the user_profiles write, upsert the shadow listing via servicedir: `is_mentor=true` → create-or-update `contents` row (slug `mentor-<username>`, title `display_name + " — Mentorship"`, body = mentor_bio, `content_hash` via `content.ComputeContentHashWithDistinguisher(bio, "mentor:"+userID)` — matches the migration distinguisher) + meta (kind/category `mentorship`, `is_remote=true`, `purchase_mode='inquiry'`), set `mentor_listing_content_id`; `is_mentor=false` → soft-delete the contents row (keep the column link for audit? NO — clear it, SET NULL via FK if row deleted).
- [ ] **Step 2:** `ListMentors`/`GetMentorByUsername`/`SearchMentors`/`GetMentorsByIDs`/`CompareMentors` — query `service_listing_meta` (kind=mentorship, contents published, `deleted_at IS NULL`) JOIN user_profiles/users; keep `sort=match` (60% keyword + 40% pgvector) — reuse the existing `ListMentorsByMatch` SQL shape against the joined set; `available_slots` still `mentor_capacity - active_mentee_count`.
- [ ] **Step 3:** `CreateConnection` → resolve shadow listing → `servicedir.CreateBooking(mentee, listingContentID, {intent:'inquiry', note:goals, destination_space_id})` → return Connection DTO built from the booking (id = booking id).
- [ ] **Step 4:** `ListOutgoingConnections`/`ListIncomingConnections`/`CancelOutgoingConnection`/`AcceptConnection`/`DeclineConnection`/`CompleteConnection`/`CancelConnection`/`ListAllConnections`/`AdminUpdateConnection` → servicedir reads + `RespondToBooking`/booker-cancel, filtered kind=mentorship; DTO maps booking status BACK to connection vocabulary (`confirmed`→`active`, `fulfilled`→`completed`).
- [ ] **Step 5:** Update `internal/mentorship` tests to the new plumbing; `go test ./internal/mentorship/ ./internal/servicedir/ -v`; `go build ./...`; commit.

### Task 7: GDPR + downstream consumers sweep

- [ ] **Step 1:** `db/queries/gdpr.sql` — remove the `mentorship_connections` statement (line ~233 of gen output); mentor-PII block also clears `mentor_listing_content_id`; verify the booker-erasure booking wipe also clears `note`/`destination_space_id`/`meta` and that provider erasure sets `provider_user_id` NULL (FK covers hard delete; anonymize path needs an explicit UPDATE — check existing `service_bookings` anonymize query and add `provider_user_id` arm). `make sqlc-gen` after editing.
- [ ] **Step 2:** `grep -rln "mentorship_connections\|MentorshipConnection" internal/` — every remaining reference must resolve to the new queries or the compat DTO (`connectionFromModel` currently takes `gen.MentorshipConnection` — retype it).
- [ ] **Step 3:** Verify `apps/admin` `/mentorship` page still works against the compat endpoints (`/admin/mentorship/connections` → bookings); `internal/comparison/resolver/mentor.go` + `internal/recommendations/handlers.go` + `internal/server/agent_tool_writers.go` compile unchanged (adapter signatures stable).
- [ ] **Step 4:** `go build ./... && go test ./internal/...` scoped; commit.

---

## Phase D — Frontend: `/services` chrome strip

### Task 8: Strip marketplace chrome, keep LeftNav

**Files:**
- Modify: `apps/main/src/routes/(marketplace)/+layout.svelte` — remove `MarketplaceHeader` + `MarketplaceFooter` (imports + render); keep `LeftNav` sticky aside + `ComposeModal`. `.marketplace-nav` sticky `top` becomes `1rem` (no `--nav-height` header above); `.marketplace-center` gets `px-4 sm:px-6 lg:px-8` + `py-6` margins. `(marketplace)/+layout.server.ts` unchanged (host guards).
- The `(marketplace)` group then contains ONLY `/services` (detail + reviews move to `(app)` in Phase E).

- [ ] **Step 1:** Edit layout; confirm no other `(marketplace)` children depend on header/footer.
- [ ] **Step 2:** `cd apps/main && bun run check`; visual verify `localhost:3000/services` desktop (LeftNav + margined content, no header/footer) + mobile (content + breadcrumb only — accepted).
- [ ] **Step 3:** Commit.

---

## Phase E — Frontend: `/services/[slug]` inside the shell

### Task 9: Move detail + reviews into `(app)`

**Files:**
- `git mv` `(marketplace)/services/[slug]/` → `(app)/services/[slug]/` (page + server; `edit/` sibling already exists — directories merge).
- `git mv` `(marketplace)/services/[slug]/reviews/` → `(app)/services/[slug]/reviews/`.
- Modify: `apps/main/src/lib/components/create/gate.ts` — add `'/(app)/services/[slug]'` to `NO_TAB_BAR_ROUTES` (StickyBookBar owns the mobile bottom edge).

- [ ] **Step 1:** Move the routes; `(marketplace)` keeps only `/services`.
- [ ] **Step 2:** Rebuild detail layout for the shell's center column (~600–720px):
  - Stack the hero (currently `lg:grid-cols-2`): breadcrumb → badges/title/rating → featured image → key-facts strip → CTA row → `.prose` body → `IncludedGrid`/`TierGrid`/`CouponCard` → departures → `HowItWorks` → `ServiceFaq` → `ServiceReviews`.
  - `ProviderCard` + `#book` summary card move into the right sidebar via `SetSidebar` (`apps/main/src/lib/components/shell/SetSidebar.svelte` — route injects a snippet into the `appshell:sidebar` context).
  - `StickyBookBar`, `BookingDialog`, claim flow, `AppealButton`, JSON-LD, view tracking unchanged.
- [ ] **Step 3:** `bun run check`; verify desktop (provider in right sidebar) + mobile (sticky bar, no double bottom bar); commit.

### Task 10: Mentorship kind — list tab, card variant, request dialog

**Files:**
- Create: `apps/main/src/lib/components/marketplace/MentorshipRequestDialog.svelte` — Bits UI `Dialog.Root`; goals textarea + destination `Autocomplete` (reuse the fetcher shape from the old `/mentors/[username]` page) → `POST /services/{id}/bookings` `{intent:'inquiry', note: goals, destination_space_id}`; success → link to `/messages/mentorship`.
- Modify: `(app)/services/[slug]/+page.svelte` — when `service.listing_kind === 'mentorship'`: render mentor block (avatar, `mentor_areas` chips, `available_slots`/`mentor_capacity`, `mentor_since`, match score when the payload carries it) instead of tiers/departures; primary CTA "Request mentorship" opens the dialog; capacity-full → disabled + note. Requires the Task-4-enriched `mentor` object in the detail payload.
- Modify: `(marketplace)/services/+page.svelte` — kind tabs gain `Mentorship` (`kind=mentorship`) gated by `features['mentorship.enabled']`; `ServiceCard` mentorship variant renders the provider avatar/name fallback instead of a missing featured image.
- Modify (stretch): `(app)/services/new/+page.svelte` — add `mentorship` to the Listing type selector for orgs/users listing mentorship offerings (category auto-pins to `mentorship`).

- [ ] **Step 1:** Dialog + i18n keys (`en.json` + `fr.json`).
- [ ] **Step 2:** Wire mentorship variant into detail + list.
- [ ] **Step 3:** `bun run check`; verify `/services?kind=mentorship` + a mentor listing request flow end-to-end; commit.

---

## Phase F — Frontend: `/messages` tabs + redirects

### Task 11: `/messages/bookings` + `/messages/mentorship` routes

**Files:**
- Create: `(app)/messages/bookings/+page.{svelte,server.ts}` and `(app)/messages/mentorship/+page.{svelte,server.ts}` — replicate the `ListPageBar`+`UrlTabs` header pattern from `/messages/requests`; tabs `Messages | Requests | Bookings | Mentorship` navigating across the four routes.
- Create: `apps/main/src/lib/components/messages/BookingListPane.svelte` — two-pane list: `role` sub-filter `sent|received`, status pills, booking rows (listing title, status chip, provider/booker name, time) → `?b=<id>` selects.
- Create: `apps/main/src/lib/components/messages/BookingDetailPane.svelte` — right pane: listing link, status timeline, goals/note, destination, party/dates/tier/coupon, price snapshot, `org_response_note`; booker actions (cancel/dispute/pay-resume — port logic from `(app)/me/service-bookings` + `/[id]`); provider actions via `POST /me/service-bookings/{id}/respond` (accept/decline/complete); "Open conversation" button → resolves/links the DM thread (respond response should include `dm_conversation_id` when auto-opened — add to BookingDTO/result if cheap, else "Open messages" → `/messages`).
- Modify: `(app)/messages/+page.svelte` + `requests/+page.svelte` — extend `UrlTabs` to all four; hide tabs by flag (`feature.dm` gates Messages/Requests; `service_directory.enabled` gates Bookings; `mentorship.enabled` gates Mentorship).
- Modify: `apps/main/src/lib/types/index.ts` — `ServiceBooking` gains `provider_user_id/provider_username/provider_display_name/provider_avatar/destination_space_id/destination_space_name/goals`.

**Flag-gating note:** `/messages/bookings` and `/messages/mentorship` MUST NOT inherit the `feature.dm` 404 (bookings are usable without DM). Gate: auth + the relevant feature flag only.

- [ ] **Step 1:** Server loads — bookings page fetches `GET /me/service-bookings?role=booker&kind=` AND `?role=provider&kind=` + `GET /me/service-bookings/{id}` when `b=` present; mentorship page same with `kind=mentorship`.
- [ ] **Step 2:** Build panes + tab wiring + i18n keys.
- [ ] **Step 3:** `bun run check`; exercise mentor accept → DM-open → commit.

### Task 12: Redirects + link sweep

**Files:**
- `(app)/me/service-bookings/+page.server.ts` → `redirect(308, '/messages/bookings')`
- `(app)/me/service-bookings/[id]/+page.server.ts` → `redirect(308, '/messages/bookings?b='+id)`
- `(app)/mentors/+page.server.ts` → `redirect(308, '/services?kind=mentorship')`
- `(app)/mentors/[username]/+page.server.ts` → resolve `mentor-<username>` listing slug server-side → `redirect(308, '/services/<slug>')` (fallback `/services?kind=mentorship`)
- `(app)/mentors/me/+page.server.ts` → `redirect(308, '/messages/mentorship')`
- `routes/mentorship/compare/+page.server.ts` → `redirect(308, '/compare/mentors')`
- Delete the dead `+page.svelte` files under redirected routes (server files become pure redirects).
- Update links: `/me/service-bookings` `PageAction` in `(marketplace)/services/+page.svelte` → `/messages/bookings`; `routeFeatureFlags.ts` `/mentors` entry → repoint; worker notification `action_url`s (done in Task 5).
- Keep `/org/[slug]/service-bookings` (org provider inbox — out of redirect scope).

- [ ] **Step 1:** Redirects + link updates.
- [ ] **Step 2:** `bun run check`; click-through each redirect; commit.

---

## Phase G — Verification

- [ ] `cd apps/main && bun run check` — zero errors.
- [ ] `go build ./... && go vet ./...` — clean.
- [ ] `go test ./internal/servicedir/ ./internal/mentorship/ ./internal/worker/` — green.
- [ ] Migration round-trip on a scratch DB (up → verify → down → up).
- [ ] Manual sweep on `localhost:3000`: `/services` (no header/footer, margins, LeftNav); `/services/{slug}` in shell (provider in right sidebar, sticky bar on mobile); `/services?kind=mentorship`; mentor listing → Request Mentorship → lands in mentor's `/messages/mentorship` → accept → DM opens; `/messages/bookings` sent/received + respond actions; every redirect target.
- [ ] Admin app `/mentorship` page still renders via compat endpoints.
- [ ] `rg "me/service-bookings|/mentors|mentorship_connections" apps/main/src internal/` — remaining hits only in compat layer, org inbox, or redirect files.
