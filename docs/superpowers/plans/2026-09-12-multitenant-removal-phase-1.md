# Multi-tenancy removal — Phase 1.0/1.1 implementation plan

> **Status: COMPLETED** — folded into the final multi-tenancy removal. All phases were completed in one pass; `tenant_id` columns, the `tenants` table, and tenant-aware APIs were removed by migration `000668_final_de_tenant`. This plan is retained for historical context.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the single-tenant foundation: audit the data, apply a preparation migration that makes `tenant_id` optional and rewrites all constraints, and de-tenant the `system_settings` and `feature_flags` subsystems. After this phase the backend still has `tenant_id` columns, but the two most-used tenant-scoped subsystems no longer reference them.

**Architecture:** The program is executed in layers. Phase 1.0 is the database foundation. Phase 1.1 is the settings and feature-flag subsystem. The remaining 94 sqlc query files are de-tenanted in per-subsystem child plans because each has different parameter patterns and must be verified independently.

**Tech Stack:** Go 1.23, pgx/v5, sqlc 1.31, PostgreSQL 18, golang-migrate, Valkey.

---

## Global constraints

- All commits go to the `main` branch.
- Follow `/.jeos/AGENTS.md`: never hand-edit `internal/db/gen`; edit `internal/db/queries/*.sql` and migrations, then run `make sqlc-gen`.
- Every new migration has a paired `.down.sql`.
- No new Valkey key pattern is used without updating `.jeos/conventions/valkey-keys-catalog.md` first.
- No AI attribution in commits, PRs, or documentation.
- The `Settings` and `FeatureChecker` public Go signatures keep their `ID` parameter in this phase (it is ignored). The parameter is removed in Phase 2 so the bulk of the codebase still compiles.

---

## Task 1: Data audit

**Files:**
- Create: `scripts/audit_single_tenant.sql`

**Interfaces:**
- Produces a SQL script and a console report confirming every scoped row belongs to the default tenant.

- [ ] **Step 1: Write the audit script**

```sql
-- scripts/audit_single_tenant.sql
-- Reports how many distinct tenant_ids exist per table and what they are.
DO $$
DECLARE
    rec record;
    sql text;
BEGIN
    FOR rec IN
        SELECT table_schema, table_name
        FROM information_schema.columns
        WHERE column_name = 'tenant_id'
          AND table_schema = 'public'
        ORDER BY table_name
    LOOP
        sql := format('SELECT %L AS table_name, tenant_id, count(*) AS rows FROM %I.%I GROUP BY tenant_id',
                      rec.table_name, rec.table_schema, rec.table_name);
        RAISE NOTICE '%', sql;
        EXECUTE sql;
    END LOOP;
END $$;
```

- [ ] **Step 2: Run the audit**

```bash
psql "$DATABASE_URL" -f scripts/audit_single_tenant.sql
```

If `DATABASE_URL` is not set, use the connection string from the local `.env`.

- [ ] **Step 3: Verify the result**

Expected output:
- Every scoped table has exactly one `tenant_id` group.
- The only `tenant_id` is `00000000-0000-0000-0000-000000000001`.

If any table has a different `tenant_id`, stop and decide on a merge/purge strategy before continuing.

- [ ] **Step 4: Commit**

```bash
git add scripts/audit_single_tenant.sql
git commit -m "chore: add single-tenant data audit script"
```

---

## Task 2: Phase 1.0 preparation migration

**Files:**
- Create: `migrations/000665_tenant_preparation.up.sql`
- Create: `migrations/000665_tenant_preparation.down.sql`

**Interfaces:**
- Consumes: audit result from Task 1.
- Produces: a reversible migration that sets the default `tenant_id` and rewrites all PK/FK/unique constraints.

- [ ] **Step 1: Generate the migration SQL**

Write a generator in Python, Go, or SQL that queries `information_schema` and produces the two files. The generator must produce statements for every scoped table. For example, for `contents`:

```sql
ALTER TABLE contents ALTER COLUMN tenant_id SET DEFAULT '00000000-0000-0000-0000-000000000001';
```

Repeat that statement for every other table with a `tenant_id` column.

For every primary key, unique, or foreign key that includes `tenant_id`, produce the equivalent of:

```sql
-- PRIMARY KEY (tenant_id, key) -> PRIMARY KEY (key)
ALTER TABLE feature_flags DROP CONSTRAINT feature_flags_pkey, ADD PRIMARY KEY (key);

-- UNIQUE(tenant_id, slug) -> UNIQUE(slug)
DROP INDEX IF EXISTS idx_contents_tenant_slug;
CREATE UNIQUE INDEX idx_contents_slug ON contents(slug);

-- FOREIGN KEY (tenant_id, reaction_key) REFERENCES reaction_types(tenant_id, key)
--    -> FOREIGN KEY (reaction_key) REFERENCES reaction_types(key)
ALTER TABLE reactions
DROP CONSTRAINT reactions_reaction_key_fkey,
ADD CONSTRAINT reactions_reaction_key_fkey
FOREIGN KEY (reaction_key) REFERENCES reaction_types(key) ON DELETE CASCADE;

-- PRIMARY KEY (tenant_id, user_id) -> PRIMARY KEY (user_id)
ALTER TABLE user_dm_settings DROP CONSTRAINT user_dm_settings_pkey, ADD PRIMARY KEY (user_id);
```

Special case for `affiliate_program_settings` (tenant_id is its whole PK):

```sql
ALTER TABLE affiliate_program_settings DROP CONSTRAINT affiliate_program_settings_pkey;
ALTER TABLE affiliate_program_settings ADD COLUMN id uuid PRIMARY KEY DEFAULT gen_random_uuid();
```

The generator must be run against a database that has all migrations applied up to 000664. Use the result as the starting point, then review by hand.

- [ ] **Step 2: Review the generated migration manually**

Open `migrations/000665_tenant_preparation.up.sql` and verify:
- Every `tenant_id` column has a default.
- Every `UNIQUE(tenant_id, ...)` and `PRIMARY KEY (tenant_id, ...)` is rewritten without `tenant_id`.
- The `reactions` composite foreign key is handled.
- The `affiliate_program_settings` special case is included.
- No constraint or index remains that requires `tenant_id` for uniqueness or lookup.

- [ ] **Step 3: Write the down migration**

The down migration must reverse every statement in the up file, restoring the original constraints and dropping the defaults. Pair each up statement with its exact reverse.

- [ ] **Step 4: Run the migration locally**

```bash
make migrate
```

- [ ] **Step 5: Verify the schema state**

```bash
pg_dump --schema-only "$DATABASE_URL" | grep -i tenant
```

Expected:
- `tenant_id` columns still exist.
- No PK, unique, or FK references `tenant_id`.
- `tenants` table still exists.

- [ ] **Step 6: Commit**

```bash
git add migrations/000665_tenant_preparation.*
git commit -m "chore: add Phase 1.0 tenant constraint and default rewrite"
```

---

## Task 3: De-tenant `system_settings`

**Files:**
- Modify: `internal/db/queries/settings.sql`
- Modify: `internal/config/settings.go`
- Regenerate: `internal/db/gen/settings.sql.go`

**Interfaces:**
- Consumes: `gen.GetSettingValue`, `gen.GetPublicSettings`, `gen.ListSystemSettings`, `gen.UpsertSystemSetting`, `gen.UpsertSystemSettingPreservePublic`, `gen.DeleteSystemSetting` (after regeneration, these have no `ID` fields).
- Produces: global settings lookups. The public `Settings` methods still accept `ID` but ignore it; the cache key is `jc:settings:{key}`.

- [ ] **Step 1: Edit `internal/db/queries/settings.sql`**

Replace the entire file with:

```sql
-- System settings queries (global scope after multi-tenancy removal).

-- name: GetSettingValue :one
SELECT value FROM system_settings WHERE key = $1;

-- name: GetPublicSettings :many
SELECT key, value FROM system_settings WHERE is_public = true ORDER BY key;

-- name: ListSystemSettings :many
SELECT key, value, is_public, updated_at FROM system_settings ORDER BY key;

-- name: UpsertSystemSetting :exec
INSERT INTO system_settings (key, value, is_public, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (key) DO UPDATE SET value = $2, is_public = $3, updated_at = now();

-- name: UpsertSystemSettingPreservePublic :exec
INSERT INTO system_settings (key, value, is_public, created_at, updated_at)
VALUES ($1, $2, false, now(), now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now();

-- name: DeleteSystemSetting :exec
DELETE FROM system_settings WHERE key = $1;
```

- [ ] **Step 2: Update `internal/config/settings.go`**

1. Remove `ID` from the internal cache key and the Valkey key. The cache map and `settingsCachePrefix` become global.
2. Update `GetSetting`, `GetSettingBool`, `GetSettingInt`, `GetStringOr`, `GetIntOr`, `GetFloatOr`, `GetBoolOr` to compute `cacheKey := key` instead of `ID.String() + ":" + key`.
3. Update `loadFromDB`, `queryAndCache`, `SetSetting`, `UpdateSettingsBatch`, `UpdateSettingsBatchWithPublic`, `GetPublicSettings`, and `InvalidateSettings` to use only `key`.
4. `InvalidateSettings` must clear the in-memory map and all `jc:settings:*` Valkey keys.
5. `GetSettingValue` is called with `key` only: `s.q.GetSettingValue(ctx, key)`.
6. `UpsertSystemSetting` and `UpsertSystemSettingPreservePublic` are called with `gen.UpsertSystemSettingParams{Key: key, Value: value, IsPublic: isPublic}`.

The public method signatures remain unchanged for this phase:

```go
func (s *Settings) GetSetting(ctx context.Context, ID pgtype.UUID, key string) (string, error)
func (s *Settings) InvalidateSettings(ctx context.Context, ID pgtype.UUID)
```

The `ID` argument is not used inside the method.

- [ ] **Step 3: Regenerate sqlc**

```bash
make sqlc-gen
```

- [ ] **Step 4: Verify build and tests**

```bash
go build ./internal/config/...
go test ./internal/config/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/settings.sql internal/config/settings.go internal/db/gen/settings.sql.go
git commit -m "refactor: make system_settings global in Phase 1.1"
```

---

## Task 4: De-tenant `feature_flags`

**Files:**
- Modify: `internal/db/queries/features.sql`
- Modify: `internal/features/features.go`
- Regenerate: `internal/db/gen/features.sql.go`

**Interfaces:**
- Consumes: new generated `GetFeatureFlag`, `ListFeatureFlags`, `UpsertFeatureFlagEnabled`, `UpsertFeatureFlagWithConfig`, `DeleteFeatureFlag` (no `ID` fields).
- Produces: `Checker.IsEnabled`, `GetAllEnabled`, `SetEnabled`, `UpdateFlag`, `DeleteFlag`, `ListFlagRows`, `GetFlagConfig`, and `SetRolloutPercentage` still accept `ID` but ignore it; the Valkey cache key is `jc:flags:{flag_key}`.

- [ ] **Step 1: Edit `internal/db/queries/features.sql`**

Replace the entire file with:

```sql
-- features.sql — Feature Flags queries (global scope after multi-tenancy removal).

-- name: GetFeatureFlag :one
SELECT key, enabled, config, updated_at, updated_by
FROM feature_flags
WHERE key = $1;

-- name: ListFeatureFlags :many
SELECT key, enabled, config, updated_at, updated_by
FROM feature_flags
ORDER BY key;

-- name: UpsertFeatureFlagEnabled :exec
INSERT INTO feature_flags (key, enabled, updated_by)
VALUES ($1, $2, $3)
ON CONFLICT (key)
DO UPDATE SET enabled = $2, updated_by = $3, updated_at = now();

-- name: UpsertFeatureFlagWithConfig :exec
INSERT INTO feature_flags (key, enabled, config, updated_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key)
DO UPDATE SET enabled = $2, config = $3, updated_by = $4, updated_at = now();

-- name: DeleteFeatureFlag :exec
DELETE FROM feature_flags
WHERE key = $1;
```

- [ ] **Step 2: Update `internal/features/features.go`**

For each function, rewrite the embedded SQL to remove `tenant_id` and adjust parameter counts. Keep the public signatures unchanged.

| Function | New SQL pattern | Notes |
|---|---|---|
| `queryEnabled` | `SELECT enabled, config FROM feature_flags WHERE key = $1` | Single `key` parameter. |
| `GetAllEnabled` | `SELECT key, enabled, config FROM feature_flags` | No `WHERE tenant_id = $1`. Remove the `ID` parameter from the SQL only. |
| `SetEnabled` | `INSERT INTO feature_flags (key, enabled, updated_by) VALUES ($1, $2, $3) ON CONFLICT (key) DO UPDATE SET enabled = $2, updated_by = $3, updated_at = now()` | Remove `tenant_id` and `ID` from VALUES. |
| `UpdateFlag` | `INSERT INTO feature_flags (key, enabled, config, updated_by) VALUES ($1, $2, $3, $4) ON CONFLICT (key) DO UPDATE SET enabled = $2, config = $3, updated_by = $4, updated_at = now()` | Remove `tenant_id` and `ID`. |
| `DeleteFlag` | `DELETE FROM feature_flags WHERE key = $1` | Single `key` parameter. |
| `ListFlagRows` | `SELECT key, enabled, config, updated_at FROM feature_flags` | No `WHERE`. |
| `GetFlagConfig` | `SELECT config FROM feature_flags WHERE key = $1` | Single `key` parameter. |
| `SetRolloutPercentage` | `INSERT INTO feature_flags (key, enabled, config, updated_by) VALUES ($1, $2, $3, $4) ON CONFLICT (key) DO UPDATE SET config = $3, updated_by = $4, updated_at = now()` | Remove `tenant_id` and `ID`. |

Also:
- Change the Valkey cache key to `flagCacheKeyPrefix + flagKey` (no tenant segment).
- Flatten `flagCache.entries` from `map[string]map[string]*flagCacheEntry` to `map[string]*flagCacheEntry`, or keep the two-level map but always use the same value for both keys.
- Update `IsEnabled` to call `cacheGet(key)`, `valkeyGet(ctx, key)`, `cacheSet(key, enabled)`, and `valkeySet(ctx, key, enabled)`.
- `ClearCacheForTenant` should either clear the entire cache or no-op in this phase; it is removed in Phase 2.
- `Gate` still uses `tenant.FromContext` in this phase; that is removed in Phase 2.
- `hashInRollout` may keep using the default tenant constant for this phase so behavior is stable. The final global hashing decision is in Phase 2.

- [ ] **Step 3: Regenerate sqlc**

```bash
make sqlc-gen
```

- [ ] **Step 4: Verify build and tests**

```bash
go build ./internal/features/...
go test ./internal/features/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/features.sql internal/features/features.go internal/db/gen/features.sql.go
git commit -m "refactor: make feature_flags global in Phase 1.1"
```

---

## Task 5: Regenerate sqlc and baseline build

**Files:**
- Regenerate: `internal/db/gen/*`

- [ ] **Step 1: Run `make sqlc-gen`**

```bash
make sqlc-gen
```

- [ ] **Step 2: Build the backend**

```bash
go build ./...
```

This is expected to fail for packages that still reference `tenant` package methods or `ID` fields outside the settings/features subsystems. Those failures become the Phase 2 backlog.

- [ ] **Step 3: Capture the build-failure list**

```bash
go build ./... 2>&1 | tee /tmp/phase1_build_errors.txt
```

- [ ] **Step 4: Commit generated changes**

```bash
git add internal/db/gen
git commit -m "chore: regenerate sqlc after settings and feature flag de-tenanting"
```

---

## Task 6: Run tests and security scans

- [ ] **Step 1: Run unit tests for the de-tenanted subsystems**

```bash
go test ./internal/config/...
go test ./internal/features/...
```

- [ ] **Step 2: Run `gosec`**

```bash
gosec ./...
```

Expected: no new HIGH/CRITICAL findings.

- [ ] **Step 3: Run `go vet`**

```bash
go vet ./...
```

- [ ] **Step 4: Run `golangci-lint` if available**

```bash
make lint
```

- [ ] **Step 5: Document results**

If any test or lint failure is unrelated to `tenant` (pre-existing), note it in the Phase 1 summary. Failures caused by `tenant` usage in non-query code are expected and become the Phase 2 backlog.

---

## Task 7: Phase 1.0/1.1 handoff

- [ ] **Step 1: Add a Phase 1.1 note to `/.jeos/adr/ADR-004-multi-tenancy-from-day-one.md`**

Add a short amendment recording that Phase 1.0/1.1 is complete and `tenant_id` columns are now defunct.

- [ ] **Step 2: Commit the ADR note**

```bash
git add .jeos/adr/ADR-004-multi-tenancy-from-day-one.md
git commit -m "docs: add Phase 1.1 completion note to ADR-004"
```

- [ ] **Step 3: Push only if explicitly requested**

```bash
git push origin main
```

Do not push without user confirmation.

---

## Appendix A: Per-subsystem query de-tenanting child plans

The remaining 94 `.sql` files in `internal/db/queries/` are de-tenanted one subsystem at a time. Each child plan targets a single subsystem (for example `contents`, `comments`, or `users`). The parent agent dispatches a subagent with the child plan and the exact file list for that subsystem.

A child plan contains:

1. **Exact file list.** The parent agent provides the list of `.sql` files in `internal/db/queries/` for that subsystem. Example for `users`:
   - `internal/db/queries/users.sql`
2. **Transformation rules.**
   - Remove `tenant_id` from `WHERE`, `JOIN`, `ON CONFLICT`, `GROUP BY`, `ORDER BY`, and `RETURNING` clauses.
   - Remove `tenant_id` from `INSERT` column lists and `VALUES`.
   - Remove `tenant_id` from `SELECT` result columns.
   - Rewrite `ON CONFLICT (tenant_id, ...)` to `ON CONFLICT (...)`.
   - Renumber positional parameters (`$1`, `$2`, ...) after removing a parameter. Where a parameter is reused in multiple places, keep the same number for the same value.
3. **Verification commands (using `users` as the example).**
   - `make sqlc-gen`
   - `go build ./...`
   - `go test ./internal/users/...` (if the package compiles)
4. **Commit instructions (using `users` as the example).**
   - `git add internal/db/queries/users.sql internal/db/gen/users.sql.go`
   - `git commit -m "refactor: de-tenant users queries"`

No child plan proceeds until `go build ./...` succeeds for the packages it touches.

---

## Plan verification checklist

- [ ] Spec coverage: every Phase 1.0/1.1 item in the design doc has a task.
- [ ] Placeholder scan: no "TBD", "TODO", or "implement later" remains.
- [ ] Type consistency: settings and features generated params no longer contain `ID`.
- [ ] Down migrations: both 000665 up and down migrations exist and are paired.
