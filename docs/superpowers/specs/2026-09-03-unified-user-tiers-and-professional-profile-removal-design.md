# Design — Unified User Tiers and Professional Profile Removal

**Date:** 2026-09-03  
**Status:** Approved for implementation  
**JEOS refs:** §02 (users), §03 (RBAC / user groups), §07 (organizations), §14 (content authorship), §17 (payments / subscription plans), §31 (follows), §33 (professional profiles), §68 (AI personas)

## Goal

Remove the separate Professional Profiles feature and replace it with a unified user-tier model: **regular, pro, business**. A user's tier is controlled through `user_groups`, is surfaced as `users.profile_type`, and gates platform capabilities including space creation, organization creation, AI query limits, API rate limits, credit pack discounts, and access to V2 tools. Tier limits and feature access are configurable by admins through per-tier `system_settings` keys and RBAC permission group roles.

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

| group slug | tier | `max_spaces_creatable` | `max_organizations_creatable` fallback | group role |
|---|---|---|---|---|
| `members` | `regular` | `3` | `organizations.max_per_user.regular` | `member` (base permissions) |
| `pro` | `pro` | `-1` (unlimited) | `organizations.max_per_user.pro` | `pro_member` |
| `business` | `business` | `-1` (unlimited) | `organizations.max_per_user.business` | `business_member` |

The `pro_member` and `business_member` roles are seeded with the base `member` permissions plus feature nodes such as `api.keys.self_manage`, `ai.assistant.unlimited` (or the `ai.assistant.daily_queries` setting keyed to the user's tier), and any V2 tool `use` nodes the admin chooses to grant.

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

## Tier-gated capabilities and settings

The tier model is the single place to resolve both **numeric limits** and **feature access**. New capabilities can be added by creating a `system_settings` key with the user's tier as the suffix, or by assigning a permission node to a tier group role. The core tier resolution code does not change when a new feature is added.

### Numeric limits (per-tier `system_settings` keys)

| Capability | Setting key | Default values (regular / pro / business) | Enforced in |
|---|---|---|---|
| Spaces per user | *(uses `user_groups.max_spaces_creatable`)* | `members = 3`, `pro = -1`, `business = -1` | `GetUserMaxSpacesCreatable` |
| Organizations per user | `organizations.max_per_user.{tier}` | `1` / `5` / `-1` | `CreateOrganization` |
| AI assistant daily queries | `ai.assistant.daily_queries.{tier}` | `10` / `100` / `-1` | `internal/ai/HandleChat` |
| API rate limit tier | `api.rate_limit.{tier}` | `100` / `1000` / `10000` req/min | `publicapi` middleware (via `api_keys.rate_limit_tier`) |
| Content posts per day | `content.posts_per_day.{tier}` | `5` / `-1` / `-1` | `CreateContent` |
| Credit pack discount | `credits.pack_discount_pct.{tier}` | `0` / `0` / `10` | `payments` checkout |

A naming convention is established: `feature.area.metric.{tier}`. Admins configure each key in the control panel. Code resolves the user's effective tier once, then reads `GetSetting(ctx, tenantID, keyPrefix + "." + tier)`.

### Feature access (RBAC permission nodes)

Tier groups are linked to roles. The `pro` and `business` groups get the `pro_member` and `business_member` roles, which are seeded with the base `member` permissions plus feature `use` nodes. Examples:

- `cv_formatter.use`
- `housing_guide.use`
- `scholarship_finder.use`
- `mentorship.use`
- `api.keys.self_manage`

A user in the `pro` group inherits the `pro_member` role; a user in the `business` group inherits the `business_member` role. The frontend uses the `permissions` array from `GET /api/v1/me` to show/hide feature links. The backend uses `rbac.RequirePermission` on the corresponding routes.

### API rate limit mapping

API keys have a `rate_limit_tier` (`basic` / `standard` / `premium`). When a user creates an API key, their user tier caps the highest rate tier they may request:

| User tier | Allowed API key rate tier |
|---|---|
| `regular` | `basic` |
| `pro` | `standard` |
| `business` | `premium` |

This maps the existing `api.rate_limit.{basic,standard,premium}` keys directly to the user tier, so no new rate-limit middleware is required.

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

### Spaces

- `user_groups.max_spaces_creatable` already exists. The tier groups are seeded with:
  - `members = 3`, `pro = -1` (unlimited), `business = -1` (unlimited).
- `GetUserMaxSpacesCreatable` already uses `MAX(ug.max_spaces_creatable)` across the user's groups, so no new code is required. The migration only sets the column values for the seeded tier groups.

### API rate limits

- `internal/publicapi/service.go:CreateAPIKey` resolves the user's tier and caps `input.RateLimitTier`:
  - `regular` → `basic`
  - `pro` → `standard`
  - `business` → `premium`
- The existing `api.rate_limit.{basic,standard,premium}` `system_settings` keys and `publicapi` middleware continue to enforce per-key rate limits.

### AI assistant query limits

- Add `ai.assistant.daily_queries.{regular,pro,business}` to `internal/control/settings_catalog.go`.
- Update `internal/ai/handlers.go:HandleChat` (or the message handler) to:
  - Resolve the user's tier.
  - Read the daily query limit from `system_settings`.
  - Track usage in a new Valkey counter `jc:ai:queries:{tenant_id}:{user_id}:{date}` with a 24h TTL.
  - Return `429 quota_exceeded` when the limit is reached.

### Content and credit packs

- Add `content.posts_per_day.{regular,pro,business}` to the catalog. `CreateContent` enforces a daily post cap per user tier.
- Add `credits.pack_discount_pct.{regular,pro,business}` to the catalog. The credit pack checkout applies the discount for the user's tier.

### Feature permissions

- Seed `pro_member` and `business_member` roles with base member permissions plus tier-specific feature `use` nodes.
- Assign the roles to the `pro` and `business` tier groups via `group_roles`.
- The backend uses `rbac.RequirePermission` and the frontend uses the `permissions` array from `/me` to gate V2 tools.

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

1. Add `user_groups.tier`, `user_groups.max_organizations_creatable`, and `subscription_plans.tier` columns.
2. Seed `pro` and `business` system tier groups with `max_spaces_creatable` and `max_organizations_creatable` values.
3. Set `members` group `tier = 'regular'` and its `max_spaces_creatable` / `max_organizations_creatable` defaults.
4. Add per-tier `system_settings` keys to the catalog:
   - `organizations.max_per_user.{regular,pro,business}`
   - `ai.assistant.daily_queries.{regular,pro,business}`
   - `content.posts_per_day.{regular,pro,business}`
   - `credits.pack_discount_pct.{regular,pro,business}`
5. Backfill `user_group_memberships` for all existing users:
   - All users → `members` group.
   - `profile_type = 'business' or 'organisation'` → also `business` group.
   - `profile_type = 'personal'` → `members` only.
6. Update `users.profile_type` to `regular / pro / business`.
7. Remove Professional Profile columns and tables.
8. Convert professional profile follows to user follows.
9. Seed `pro_member` and `business_member` roles, assign them to the `pro` and `business` groups, and grant base + tier-specific feature nodes.

## Catalog updates

- `.jeos/sections/33-professional-profiles.md` — mark as deprecated/removed.
- `.jeos/sections/02-authentication-sessions.md` — update `users.profile_type` values.
- `.jeos/sections/07-organization-profiles-claim-ownership.md` — document per-tier organization creation limits.
- `conventions/system-settings-catalog.md` — add per-tier keys: `organizations.max_per_user.{regular,pro,business}`, `ai.assistant.daily_queries.{regular,pro,business}`, `content.posts_per_day.{regular,pro,business}`, `credits.pack_discount_pct.{regular,pro,business}`.
- `conventions/valkey-keys-catalog.md` — add `jc:ai:queries:{tenant_id}:{user_id}:{date}` for AI daily query counting.
- `conventions/api-routes-catalog.md` — remove professional profile routes; update `/admin/users/{id}/profile-type`; document per-tier rate limit and query limit behavior.
- `conventions/permissions-catalog.md` (or `sections/03-rbac.md`) — add tier-specific `use` permission nodes (e.g., `cv_formatter.use`, `housing_guide.use`) and document `pro_member` / `business_member` roles.

## Testing plan

- `go test ./internal/organizations/...` — verify tier-based org limits.
- `go test ./internal/users/...` — verify `SetUserTier` and `profile_type` sync.
- `go test ./internal/follows/...` — verify professional profile follow migration.
- `go build ./...` and `go vet ./...` clean.
- `make sqlc-gen` and verify generated `internal/db/gen` changes.
- `bun run check` in `apps/main` and `apps/admin`.
- Migration tests for data backfill and professional profile removal.
- `go test ./internal/publicapi/...` — verify API key rate limit tier is capped by user tier.
- Manual verification: admin tier change, organization creation limit, public `/users` filter, `/pricing/plans` tier display.

## Out of scope

- Recurring subscription billing and webhook-driven tier changes. `subscription_plans.tier` is stored and exposed for future billing integration, but the actual payment/subscription creation remains unchanged.
- Complete wiring of every per-tier feature in this single PR. The first implementation installs the tier system, enforces `organizations.max_per_user.{tier}`, and seeds the `pro` / `business` tier groups with `max_spaces_creatable`, `pro_member`, and `business_member` roles. AI query limits, content post limits, credit pack discounts, and V2 tool permission nodes will be fully implemented as each consuming feature is built, but the settings and permission catalog entries are added now.
