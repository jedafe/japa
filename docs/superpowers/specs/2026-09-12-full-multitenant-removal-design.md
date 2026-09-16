# Full multi-tenancy removal — design

## Status
**COMPLETED** — implemented and pushed to `main` on 2026-09-14. This document is retained as the historical design record; the authoritative source of truth is now `migrations/000668_final_de_tenant` and the single-tenant code in the main repo.

## Context
JapaConnect launched with `tenant_id` baked into almost every scoped table, but in practice has only ever operated as a single tenant. `SingleTenantMiddleware` already resolves the default tenant once at boot and injects it into every request, so the active runtime cost of the scaffolding is negligible. This design covers the deliberate removal of all remaining multi-tenant scaffolding from the database, Go backend, SvelteKit frontends, workers, Valkey cache keys, and project documentation.

## Goal
Remove the multi-tenant concept entirely:

- Delete the `tenants` table and all `tenant_id` columns.
- Remove `tenant_id` from every sqlc query and generated Go file.
- Remove the tenant middleware, context helpers, and all handler/service references.
- Convert `system_settings`, `feature_flags`, and other tenant-scoped subsystems to global scope.
- Remove tenant fields from frontend types, API clients, and the Control Panel tenant UI.
- Update all tenant-scoped Valkey cache key patterns to be global.
- Update JEOS specs, ADR-004, and convention catalogs to reflect single-tenant operation.

## Non-goals
- **Organizations are not tenants.** The `organizations` table, `contents.organization_id`, and public org profiles remain untouched.
- Geo/space personalization is not replaced by tenancy.
- No new product features are added.
- No data from any other tenant is migrated or merged; the program assumes the platform has only ever held the default tenant (`00000000-0000-0000-0000-000000000001`).

## Core assumption
Before the final schema migration, a data audit must confirm that every scoped row belongs to the default tenant. If any non-default tenant rows exist, a separate merge/purge decision is required before this program can proceed.

## Guiding principles

1. **Generated files are regenerated, never hand-edited.** All files in `internal/db/gen` come from `make sqlc-gen`. Changes start in `internal/db/queries/*.sql` and the migration files.
2. **Column drops are the last step.** Constraint rewrites are done in Phase 1.0 so queries can become fully single-tenant, but the actual `tenant_id` columns and the `tenants` table are not dropped until the rest of the code is de-tenanted.
3. **Per-layer verification.** Each phase ends with build, unit tests, `gosec`, and the relevant app checks before the next phase begins.
4. **No Valkey key pattern without a catalog entry.** New global key patterns are added to `.jeos/conventions/valkey-keys-catalog.md` before use.
5. **No migration without a down migration.** Every schema migration is paired so the change is reversible during development.

## Phases

### Phase 1 — DB access layer and tenant-scoped stores
**Goal:** Remove `tenant_id` from the active query layer, first by making the database column optional, then by stripping `tenant_id` from every sqlc query and generated query method. Also convert `system_settings` and `feature_flags` to global scope.

**What changes:**

- **Preparation migration (Phase 1.0)**
  - Precondition: the data audit has confirmed every row already belongs to the default tenant.
  - For every scoped table that has `tenant_id uuid NOT NULL REFERENCES tenants(id)`, run the equivalent of:
    ```sql
    ALTER TABLE contents ALTER COLUMN tenant_id SET DEFAULT '00000000-0000-0000-0000-000000000001';
    ```
    This is repeated for every scoped table (e.g. `users`, `comments`, `reactions`, `feature_flags`, `system_settings`, and all others listed in the database catalog).
  - Rewrite every primary key, unique constraint, and foreign key that includes `tenant_id` so it no longer includes that column. The column itself is kept, but it is no longer part of any key or constraint. For example:
    - `PRIMARY KEY (tenant_id, key)` → `PRIMARY KEY (key)`
    - `UNIQUE(tenant_id, slug)` → `UNIQUE(slug)`
    - `FOREIGN KEY (tenant_id, reaction_key) REFERENCES reaction_types(tenant_id, key)` → `FOREIGN KEY (reaction_key) REFERENCES reaction_types(key)`
    - `PRIMARY KEY (tenant_id, user_id)` → `PRIMARY KEY (user_id)`
  - Create replacement indexes where needed so the new PK/unique constraints are efficient and queries that omit `tenant_id` still perform well.
  - A single consolidated migration or one per subsystem may be used, as long as each is paired with a down migration that restores the original constraints and drops the defaults.
- `internal/db/queries/*.sql` (96 files)
  - Remove `tenant_id` from `WHERE`, `JOIN`, `ON CONFLICT`, `GROUP BY`, `ORDER BY`, and `RETURNING` clauses.
  - Remove `tenant_id` from query parameters and result columns.
  - Rewrite unique-constraint-aware operations (e.g. `ON CONFLICT (tenant_id, key)` → `ON CONFLICT (key)`).
- `internal/db/gen/*.sql.go` (99 files) and `internal/db/gen/models.go`
  - Regenerated via `make sqlc-gen`.
  - Query methods no longer accept or filter by `tenant_id`; however, `ID` model fields still exist in this phase because the underlying schema still contains `tenant_id` columns. Code in Phase 2 must ignore these fields.
  - Internally, `internal/config/settings.go` stops using `ID` for queries and cache keys (cache key becomes `jc:settings:{key}`). The public `GetSetting`, `GetSettingBool`, etc. signatures keep the `ID` parameter in Phase 1 but ignore it, so the rest of the codebase still compiles. The parameter is removed in Phase 2.
- `internal/features/features.go`
  - Remove `tenant_id` from the `feature_flags` query.
  - Change the Valkey cache key from `jc:flags:{tenant_id}:{flag_key}` to `jc:flags:{flag_key}`.
  - The public `FeatureChecker` and `Checker.IsEnabled` signatures keep the `ID` parameter in Phase 1 but ignore it; the parameter is removed in Phase 2.
  - Rollout hashing can continue to use the default tenant constant in Phase 1; the final global hashing decision is made in Phase 2.
- `internal/db/queries/settings.sql` and `internal/db/queries/features.sql`
  - Remove tenant-scoped parameters and filters.

**Exit criteria:**

- The Phase 1.0 preparation migration has been applied and every `tenant_id` column has the default tenant value.
- `make sqlc-gen` succeeds.
- `go build ./...` passes for packages that compile against the new generated code.
- `system_settings` and `feature_flags` unit tests pass.
- No `tenant_id` remains in `internal/db/queries` or in sqlc query methods.
- `ID` fields may still exist in `internal/db/gen/models.go`; they are treated as defunct until Phase 4.

### Phase 2 — Go backend runtime
**Goal:** Remove tenant context propagation and all non-generated Go code references.

**What changes:**

- `internal/tenant/tenant.go`, `internal/tenant/store.go`, `internal/tenant/tenant_test.go`
  - Delete the package.
  - Remove `tenant.SingleTenantMiddleware` from `internal/server/server.go`.
  - Remove `tenant.FromContext` and `tenant.IDFromContext` usage.
- `internal/server/server.go`
  - Remove `defaultTenant` resolution and middleware injection.
  - Remove tenant from bot detection middleware initialization, if any.
- Handlers and services (135+ files with `requireTenant` and 12 files with `tenant.IDFromContext`)
  - Remove `requireTenant` helper and `ID` local variables.
  - Update call sites to settings/feature-checker/other services without `ID`.
  - Remove `tenant_id` from JSON response DTOs where it is exposed.
- `internal/worker/handlers.go` and other worker files
  - Remove tenant iteration in RSS, sitemap, LLMs, spam, feed, and maintenance tasks.
  - Remove `tenant_id` from worker payloads and cache-key builders.
- Valkey key builders
  - Update all `jc:*:{tenant_id}` patterns to global variants.
  - Update `.jeos/conventions/valkey-keys-catalog.md`.
- `internal/publicapi/service.go`, `internal/organizations/service.go`, `internal/university/service.go`, etc.
  - Remove tenant-scoped cache keys and query parameters.
- `cmd/*` (24 references)
  - Remove `tenant_id` from CLI scripts and backfill tools.

**Exit criteria:**

- `go build ./...` passes.
- `go test ./...` passes.
- `gosec` reports no new HIGH/CRITICAL findings.
- No `tenant.FromContext`, `tenant.IDFromContext`, or `requireTenant` remains.

### Phase 3 — Frontend apps
**Goal:** Remove tenant from the SvelteKit surfaces and API clients.

**What changes:**

- `apps/control/src/routes/tenants/+page.svelte` and `TenantSettingsDialog.svelte`
  - Delete these components and their test files.
  - Remove `tenants` route from navigation and constants.
- `apps/control/src/lib/api/types.ts`, `apps/main/src/lib/types/index.ts`, etc.
  - Remove `tenant_id` from every TypeScript interface and DTO.
- `apps/main/src/lib/server/api.ts`, `apps/learn/src/lib/server/api.ts`, `apps/tools/src/lib/server/api.ts`, `apps/docs/src/lib/server/api.ts`
  - Remove tenant header forwarding (e.g. `X-Forwarded-Host` for tenant resolution can remain for reverse-proxy correctness, but no tenant ID is passed).
- `apps/control/src/lib/locales/en.json`, `fr.json`, and other i18n files
  - Remove tenant-related locale keys.
- `apps/studio-desktop`
  - Remove `tenant_id` references in `src/lib/services/db.ts` and `src/lib/services/settings.ts`.
- `apps/*/src/app.d.ts` and `hooks.server.ts`
  - Remove tenant fields from app-level types and hook logic.
- `packages/*` (26 references)
  - Update shared packages that reference tenant.

**Exit criteria:**

- `bun run check` and `bun run build` pass in `apps/admin`, `apps/control`, `apps/docs`, `apps/learn`, `apps/main`, `apps/tools`, and `apps/studio-desktop`.
- Playwright smoke tests for the admin and control surfaces pass.

### Phase 4 — Database schema migration
**Goal:** Physically remove every `tenant_id` column and the `tenants` table. By this point, all primary keys, unique constraints, and foreign keys have already been rewritten in Phase 1.0, so this phase only drops the now-unused column and its table.

**What changes (one migration per subsystem, paired with down migrations):**

- `system_settings`
  - Drop `tenant_id` column.
- `feature_flags`
  - Drop `tenant_id` column and the `idx_feature_flags_tenant` index.
- `reaction_types`
  - Drop `tenant_id` and `idx_reaction_types_tenant`.
- `reactions`
  - Drop `tenant_id` and any index that was only needed because `tenant_id` was in the leading position (e.g. `idx_reactions_entity`, `idx_reactions_user`).
- `contents`
  - Drop `tenant_id` and `idx_contents_tenant_id`.
- `users`
  - Drop `tenant_id` and any index that was only needed because it led with `tenant_id`.
- `user_dm_settings` and `user_privacy_settings`
  - Drop `tenant_id`.
- `affiliate_program_settings`
  - Drop `tenant_id` and add `id uuid PRIMARY KEY DEFAULT gen_random_uuid()` if it does not already exist.
- Every other scoped table
  - Drop `tenant_id uuid` and any index that was only needed because it led with `tenant_id` (e.g. `idx_*_tenant_id`).
- `tenants`
  - Drop table after all FKs referencing it are removed.

**Preconditions:**

- Phase 1 and Phase 2 are complete and deployed.
- Data audit confirms one default tenant and no cross-tenant rows.
- Production backup taken.

**Exit criteria:**

- `make migrate-up` and `make migrate-down` both run cleanly in a fresh environment.
- `make sqlc-gen` still succeeds after the migration changes.
- `go test ./...` passes against the new schema.
- `pg_dump --schema-only` shows no `tenant_id` columns or `tenants` table.

### Phase 5 — Documentation and spec cleanup
**Goal:** Update the source of truth documents and remove dead references.

**What changes:**

- `.jeos/sections/04-multi-tenancy.md`
  - Replace with a note that the platform is single-tenant and the section is deprecated/retired.
- `.jeos/adr/ADR-004-multi-tenancy-from-day-one.md`
  - Add a final amendment documenting the removal and the rationale.
- `.jeos/conventions/valkey-keys-catalog.md`
  - Remove or rewrite all `{tenant_id}` and `{tenant}` placeholders.
- `.jeos/conventions/system-settings-catalog.md`, `api-routes-catalog.md`, `asynq-tasks.md`
  - Remove tenant-specific references.
- `migrations/schemas/MIGRATION-ORDER.md`
  - Update if tenant migrations are removed/renamed.
- Seed scripts and fixtures
  - Remove default-tenant seeding and `tenant_id` from seed data.
- `README` / onboarding docs
  - Remove multi-tenant setup instructions.

**Exit criteria:**

- `grep -ri 'tenant' .jeos/ docs/` returns only historical references or intentional exceptions.
- `make docs` or equivalent passes.

## Detailed technical notes

### Settings and feature flags
These are the two most widely used tenant-scoped subsystems and are the highest priority in Phase 1. Internally, the DB queries and cache keys become global, but the public Go API signatures keep the `ID` parameter in Phase 1 (it is ignored) so the rest of the codebase continues to compile. Phase 2 removes the parameters entirely.

- `Settings.GetSetting(ctx, ID, key)` still accepts `ID` but ignores it; internally it queries and caches by `key` only.
- `Settings.InvalidateSettings(ctx, ID)` still accepts `ID` but ignores it; it flushes all settings.
- `feature.Checker.IsEnabled(ctx, ID, key)` still accepts `ID` but ignores it; the `feature_flags` query uses `key` only and the cache key is `jc:flags:{flag_key}`.

Because both use Valkey, the cache key change must be made at the same time as the code change to avoid stale tenant-scoped cache entries.

### Cache key patterns
The Valkey catalog contains many tenant-scoped patterns. Examples that become global:

- `jc:settings:{tenant_id}:{key}` → `jc:settings:{key}`
- `jc:flags:{tenant_id}:{flag_key}` → `jc:flags:{flag_key}`
- `jc:cache:feed:foryou:{tenant_id}:{user_id}:...` → `jc:cache:feed:foryou:{user_id}:...`
- `jc:cache:feed:rss:{tenant_id}` → `jc:cache:feed:rss`
- `jc:llms_txt:{tenant_id}` → `jc:llms_txt`
- `jc:wallet:currencies:{tenant_id}` → `jc:wallet:currencies`
- `jc:org:profile:{tenant_id}:{slug}` → `jc:org:profile:{slug}`

A full scan of `valkey-keys-catalog.md` is required; no pattern may be changed without updating the catalog first.

### Workers
Several workers iterate over tenants or use tenant IDs in payloads:

- `rss:regenerate` (`internal/worker/handlers.go`)
- `sitemap:generate` (`internal/seo/sitemap.go`)
- `maintenance:regenerate_llms_txt`
- `maintenance:compute_trending`
- `maintenance:compute_suggestions`

These must be flattened to a single-tenant model. Any payload field named `tenant_id` is removed; any per-tenant cache key is converted to a global key.

### Unique constraints and indexes
Every `UNIQUE(tenant_id, ...)` and `PRIMARY KEY (tenant_id, ...)` must be rewritten. Some tables have composite foreign keys such as `reactions.tenant_id + reaction_key → reaction_types.tenant_id + key`; these become simple single-column FKs. Before the migration, a data-integrity query must confirm no duplicate keys would violate the new constraints (e.g. two tenants having the same `reaction_types.key`).

### Slug and content hash
`contents.slug` and `contents.content_hash` are currently unique per tenant. They must become globally unique. This is safe only if the data audit confirms no slug or content-hash collisions across tenants.

### Users and auth
`users.tenant_id` is part of the auth context. The `users` table may have `UNIQUE(tenant_id, email)` or similar. Removing the tenant scope makes `email` and `username` globally unique, which should already be true in a single-tenant deployment. If not, duplicates must be reconciled before the migration.

### Public API
`internal/publicapi/service.go` uses `tenant_id` to scope external API calls. The service signature changes to no longer accept a tenant ID, and the underlying queries return global data.

## Data migration and zero-downtime

Because this is a column-drop and constraint-rewrite program, not a rewrite of live traffic behavior, the zero-downtime path is:

1. Deploy Phase 1 and Phase 2 code while `tenant_id` columns still exist (code no longer reads/writes them).
2. Run a data audit query to confirm single-tenant integrity.
3. Schedule a maintenance window for Phase 4 migrations, or run them as a single transaction if table sizes allow.
4. After migration, run the full backend test suite and smoke tests.
5. Flush all Valkey keys that contained the old tenant segment to avoid stale cache.

## Testing strategy

- **Unit tests:** `go test ./...` after each phase.
- **Security scan:** `gosec ./...` after each Go change.
- **Lint:** `golangci-lint run`.
- **Frontend checks:** `bun run check` and `bun run build` for all six SvelteKit apps.
- **Integration:** Run the backend audit scripts and Playwright smoke tests.
- **Migration checks:** `make migrate-up` and `make migrate-down` on a fresh database; `pg_dump --schema-only` inspection.
- **Data integrity:** Custom SQL queries to confirm no cross-tenant rows and no duplicate keys before constraint rewrites.

## Risks and mitigation

| Risk | Mitigation |
|---|---|
| Missed `tenant_id` filter leaves a cross-tenant query | Comprehensive `grep` audit at end of each phase; integration tests on all 475 routes. |
| Duplicate-key migration failure | Pre-migration data audit; script to detect conflicts in `UNIQUE(tenant_id, ...)` columns. |
| Stale Valkey cache under old tenant-scoped keys | Flush all `jc:*` keys after key-pattern changes; verify cache catalog. |
| Feature flag rollout hashing changes behavior | Decision made in Phase 1 design: either remove per-tenant hashing or replace with a global salt; document in spec. |
| Frontend DTOs out of sync with backend | Regenerate TypeScript types from OpenAPI spec after API DTOs change. |
| Large migration transaction locks tables | Run migrations in subsystem-level migrations; use `CONCURRENTLY` for index re-creation where possible; plan maintenance window. |
| JEOS / ADR drift | Phase 5 is dedicated to doc updates; every convention catalog is reviewed. |

## Timeline

This is a program, not a single feature. A realistic estimate with one senior backend engineer and one senior frontend engineer:

- **Phase 1:** 1.5–2 weeks
- **Phase 2:** 2.5–3.5 weeks
- **Phase 3:** 1–2 weeks
- **Phase 4:** 1–1.5 weeks
- **Phase 5:** 0.5–1 week
- **QA, integration, and buffer:** 1–2 weeks

Total: **6–10+ weeks**, depending on how tightly coupled the Phase 1/2 query and caller changes prove to be.

## References

- `/.jeos/adr/ADR-004-multi-tenancy-from-day-one.md` — single-tenant freeze and original removal cost estimate.
- `/.jeos/sections/04-multi-tenancy.md` — multi-tenancy section.
- `internal/tenant/tenant.go` — single-tenant middleware.
- `internal/server/server.go` lines 2428-2431 — middleware wiring.
- `internal/config/settings.go` lines 26-37 and 203-218 — per-tenant settings API.
- `internal/features/features.go` lines 28-37 and 92-100 — per-tenant feature flags.
- `/.jeos/conventions/valkey-keys-catalog.md` — tenant-scoped key patterns.

## 2026-09-15 cleanup sweep (post-schema)

A final terminology and dead-code cleanup pass removed the remaining active
multi-tenant references from the runtime code:

- Deleted the dead Control Panel `/tenants` page, `TenantSettingsDialog`, and
  `Tenant` type.
- Removed `tenant_id` / `tenantId` from frontend types, shared package models,
  and Studio Desktop's local Tauri database.
- Renamed `tenantHost` to `requestHost` in SvelteKit `app.d.ts`, hooks, and API
  clients, while keeping actual `Host` / `X-Forwarded-Host` forwarding.
- Replaced `tenant_unavailable` with `service_unavailable` and reworded JCE
  messages from tenant-specific to platform-specific language.
- Removed dead backend `tenant` settings handlers and `t == nil` no-op tenant
  checks from handlers and middleware.
- Renamed `ListTenantAdminIDs` to `ListSystemAdminIDs`, `ClearCacheForTenant`
  to global `ClearCache`, and `UserBelongsToTenant` to `UserExists`.
- Removed all `requireTenant` / `requireTenantAndUser` / `requireTenantFromContext`
  stubs from internal packages.
- Renamed all sqlc query names that contained `ByTenant`, `ForTenant`, or
  `AndTenant` to platform/global equivalents (e.g. `ListSpacesByTenant` →
  `ListSpaces`, `GetMenuBySlugForTenant` → `GetActiveMenuBySlug`) and updated
  every Go call site.
- Reworded `tenant`/`tenants` to `platform`/`platforms` in backend Go source
  files, SQL query comments, and generated `internal/db/gen` comments.
- Rewrote the obsolete `scripts/patch_returning_select.py`,
  `scripts/fix_gdpr_positional.py`, and `scripts/audit_single_tenant.sql` as
  no-ops (historical).
- Updated `scripts/generate_tool_crud_migration.py` so generated `ai_tools`
  seed migrations no longer reference `tenant_id`.
- Verified `make sqlc-gen`, `go build ./...`, `go test ./...`, `gosec` on the
  changed packages, and `go vet` on the changed packages.

A small number of references intentionally remain:
- `migrations/` files contain historical `tenant_id` columns, seed data, and
  permission node names required to preserve migration correctness.
- `docs/` and `.jeos/` design documents describing the original multi-tenant
  architecture are retained as historical records.
- `apps/*/src/lib/i18n/fr.json` contains the French word `maintenant`, which is
  a false positive and is not a tenant reference.

## 2026-09-15 follow-up: clean up full-suite lint

- Removed no-op `append(args)` / `append(tArgs)` / `append(eArgs)` /
  `append(sArgs)` calls in `internal/digest/store.go` that caused
  `go vet ./...` warnings.
- Corrected the argument index (`argIdx := 2` / `argIdx = 2` → `1`) in the
  same digest queries so space ID placeholders now start at `$1` instead of
  `$2`.
- Added `internal/db/gen/.*\.go` G101 exclusions to `.gosec.json` so the
  generated SQL query string constants are no longer flagged as hardcoded
  credentials. Run `gosec -conf .gosec.json ./...` to use the project config.
  `go vet ./...` and `gosec -conf .gosec.json ./...` now both pass.
