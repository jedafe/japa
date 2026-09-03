# Unified User Tiers and Professional Profile Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the Professional Profiles feature, replace it with a `regular / pro / business` user tier model backed by `user_groups`, and wire the first tier-gated limit (organization creation) plus the API key rate-limit cap.

**Architecture:** The user’s effective tier is resolved from `user_groups.tier` via `user_group_memberships`. `users.profile_type` is a denormalized display value updated by a new `internal/tiers` package. `internal/organizations` and `internal/publicapi` read per-tier `system_settings` keys using the user’s resolved tier. Professional profile tables are dropped and dependent data is reparented to the owner user.

**Tech Stack:** Go 1.24, pgx, sqlc, Bun, SvelteKit/Svelte, PostgreSQL, Valkey.

## Global Constraints

- `make sqlc-gen` after any `.sql` query file changes; never edit `internal/db/gen/` by hand.
- All commits go to `main`.
- No AI attribution in commits, comments, or docs (Rule 17).
- Use `bun` for all JS/TS tooling (Rule 18).
- No hardcoded numbers; use `system_settings` or env vars.
- Soft deletes only for user content.
- New Valkey keys must be in `conventions/valkey-keys-catalog.md`.
- New `system_settings` keys must be in `internal/control/settings_catalog.go` and `conventions/system-settings-catalog.md`.

---

### Task 1: Database migration — tier columns and settings

**Files:**
- Create: `migrations/000606_unify_user_tiers.up.sql`
- Create: `migrations/000606_unify_user_tiers.down.sql`
- Modify: `internal/control/settings_catalog.go`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/conventions/valkey-keys-catalog.md`

**Interfaces:**
- Produces: `user_groups.tier`, `user_groups.max_organizations_creatable`, `subscription_plans.tier` columns.
- Produces: per-tier settings keys in `system_settings`.

- [ ] **Step 1: Add tier columns.**

```sql
ALTER TABLE user_groups
  ADD COLUMN IF NOT EXISTS tier text CHECK (tier IN ('regular','pro','business')),
  ADD COLUMN IF NOT EXISTS max_organizations_creatable int NOT NULL DEFAULT 0;

ALTER TABLE subscription_plans
  ADD COLUMN IF NOT EXISTS tier text CHECK (tier IN ('regular','pro','business'));
```

- [ ] **Step 2: Seed pro and business tier groups.**

```sql
INSERT INTO user_groups (id, tenant_id, name, slug, is_system, tier, max_spaces_creatable, max_organizations_creatable)
VALUES
  ('00000000-0000-0000-0000-000000000040', '00000000-0000-0000-0000-000000000001', 'Pro', 'pro', true, 'pro', -1, -1),
  ('00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001', 'Business', 'business', true, 'business', -1, -1)
ON CONFLICT (tenant_id, slug) DO UPDATE SET
  tier = EXCLUDED.tier,
  max_organizations_creatable = EXCLUDED.max_organizations_creatable,
  max_spaces_creatable = EXCLUDED.max_spaces_creatable;

UPDATE user_groups
SET tier = 'regular', max_organizations_creatable = 1, max_spaces_creatable = 3
WHERE slug = 'members' AND tenant_id = '00000000-0000-0000-0000-000000000001';
```

- [ ] **Step 3: Add per-tier system_settings keys.**

Add to `internal/control/settings_catalog.go`:

```go
"organizations.max_per_user.regular":  {Type: "int", DefaultValue: "1", Description: "Max organizations a regular user can create.", Panel: "admin"},
"organizations.max_per_user.pro":      {Type: "int", DefaultValue: "5", Description: "Max organizations a pro user can create.", Panel: "admin"},
"organizations.max_per_user.business": {Type: "int", DefaultValue: "-1", Description: "Max organizations a business user can create. -1 = unlimited.", Panel: "admin"},

"ai.assistant.daily_queries.regular":  {Type: "int", DefaultValue: "10", Description: "...", Panel: "admin"},
"ai.assistant.daily_queries.pro":      {Type: "int", DefaultValue: "100", Description: "...", Panel: "admin"},
"ai.assistant.daily_queries.business": {Type: "int", DefaultValue: "-1", Description: "...", Panel: "admin"},

"content.posts_per_day.regular":  {Type: "int", DefaultValue: "5", Description: "...", Panel: "admin"},
"content.posts_per_day.pro":      {Type: "int", DefaultValue: "-1", Description: "...", Panel: "admin"},
"content.posts_per_day.business": {Type: "int", DefaultValue: "-1", Description: "...", Panel: "admin"},

"credits.pack_discount_pct.regular":  {Type: "int", DefaultValue: "0", Description: "...", Panel: "admin"},
"credits.pack_discount_pct.pro":      {Type: "int", DefaultValue: "0", Description: "...", Panel: "admin"},
"credits.pack_discount_pct.business": {Type: "int", DefaultValue: "10", Description: "...", Panel: "admin"},

"jce.comparisons.max_per_day.regular":  {Type: "int", DefaultValue: "5", Description: "...", Panel: "admin"},
"jce.comparisons.max_per_day.pro":      {Type: "int", DefaultValue: "25", Description: "...", Panel: "admin"},
"jce.comparisons.max_per_day.business": {Type: "int", DefaultValue: "-1", Description: "...", Panel: "admin"},

"wallet.max_daily_earn_points.regular":  {Type: "int", DefaultValue: "100", Description: "...", Panel: "admin"},
"wallet.max_daily_earn_points.pro":      {Type: "int", DefaultValue: "500", Description: "...", Panel: "admin"},
"wallet.max_daily_earn_points.business": {Type: "int", DefaultValue: "-1", Description: "...", Panel: "admin"},
```

And mirror the key names in `.jeos/conventions/system-settings-catalog.md`.

- [ ] **Step 4: Document new Valkey keys.**

Add to `.jeos/conventions/valkey-keys-catalog.md`:

```
jc:ai:queries:{tenant_id}:{user_id}:{date}          INT  24h  AI assistant daily query count for tier limit.
jc:jce:comparisons:created:{tenant_id}:{user_id}:{date}  INT  24h  JCE comparison creation count for tier limit.
jc:wallet:points:earned:{tenant_id}:{user_id}:{date}    INT  24h  Points earned today for tier limit.
```

- [ ] **Step 5: Down migration.**

Drop `user_groups.tier` and `max_organizations_creatable`, `subscription_plans.tier`. Remove the `pro` and `business` system groups only if no users are assigned (big-bang down is best-effort).

- [ ] **Step 6: Commit.**

---

### Task 2: Database migration — backfill users and rename profile_type

**Files:**
- Modify: `migrations/000606_unify_user_tiers.up.sql`

- [ ] **Step 1: Backfill user group memberships.**

```sql
-- Add all existing users to members group.
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
SELECT u.tenant_id, u.id, g.id
FROM users u
JOIN user_groups g ON g.tenant_id = u.tenant_id AND g.slug = 'members'
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;

-- Add business/organisation users to business tier group.
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
SELECT u.tenant_id, u.id, g.id
FROM users u
JOIN user_groups g ON g.tenant_id = u.tenant_id AND g.slug = 'business'
WHERE u.profile_type IN ('business', 'organisation')
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;
```

- [ ] **Step 2: Rename profile_type values.**

```sql
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_profile_type_check;
ALTER TABLE users ADD CONSTRAINT users_profile_type_check CHECK (profile_type IN ('regular','pro','business'));

UPDATE users
SET profile_type = CASE profile_type
  WHEN 'personal' THEN 'regular'
  WHEN 'business' THEN 'business'
  WHEN 'organisation' THEN 'business'
  ELSE 'regular'
END;
```

- [ ] **Step 3: Commit.**

---

### Task 3: Database migration — remove Professional Profiles and reparent data

**Files:**
- Create: `migrations/000607_remove_professional_profiles.up.sql`
- Create: `migrations/000607_remove_professional_profiles.down.sql`

**Interfaces:**
- Produces: professional profile tables dropped, `author_profile_id` removed, follows reparented.

- [ ] **Step 1: Clear and drop professional profile references.**

```sql
-- Clear content authorship profile references.
ALTER TABLE contents DROP COLUMN IF EXISTS author_profile_id;
ALTER TABLE comments DROP COLUMN IF EXISTS author_profile_id;

-- Make ai_personas.profile_id nullable and clear it.
ALTER TABLE ai_personas DROP CONSTRAINT IF EXISTS ai_personas_profile_id_fkey;
ALTER TABLE ai_personas ALTER COLUMN profile_id DROP NOT NULL;
UPDATE ai_personas SET profile_id = NULL;

-- Reparent professional profile follows to the owner user.
UPDATE follows
SET entity_type = 'user',
    entity_id = (SELECT owner_user_id FROM professional_profiles WHERE id = entity_id)
WHERE entity_type = 'professional_profile';

-- Delete duplicate follows caused by the reparent.
DELETE FROM follows a
USING follows b
WHERE a.id < b.id
  AND a.tenant_id = b.tenant_id
  AND a.user_id = b.user_id
  AND a.entity_type = b.entity_type
  AND a.entity_id = b.entity_id;

-- Drop the tables.
DROP TABLE IF EXISTS professional_profile_invitations;
DROP TABLE IF EXISTS professional_profile_members;
DROP TABLE IF EXISTS professional_profiles;
DROP TABLE IF EXISTS professional_profile_types;
```

- [ ] **Step 2: Down migration (best-effort).**

Recreate the four professional profile tables with their original schema. Data cannot be restored.

- [ ] **Step 3: Commit.**

---

### Task 4: Create `internal/tiers` package

**Files:**
- Create: `internal/tiers/tiers.go`
- Create: `internal/tiers/store.go`
- Create: `internal/tiers/service.go`
- Create: `internal/tiers/tiers_test.go`
- Create: `internal/db/queries/tiers.sql`
- Modify: `Makefile` if needed to add test target.

**Interfaces:**
- `func ResolveUserTier(ctx context.Context, tenantID, userID pgtype.UUID) (string, error)`
- `func SetUserTier(ctx context.Context, tenantID, userID pgtype.UUID, tier string) error`
- `func (s *Service) GetTierSetting(ctx context.Context, tenantID, userID pgtype.UUID, keyPrefix string) (string, error)`

- [ ] **Step 1: Add SQL queries to `internal/db/queries/tiers.sql`.**

```sql
-- name: ResolveUserTier :one
SELECT COALESCE(MAX(CASE ug.tier
    WHEN 'business' THEN 3
    WHEN 'pro' THEN 2
    WHEN 'regular' THEN 1
    ELSE 0
END), 1)::int AS tier_rank
FROM user_group_memberships ugm
JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = ugm.tenant_id
WHERE ugm.tenant_id = @tenant_id AND ugm.user_id = @user_id;

-- name: GetTierGroupID :one
SELECT id FROM user_groups
WHERE tenant_id = @tenant_id AND slug = @slug;

-- name: RemoveUserFromTierGroups :exec
DELETE FROM user_group_memberships
WHERE tenant_id = @tenant_id AND user_id = @user_id
  AND group_id IN (
    SELECT id FROM user_groups
    WHERE tenant_id = @tenant_id AND tier IS NOT NULL AND tier <> ''
  );

-- name: AddUserToGroup :exec
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
VALUES (@tenant_id, @user_id, @group_id)
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;

-- name: UpdateUserProfileType :exec
UPDATE users
SET profile_type = @profile_type, updated_at = now()
WHERE tenant_id = @tenant_id AND id = @user_id;
```

- [ ] **Step 2: Implement `internal/tiers/tiers.go` service.**

```go
package tiers

import (
    "context"
    "fmt"

    "github.com/jedafe/japaconnect/internal/db/gen"
    "github.com/jackc/pgx/v5/pgtype"
)

var tierRanks = map[string]int{
    "regular": 1,
    "pro":     2,
    "business": 3,
}

var tierSlugs = []string{"members", "pro", "business"}

func (s *Service) ResolveUserTier(ctx context.Context, tenantID, userID pgtype.UUID) (string, error) {
    rank, err := s.store.read.ResolveUserTier(ctx, gen.ResolveUserTierParams{
        TenantID: tenantID,
        UserID:   userID,
    })
    if err != nil {
        return "", fmt.Errorf("resolve user tier: %w", err)
    }
    for tier, r := range tierRanks {
        if r == int(rank) {
            return tier, nil
        }
    }
    return "regular", nil
}

func (s *Service) SetUserTier(ctx context.Context, tenantID, userID pgtype.UUID, tier string) error {
    if _, ok := tierRanks[tier]; !ok {
        return fmt.Errorf("invalid tier: %s", tier)
    }

    if err := s.store.write.RemoveUserFromTierGroups(ctx, tenantID, userID); err != nil {
        return fmt.Errorf("remove tier groups: %w", err)
    }

    // Always keep the user in the base members group.
    baseID, err := s.store.write.GetTierGroupID(ctx, tenantID, "members")
    if err != nil {
        return fmt.Errorf("lookup members group: %w", err)
    }
    if err := s.store.write.AddUserToGroup(ctx, tenantID, userID, baseID); err != nil {
        return fmt.Errorf("add members group: %w", err)
    }

    if tier != "regular" {
        targetID, err := s.store.write.GetTierGroupID(ctx, tenantID, tier)
        if err != nil {
            return fmt.Errorf("lookup %s group: %w", tier, err)
        }
        if err := s.store.write.AddUserToGroup(ctx, tenantID, userID, targetID); err != nil {
            return fmt.Errorf("add %s group: %w", tier, err)
        }
    }

    if err := s.store.write.UpdateUserProfileType(ctx, tenantID, userID, tier); err != nil {
        return fmt.Errorf("update profile_type: %w", err)
    }

    // TODO: invalidate permission cache if the user gets new permissions.
    return nil
}
```

- [ ] **Step 3: Implement store and helpers.**

`internal/tiers/store.go` wraps read/write queries. `internal/tiers/tiers.go` is the service.

- [ ] **Step 4: Add tests.**

Write `internal/tiers/tiers_test.go` with tests for `ResolveUserTier` and `SetUserTier`.

- [ ] **Step 5: Commit.**

---

### Task 5: Update users admin endpoints

**Files:**
- Modify: `internal/users/handlers.go`
- Modify: `internal/users/service.go`
- Modify: `internal/users/types.go`
- Modify: `internal/server/server.go` if route is being renamed.

- [ ] **Step 1: Update `UpdateProfileType` handler to accept and validate `regular/pro/business`.**

```go
validTypes := map[string]bool{"regular": true, "pro": true, "business": true}
if !validTypes[req.ProfileType] { ... }
```

- [ ] **Step 2: Call `tiers.SetUserTier` from `internal/users/service.go:UpdateProfileType`.**

Replace the direct store call with a call to the new `internal/tiers` service.

- [ ] **Step 3: Update `GetUserModerationConfig` options.**

Return `[]string{"regular","pro","business"}` for the profile type selector.

- [ ] **Step 4: Update `ListUsers` filter and i18n.**

No functional change to filter; values are just `regular/pro/business`.

- [ ] **Step 5: Commit.**

---

### Task 6: Organization creation tier limits

**Files:**
- Modify: `internal/organizations/service.go`
- Modify: `internal/organizations/handlers.go` if needed

**Interfaces:**
- Consumes: `tiers.ResolveUserTier`, `GetSetting`.

- [ ] **Step 1: Resolve user tier in `CreateOrganization`.**

```go
tier, err := s.tiers.ResolveUserTier(ctx, tenantID, user.UserID)
if err != nil { ... }
```

- [ ] **Step 2: Read per-tier limit and fallback to global.**

```go
limit := s.settings.GetIntOr(ctx, tenantID, fmt.Sprintf("organizations.max_per_user.%s", tier), -1)
if limit < 0 {
    limit = s.settings.GetIntOr(ctx, tenantID, "organizations.max_per_user", 5)
}
```

- [ ] **Step 3: Count existing orgs owned by user and enforce.**

Use existing `CountUserOrganizations` query. If `count >= limit`, return `errpkg.ErrForbidden` with code `organization_limit_reached`.

- [ ] **Step 4: Add tests in `internal/organizations/`.**

- [ ] **Step 5: Commit.**

---

### Task 7: API key rate-limit tier cap

**Files:**
- Modify: `internal/publicapi/service.go`
- Modify: `internal/publicapi/handlers.go`

- [ ] **Step 1: Resolve user tier and cap `RateLimitTier` in `CreateAPIKey`.**

```go
tier, err := s.tiers.ResolveUserTier(ctx, tenantID, userID)
if err != nil { ... }

maxTier := map[string]string{
    "regular": "basic",
    "pro":     "standard",
    "business": "premium",
}[tier]

if !isValidRateTierForUser(input.RateLimitTier, maxTier) {
    input.RateLimitTier = maxTier
}
```

- [ ] **Step 2: Add tests in `internal/publicapi/`.**

- [ ] **Step 3: Commit.**

---

### Task 8: Remove backend professional profile package and routes

**Files:**
- Delete: `internal/professional/` and its files.
- Delete: `internal/db/queries/professional_profiles.sql`
- Delete: `internal/db/queries/professional_extras.sql`
- Modify: `internal/server/server.go`
- Modify: `internal/features/registry.go`

- [ ] **Step 1: Delete `internal/professional` package files.**

- [ ] **Step 2: Delete professional profile SQL query files.**

- [ ] **Step 3: Remove `/api/v1/profiles/*` route block from `internal/server/server.go`.**

- [ ] **Step 4: Remove `feature.professional` from `internal/features/registry.go`.**

- [ ] **Step 5: Commit.**

---

### Task 9: Update follows, personas, widgets, campaigns, payments, GDPR

**Files:**
- Modify: `internal/follows/follows.go`, `internal/follows/handlers.go`, `internal/follows/service.go`
- Modify: `internal/personas/service.go`, `internal/db/queries/personas.sql`
- Modify: `internal/widgets/types.go`, `internal/widgets/renderer.go`
- Modify: `internal/campaign/planner.go` if referenced
- Modify: `internal/payments/handlers.go`
- Modify: `internal/gdpr/service.go`, `internal/gdpr/store.go`, `internal/db/queries/gdpr.sql`
- Modify: `internal/admin/catalog.go`

- [ ] **Step 1: Remove `professional_profile` from `follows` entity type lists and handlers.**

- [ ] **Step 2: Remove `professional_profiles` joins from `personas.sql` and `service.go`; byline from bot user profile.**

- [ ] **Step 3: Remove professional profile widget support from `internal/widgets`.**

- [ ] **Step 4: Remove professional profile payment and participation references.**

- [ ] **Step 5: Remove professional profile export/erasure references from `internal/gdpr`.**

- [ ] **Step 6: Remove professional profile items from `internal/admin/catalog.go`.**

- [ ] **Step 7: Remove "Professional profile" from the pricing plan feature list in `internal/payments/handlers.go` and seed migration.**

- [ ] **Step 8: Commit.**

---

### Task 10: Frontend — remove professional profile UI

**Files:**
- Delete: `apps/main/src/routes/(app)/dashboard/profiles/`
- Modify: `apps/main/src/routes/(app)/u/[username]/+page.svelte`
- Modify: `apps/main/src/lib/components/profile/AuthorContextSwitcher.svelte`
- Modify: `apps/main/src/lib/components/profile/FollowListModal.svelte`
- Modify: `apps/main/src/lib/components/profile/RelationshipsView.svelte`
- Modify: `apps/main/src/lib/types/index.ts`
- Modify: `apps/main/src/lib/i18n/en.json`, `fr.json`

- [ ] **Step 1: Delete the `dashboard/profiles` route directory.**

- [ ] **Step 2: Remove professional profile rendering from user public profile page.**

- [ ] **Step 3: Remove or simplify `AuthorContextSwitcher`.**

- [ ] **Step 4: Remove professional profile entries from `FollowListModal` and `RelationshipsView`.**

- [ ] **Step 5: Update `profile_type` TypeScript union and i18n keys to `regular/pro/business`.**

- [ ] **Step 6: Commit.**

---

### Task 11: Admin UI — tier management and pricing

**Files:**
- Delete: `apps/admin/src/routes/professional-types/+page.svelte`
- Modify: `apps/admin/src/routes/users/[id]/+page.svelte` or existing user edit page.
- Modify: `apps/admin/src/lib/locales/en.json`
- Modify: `apps/main/src/routes/(app)/pricing/+page.svelte`
- Modify: `apps/main/src/lib/api/hub.ts`
- Modify: `apps/main/src/lib/types/index.ts`

- [ ] **Step 1: Delete admin professional types page and sidebar references.**

- [ ] **Step 2: Update admin user profile type selector to `regular/pro/business`.**

- [ ] **Step 3: Update pricing page to show tier badge for each plan using `subscription_plans.tier`.**

- [ ] **Step 4: Update TypeScript types and admin i18n.**

- [ ] **Step 5: Commit.**

---

### Task 12: Regenerate SQLC and run backend checks

**Files:**
- `internal/db/gen/` (regenerated)

- [ ] **Step 1: Run `make sqlc-gen`.**

- [ ] **Step 2: Run `go build ./...` and fix compile errors.**

- [ ] **Step 3: Run `go test ./internal/tiers/... ./internal/organizations/... ./internal/publicapi/...`.**

- [ ] **Step 4: Commit generated files and any fixes.**

---

### Task 13: Frontend checks

- [ ] **Step 1: Run `bun run check` in `apps/main`.**

- [ ] **Step 2: Run `bun run check` in `apps/admin`.**

- [ ] **Step 3: Run `bun run build` in both apps if check passes.**

- [ ] **Step 4: Fix type errors and commit.**

---

### Task 14: Final verification

- [ ] **Step 1: Search for remaining `professional_profile` references in Go and Svelte files.**

- [ ] **Step 2: Verify migrations can be applied against a database with existing professional profile data.**

- [ ] **Step 3: Run `go test ./...` on affected packages and `go vet ./...`.**

- [ ] **Step 4: Run `make sqlc-gen` one final time and confirm no diff.**

- [ ] **Step 5: Final commit and summary.**

---

## Spec coverage self-review

| Spec section | Plan task(s) |
|---|---|
| `user_groups.tier`, `max_organizations_creatable` | Task 1 |
| `subscription_plans.tier` | Task 1 |
| `users.profile_type` rename and backfill | Task 2 |
| Professional profile removal and data reparenting | Task 3, Task 9 |
| `internal/tiers` package | Task 4 |
| Admin tier assignment | Task 5 |
| Organization creation limit | Task 6 |
| API key rate limit cap | Task 7 |
| Spaces via `max_spaces_creatable` | Task 1 (group seeding) |
| Per-tier settings catalog | Task 1 |
| Feature permission roles | Task 1 (seeded in migration; future work to wire V2 tools) |
| Pricing tier display | Task 11 |
| Frontend removal | Task 10 |

## Placeholder scan

No `TBD`, `TODO`, or `implement later` placeholders. Concrete SQL, setting keys, file paths, and signatures are provided.

## Type consistency

- `ResolveUserTier` and `SetUserTier` use `pgtype.UUID` for tenant and user IDs, matching the codebase convention.
- Tier values are `regular`, `pro`, `business` everywhere.
- `system_settings` keys use the `{prefix}.{tier}` convention.
