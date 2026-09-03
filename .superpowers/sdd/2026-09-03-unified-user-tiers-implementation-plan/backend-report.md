# Backend Implementation Report — Unified User Tiers & Professional Profile Removal

**Plan:** `docs/superpowers/plans/2026-09-03-unified-user-tiers-implementation-plan.md`  
**Spec:** `docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md`  
**Commit:** `4a0719ed` on `main`

## Scope

Backend Tasks 4–9 of the implementation plan were completed in a single
conventional commit (the changes are tightly coupled through shared generated
SQL and `internal/server/server.go` wiring, so a single commit keeps the repo
compilable). The earlier Tasks 1–3 (migrations, `user_groups.tier`, and
per-tier system settings) were already in place.

| Task | Status | Key deliverables |
|------|--------|------------------|
| 4 | Done | `internal/tiers`, `internal/db/queries/tiers.sql`, generated `tiers.sql.go` |
| 5 | Done | `users` admin profile-type endpoint now uses `regular/pro/business` and the tier service |
| 6 | Done | `organizations` `CreateOrganization` reads `organizations.max_per_user.{tier}` |
| 7 | Done | `publicapi` rate limiter caps API key tier by the key owner’s resolved user tier |
| 8 | Done | `internal/professional` package, query files, generated code, and server routes removed |
| 9 | Done | `follows`, `personas`, `widgets`, `campaigns`, `payments`, `gdpr`, and `admin/catalog` updated |

## Major code changes

### 1. `internal/tiers` (Task 4)

New package:

- `internal/db/queries/tiers.sql` — `ResolveUserTier`, `GetTierGroupID`, `RemoveUserFromTierGroups`, `AddUserToTierGroup`
- `internal/db/gen/tiers.sql.go` (generated)
- `internal/tiers/tiers.go:49` — `ResolveUserTier` returns `regular/pro/business` from `user_groups.tier`
- `internal/tiers/tiers.go:69` — `SetUserTier` adds/removes tier group rows and updates `users.profile_type`
- `internal/tiers/tiers.go:129` — `GetTierSetting` helper for per-tier setting keys
- `internal/tiers/store.go:7` — `Store` wrapper for sqlc readers/writers
- `internal/tiers/tiers_test.go` and `tiers_integration_test.go`

### 2. Users admin endpoints (Task 5)

- `internal/users/service.go:85` — new `SetTierService` dependency injection
- `internal/users/service.go:540` — `GetUserModerationConfig` now advertises `regular/pro/business` profile types
- `internal/users/service.go:1430` — `UpdateProfileType` routes through the tier service when available
- `internal/users/handlers.go:612` — `UpdateProfileType` handler validates `regular/pro/business` (`internal/users/handlers.go:638`)
- `internal/users/users_test.go:30` — test fixtures updated from `personal` to `regular`

### 3. Organization creation tier limits (Task 6)

- `internal/organizations/organizations.go:79` — new `TierResolver` interface
- `internal/organizations/organizations.go:100` — `Service.tierResolver` field
- `internal/organizations/organizations.go:140` — `SetTierResolver`
- `internal/organizations/service.go:105` — `CreateOrganization` now:
  1. Resolves the creator’s effective tier.
  2. Reads `organizations.max_per_user.{tier}`.
  3. Falls back to the legacy `organizations.max_per_user` key.
  4. Enforces `0 = disabled`, `-1 = unlimited`, `N = cap` using `CountOrganizationsByOwner`.

### 4. API key rate-limit tier cap (Task 7)

- `internal/publicapi/service.go:54` — `TierResolver` interface
- `internal/publicapi/service.go:338` — `SetTierResolver`
- `internal/publicapi/service.go:344` — `ResolveUserTier` method on `*Service`
- `internal/publicapi/middleware.go:185` — `capRateLimitTier` maps `regular→basic`, `pro→standard`, `business→premium`
- `internal/publicapi/middleware.go:50` — `SlidingWindowRateLimitWithSettings` now takes a `TierResolver`
- `internal/publicapi/ratelimit.go:76` — middleware uses the capped tier for `resolveTierLimit`
- `internal/publicapi/ratelimit_test.go` — updated unlimited-bypass test to assert the new cap behavior
- `internal/server/server.go:4126` — wired `publicAPIService` as the resolver for the rate-limiter

### 5. Professional profile removal (Task 8)

Deleted:

- `internal/professional/` (`handlers.go`, `professional.go`, `professional_test.go`, `service.go`, `store.go`)
- `internal/db/queries/professional_profiles.sql`
- `internal/db/queries/professional_extras.sql`
- `internal/db/gen/professional_profiles.sql.go`
- `internal/db/gen/professional_extras.sql.go`
- Professional profile routes and admin verify routes in `internal/server/server.go`
- `professionalService` initialization and imports in `internal/server/server.go`

### 6. Dependent backend cleanup (Task 9)

- `internal/follows/follows.go:18` — removed `EntityTypeProfessionalProfile`
- `internal/follows/service.go:435` / `service.go:485` / `service.go:499` — removed professional-profile lookup/follower-count logic and validation
- `internal/follows/handlers.go:259` / `handlers.go:284` / `handlers.go:325` — removed `professional_profile` from type filter map and error messages
- `internal/follows/follows_test.go:32` and `follows_test.go:68` — tests updated
- `internal/personas/service.go:268` — persona byline now created as a `user_profile` (`display_name`/`bio`) instead of `professional_profiles`
- `internal/personas/service.go:352` — `CreateAIPersona` uses a zero `pgtype.UUID{}` for `ProfileID`
- `internal/personas/store.go:67` — removed `CreateProfessionalProfile` and `GetProfileTypeBySlug`
- `internal/campaign/planner.go:30` — `Planner.PlanPersonas` no longer takes `profileTypeDefaults`
- `internal/campaign/planner.go:70` — prompt no longer references professional profile types
- `internal/campaign/service.go:573` and `internal/campaign/planner_test.go` — call sites updated
- `internal/gdpr/service.go:1370` / `service.go:2180` / `service.go:2610` — removed professional-profile deletion and export collection
- `internal/gdpr/store.go:328` — removed professional-profile export helpers
- `internal/widgets/types.go:81` and `internal/widgets/renderer.go:1035` — removed `professional_profiles` widget type and renderer
- `internal/payments/handlers.go:172` — removed “Professional profile” from the Pro plan feature list
- `internal/admin/catalog.go:354` / `catalog.go:399` / `catalog.go:405` / `catalog.go:452` — removed `professional_profiles` widget catalog entries

### 7. Server wiring

- `internal/server/server.go:486` — `tierService := tiers.NewService(database)`
- `internal/server/server.go:730` — `orgService.SetTierResolver(tierService)`
- `internal/server/server.go:2103` — `publicAPIService.SetTierResolver(tierService)`
- `internal/server/server.go:2116` — `usersService.SetTierService(tierService)`
- `internal/server/server.go` — removed `internal/professional` import, `professionalService` initialization, and all `/profiles/*` / `/admin/profiles/{id}/verify` routes

### 8. SQL/generated SQL

- `internal/db/queries/tiers.sql` (new)
- `internal/db/queries/campaigns.sql`, `follows.sql`, `gdpr.sql`, `personas.sql`, `contents.sql` — professional profile references removed/reparented
- `internal/db/gen/tiers.sql.go` (new)
- `internal/db/gen/campaigns.sql.go`, `comments.sql.go`, `contents.sql.go`, `follows.sql.go`, `gdpr.sql.go`, `models.go`, `personas.sql.go`, `subscription_plans.sql.go`, `auth.sql.go` — regenerated to reflect tier columns and removed professional tables

## Validation

```bash
cd /home/jmobile/websites/japa/japaconnect
make sqlc-gen      # ok
go build ./...     # ok
go vet ./...       # ok
go test ./...      # ok
```

### Targeted package tests

All passed:

- `internal/tiers`
- `internal/users`
- `internal/organizations`
- `internal/publicapi`
- `internal/follows`
- `internal/personas`
- `internal/campaign`
- `internal/gdpr`

### Integration test

`go test -tags integration ./internal/tiers/...` failed at test harness setup:

```
applying migration 000605_add_feature_withdraw_flag.up.sql:
insert or update on table "feature_flags" violates foreign key constraint
"feature_flags_updated_by_fkey" (SQLSTATE 23503)
```

This is a pre-existing migration ordering issue (the `feature_flags` seed
references a user before the seed user is created). It is unrelated to the tier
or professional-profile removal work. The `japa_test` database was reachable on
`localhost:5432`; the harness simply could not apply the unrelated
`000605_add_feature_withdraw_flag` migration.

## Notes and follow-ups

- The `users.profile_type` CHECK constraint and `CreateUser` query already
  default to `regular` from the earlier migration work.
- The per-tier organization limit falls back to the legacy
  `organizations.max_per_user` key when the per-tier key is not set.
- API rate limits continue to use the existing `api.rate_limit.basic`,
  `api.rate_limit.standard`, and `api.rate_limit.premium` settings; the user
  tier only determines which of these three caps apply.
- `internal/professional` is fully removed from the build and route table.
- The working tree still contains unrelated wallet/learning/frontend changes
  that were not part of this backend task and were not committed.
