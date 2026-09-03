# Design — Unified User Tiers and Professional Profile Removal

**Date:** 2026-09-03  
**Status:** Approved for implementation  
**JEOS refs:** §02 (users), §03 (RBAC / user groups), §07 (organizations), §14 (content authorship), §17 (payments / subscription plans), §31 (follows), §33 (professional profiles), §68 (AI personas)

## Goal

Remove the separate Professional Profiles feature and replace it with a unified user-tier model: **regular, pro, business**. A user's tier is controlled through `user_groups`, is surfaced as `users.profile_type`, and gates platform capabilities such as how many organizations the user can create. Tier limits are configurable by admins through per-tier `system_settings` keys.

## Decisions from clarification

- **Remove Professional Profiles entirely.** Drop the `professional_profiles`, `professional_profile_types`, `professional_profile_members`, and `professional_profile_invitations` tables; remove all routes, frontend pages, and references.
- **Reparent existing data.** Content authored as a professional profile stays with `author_user_id`; `author_profile_id` is removed. Follows of professional profiles become follows of the owner user. AI personas use the bot user as the public byline.
- **Tier source of truth is `user_groups`.** Add `user_groups.tier` with values `regular / pro / business`. A user's effective tier is the highest tier among their group memberships.
- **`users.profile_type` is derived and denormalized.** It is renamed to `regular / pro / business`, updated by the tier-change service, and used for fast public display and filtering.
- **Tier limits use `system_settings` per-tier keys.** `organizations.max_per_user.{regular,pro,business}` is the first example. Future limits (AI queries, API quotas, credit discounts, etc.) can be added as new `system_settings` keys without schema or core tier changes.
- **Subscription plans map to tiers.** Add `subscription_plans.tier` so `/pricing` plans can be configured to grant `pro` or `business` when billing is wired. For now, tier assignment is admin-driven; recurring subscription creation remains out of scope.
- **New users default to `regular`.** Registration adds the user to the base `members` tier group (tier `regular`) and sets `profile_type = 'regular'`.

## Data model

### `user_groups` additions

```sql
ALTER TABLE user_groups
  ADD COLUMN tier text CHECK (tier IN ('regular','pro','business')),
  ADD COLUMN max_organizations_creatable int NOT NULL DEFAULT 0;
```

The `members` system group is assigned `tier = 'regular'` with a configurable default (`organizations.max_per_user.regular`).
Two new system groups are seeded:

| group slug | tier | `max_organizations_creatable` fallback |
|---|---|---|
| `pro` | `pro` | `organizations.max_per_user.pro` |
| `business` | `business` | `organizations.max_per_user.business` |

These groups carry no extra RBAC permission nodes beyond those already granted to `members`.

### `subscription_plans` additions

```sql
ALTER TABLE subscription_plans ADD COLUMN tier text CHECK (tier IN ('regular','pro','business'));
```

The public `/pricing/plans` endpoint exposes this mapping so the frontend can show the tier each plan unlocks.

### `users.profile_type`

```sql
ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_profile_type_check,
  ADD CONSTRAINT users_profile_type_check CHECK (profile_type IN ('regular','pro','business'));

UPDATE users
SET profile_type = CASE profile_type
  WHEN 'personal' THEN 'regular'
  WHEN 'business' THEN 'business'
  WHEN 'organisation' THEN 'business'
  ELSE 'regular'
END;
```

### Professional Profile removal

```sql
-- 1. Remove FK dependencies on professional_profiles.
ALTER TABLE contents     DROP COLUMN IF EXISTS author_profile_id;
ALTER TABLE comments     DROP COLUMN IF EXISTS author_profile_id;
ALTER TABLE ai_personas  DROP CONSTRAINT IF EXISTS ai_personas_profile_id_fkey,
                         ALTER COLUMN profile_id DROP NOT NULL;

-- 2. Convert professional_profile follows to user follows.
UPDATE follows
SET entity_type = 'user',
    entity_id = (SELECT owner_user_id FROM professional_profiles WHERE id = entity_id)
WHERE entity_type = 'professional_profile';

-- 3. Drop the Professional Profile tables.
DROP TABLE IF EXISTS professional_profile_invitations;
DROP TABLE IF EXISTS professional_profile_members;
DROP TABLE IF EXISTS professional_profiles;
DROP TABLE IF EXISTS professional_profile_types;
```

## Tier resolution

1. **Get user group memberships.** Query `user_group_memberships` joined to `user_groups`.
2. **Pick the highest tier.** `business` > `pro` > `regular`.
3. **Resolve per-tier settings.** Read `system_settings` keys with the tier suffix, e.g. `organizations.max_per_user.{tier}`.

Helper (conceptual):

```go
func ResolveUserTier(ctx, userID, tenantID) (string, error)
func GetTierSetting(ctx, tenantID, tier, key) (string, error)
```

## Backend changes

### Tier management

- New `internal/tiers` package with:
  - `ResolveUserTier(ctx, userID, tenantID) (string, error)`
  - `SetUserTier(ctx, userID, tenantID, tier string) error` — adds/removes tier group memberships and syncs `users.profile_type`.
- Update `PATCH /api/v1/admin/users/{id}/profile-type` to accept `regular/pro/business` and call `SetUserTier`.
- Update `GET /api/v1/admin/users/moderation-config` profile type options.
- Update `ListPublicUsers` to filter by the new `profile_type` values.

### Organization creation limits

- Update `internal/organizations/service.go:CreateOrganization`:
  - Resolve the user's tier.
  - Read `organizations.max_per_user.{tier}` from `system_settings` (fallback to existing `organizations.max_per_user` during transition).
  - Count existing organizations owned by the user and enforce the limit.
- Add `organizations.max_per_user.{regular,pro,business}` to `internal/control/settings_catalog.go`.

### Professional Profile removal

- Delete `internal/professional` package.
- Delete `internal/db/queries/professional_profiles.sql` and `internal/db/queries/professional_extras.sql`.
- Remove `/api/v1/profiles/*` routes from `internal/server/server.go`.
- Update `internal/follows` to remove `professional_profile` entity handling.
- Update `internal/personas` to remove `professional_profiles` joins; use the bot `users` row for byline.
- Update `internal/widgets`, `internal/campaign/planner.go`, `internal/payments/handlers.go` feature list, and any other references found in the codebase search.
- Remove `feature.professional` from `internal/features/registry.go`.
- Update `internal/gdpr` erasure/export logic to remove professional profile handling.

### Registration

- After `CreateUser`, add the new user to the tenant's `members` group and set `users.profile_type = 'regular'`.

## Frontend changes

- Remove `apps/main/src/routes/(app)/dashboard/profiles/*`.
- Remove `apps/admin/src/routes/professional-types/+page.svelte` and persona professional-profile UI.
- Update `AuthorContextSwitcher.svelte`, `FollowListModal.svelte`, and `RelationshipsView.svelte` to remove professional profile authorship.
- Update `/u/[username]` to remove professional profile rendering.
- Update pricing page to show tier badge for each plan (`subscription_plans.tier`).
- Update i18n keys and TypeScript types for `profile_type`.

## AI personas

- `ai_personas.profile_id` becomes nullable.
- Personas display using the bot `users` row (display name, avatar, bio).
- Existing personas: set `profile_id = NULL`; the UI resolves byline from `user_id`.

## Data migration plan

1. Add `user_groups.tier` and `subscription_plans.tier` columns.
2. Seed `pro` and `business` tier groups.
3. Set `members` group `tier = 'regular'` and default `max_organizations_creatable`.
4. Add `organizations.max_per_user.{regular,pro,business}` settings with defaults.
5. Backfill `user_group_memberships` for all existing users:
   - All users → `members` group.
   - `profile_type = 'business' or 'organisation'` → also `business` group.
   - `profile_type = 'personal'` → `members` only.
6. Update `users.profile_type` to `regular / pro / business`.
7. Remove Professional Profile columns and tables.
8. Convert professional profile follows to user follows.

## Catalog updates

- `.jeos/sections/33-professional-profiles.md` — mark as deprecated/removed.
- `.jeos/sections/02-authentication-sessions.md` — update `users.profile_type` values.
- `.jeos/sections/07-organization-profiles-claim-ownership.md` — document per-tier organization creation limits.
- `conventions/system-settings-catalog.md` — add `organizations.max_per_user.{regular,pro,business}`.
- `conventions/api-routes-catalog.md` — remove professional profile routes; update `/admin/users/{id}/profile-type`.

## Testing plan

- `go test ./internal/organizations/...` — verify tier-based org limits.
- `go test ./internal/users/...` — verify `SetUserTier` and `profile_type` sync.
- `go test ./internal/follows/...` — verify professional profile follow migration.
- `go build ./...` and `go vet ./...` clean.
- `make sqlc-gen` and verify generated `internal/db/gen` changes.
- `bun run check` in `apps/main` and `apps/admin`.
- Migration tests for data backfill and professional profile removal.
- Manual verification: admin tier change, organization creation limit, public `/users` filter, `/pricing/plans` tier display.

## Out of scope

- Recurring subscription billing and webhook-driven tier changes. `subscription_plans.tier` is stored and exposed for future billing integration, but the actual payment/subscription creation remains unchanged.
- New per-tier limits beyond `organizations.max_per_user.{regular,pro,business}`. The architecture supports them; each new limit will be added as a `system_settings` key when the consuming feature is built.
