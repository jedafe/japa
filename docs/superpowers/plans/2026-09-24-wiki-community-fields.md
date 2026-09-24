# Wiki Community Fields Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `wiki_revisions` into the platform's community contribution queue — revisions can carry structured field ops against any `(entity_type, entity_id)`, tiered review applies them to `custom_field_values`, and JCE comparison cells hydrate live from entity `field_values`.

**Architecture:** Approach A+ (polymorphic ops on the existing wiki revision queue, `wiki_page_id` optional). Ops are validated at submit (field exists, type-coercible, `can_edit` per §06), auto-approved for `wiki.fields.manage` holders, applied transactionally on approve, and propagated to JCE via the `wiki:fields_propagate` Asynq task + `comparison_items.item_ref` GIN index.

**Tech Stack:** Go (chi, pgx, sqlc), PostgreSQL migrations (`cmd/migrate`), Asynq, Valkey, SvelteKit 2 + Svelte 5 (`apps/main` SSR, `apps/admin` SPA via EntityPage scaffold).

**Spec:** `docs/superpowers/specs/2026-09-24-wiki-community-fields-design.md` — read it first. JEOS amendments already landed in `.jeos` (§06, §30 Sub-spec 5, §80 Community Field Ops, catalogs).

## Global Constraints

- `make migrate` runs migrations; `make sqlc-gen` regenerates `internal/db/gen` — never hand-edit generated files (Rule 2).
- All Valkey keys use `valkey.MustKey(...)` producing `jc:`-prefixed keys; no new key patterns beyond what the spec lists.
- API errors use `errpkg` standard format with `snake_case` codes (Rule 5).
- New routes are gated by `features.Gate(featureChecker, "wiki.structured_edits")` inside the existing `wiki.enabled` group.
- New RBAC nodes: `wiki.fields.edit`, `wiki.fields.propose`, `wiki.fields.manage` — seed them in the migration (see `migrations/000730_seed_university_courses_rbac_nodes.up.sql` for the seeding pattern).
- Asynq task `wiki:fields_propagate` is already in the §11 catalog — register it in `internal/worker/tasks.go` like `TypeWikiSeedStubPages`.
- Frontend: `bun`/`bunx` only, never npm (Rule 18). Admin list UIs use the `EntityPage` scaffold (see `apps/admin/AGENTS.md`).
- No AI-attribution strings anywhere (Rule 17).
- `wiki` package store uses handwritten SQL against `*db.DB` (not sqlc) — follow that convention inside `internal/wiki/store.go`. New queries against comparison tables go in `internal/db/queries/comparison.sql` + `make sqlc-gen`.

---

### Task 1: Migration 000731 — schema

**Files:**
- Create: `migrations/000731_wiki_community_fields.up.sql`
- Create: `migrations/000731_wiki_community_fields.down.sql`

**Interfaces:**
- Produces: `wiki_revisions.kind`, `wiki_revisions.ops`, `wiki_revisions.target_entity_type`, `wiki_revisions.target_entity_id`, nullable `wiki_revisions.wiki_page_id`; `comparison_dimensions.source_field_id`; `wiki_pages.data_entity_type`; RBAC nodes `wiki.fields.edit|propose|manage`.

- [ ] **Step 1: Write the up migration**

```sql
-- 000731_wiki_community_fields.up.sql
-- §80 Community Field Ops + §30 Sub-spec 5
ALTER TABLE wiki_revisions
  ADD COLUMN kind text NOT NULL DEFAULT 'content',
  ADD COLUMN ops jsonb NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN target_entity_type text,
  ADD COLUMN target_entity_id uuid,
  ALTER COLUMN wiki_page_id DROP NOT NULL;

ALTER TABLE wiki_revisions
  ADD CONSTRAINT wiki_revisions_kind_check
    CHECK (kind IN ('content','fields','mixed'));

CREATE INDEX idx_wiki_revisions_target
  ON wiki_revisions (target_entity_type, target_entity_id);
CREATE INDEX idx_wiki_revisions_status_kind
  ON wiki_revisions (status, kind);

ALTER TABLE comparison_dimensions
  ADD COLUMN source_field_id uuid REFERENCES custom_fields(id);

ALTER TABLE wiki_pages
  ADD COLUMN data_entity_type text;

-- RBAC nodes (mirror 000730 seeding pattern: insert into the permissions/rbac
-- catalog table used by that migration — read it and copy the INSERT shape).
-- Nodes: wiki.fields.edit, wiki.fields.propose, wiki.fields.manage
-- Default grants: wiki.fields.manage → admin, super_admin groups.
```

**Important:** open `migrations/000730_seed_university_courses_rbac_nodes.up.sql` first and copy its exact INSERT statement shape for the three new nodes — do not invent the table name.

- [ ] **Step 2: Write the down migration**

```sql
-- 000731_wiki_community_fields.down.sql
DELETE FROM wiki_revisions WHERE wiki_page_id IS NULL;  -- page-less ops revisions
ALTER TABLE wiki_revisions
  ALTER COLUMN wiki_page_id SET NOT NULL,
  DROP COLUMN target_entity_id,
  DROP COLUMN target_entity_type,
  DROP COLUMN ops,
  DROP COLUMN kind;
ALTER TABLE comparison_dimensions DROP COLUMN source_field_id;
ALTER TABLE wiki_pages DROP COLUMN data_entity_type;
-- + DELETE the three rbac node rows (mirror 000730 down shape)
```

- [ ] **Step 3: Run migration**

Run: `make migrate`
Expected: applies cleanly; `wiki_revisions` shows new columns.

- [ ] **Step 4: Commit**

```bash
git add migrations/000731_wiki_community_fields.up.sql migrations/000731_wiki_community_fields.down.sql
git commit -m "Add wiki community fields schema (§80 ops revisions, §30 source_field_id)"
```

---

### Task 2: sqlc — propagation query + dimension source_field_id

**Files:**
- Modify: `internal/db/queries/comparison.sql` (dimension queries + new item_ref query)
- Regenerate: `internal/db/gen/comparison.sql.go` via `make sqlc-gen`

**Interfaces:**
- Produces: `gen.ListComparisonContentIDsByItemRef(ctx, itemRef []byte) ([]pgtype.UUID, error)`; `CreateComparisonDimensionParams.SourceFieldID`; `source_field_id` present on dimension row types returned by `ListComparisonDimensionsByContent`/`GetComparisonDimensionByID`/`UpdateComparisonDimension`.

- [ ] **Step 1: Update dimension queries to carry source_field_id**

In `internal/db/queries/comparison.sql`, update `CreateComparisonDimension`:

```sql
-- name: CreateComparisonDimension :one
INSERT INTO comparison_dimensions (content_id, type_id, label, dimension_type, position, source_field_id) VALUES (@content_id, sqlc.narg(type_id), @label, @dimension_type, @position, sqlc.narg(source_field_id))
RETURNING id, content_id, type_id, label, dimension_type, position, source_field_id, created_at, deleted_at;
```

Add `source_field_id` to the SELECT column list of `ListComparisonDimensionsByContent`, `GetComparisonDimensionByID`, and to both `SET` (as `sqlc.narg`) and `RETURNING` of `UpdateComparisonDimension`.

- [ ] **Step 2: Add the propagation lookup query**

Append to `internal/db/queries/comparison.sql`:

```sql
-- name: ListComparisonContentIDsByItemRef :many
-- Finds published comparison contents containing an item whose item_ref
-- matches {type, id} exactly (§30 Sub-spec 5 propagation).
SELECT DISTINCT content_id
FROM comparison_items
WHERE deleted_at IS NULL
  AND item_ref @> @item_ref::jsonb;
```

- [ ] **Step 3: Regenerate + build**

Run: `make sqlc-gen && go build ./internal/...`
Expected: compiles; `CreateComparisonDimensionParams` has `SourceFieldID pgtype.UUID`.

- [ ] **Step 4: Fix compile errors from signature change**

`internal/comparison/jce_service.go` (~line 457) and any other `CreateComparisonDimension` callers now need `SourceFieldID` in params. In the autoFields loop, set `SourceFieldID: f.ID` (fields are `gen.ListCustomFieldsByGroupRow`). Other callers pass `pgtype.UUID{}`.

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/comparison.sql internal/db/gen/ internal/comparison/jce_service.go
git commit -m "Link comparison dimensions to source fields; add item_ref propagation query"
```

---

### Task 3: Wiki store — ops columns on revisions

**Files:**
- Modify: `internal/wiki/store.go` (WikiRevision struct, SubmitRevision, ApproveRevision, RejectRevision, listers)

**Interfaces:**
- Consumes: migration columns from Task 1.
- Produces:
  - `WikiRevision` fields: `Kind string`, `Ops []byte`, `TargetEntityType *string`, `TargetEntityID pgtype.UUID`
  - `Store.SubmitRevision(ctx, pageID, userID pgtype.UUID, kind string, body, ops []byte, summary string, targetType *string, targetID pgtype.UUID) (pgtype.UUID, error)` — extended signature (update existing callers)
  - `Store.GetRevision(ctx, revID pgtype.UUID) (WikiRevision, error)`
  - `Store.ApproveRevisionByID(ctx, revID, reviewerID pgtype.UUID, notes string) (WikiRevision, error)` — revision-keyed approve returning the full row (kind+ops+content)
  - `Store.RejectRevisionByID(ctx, revID, reviewerID pgtype.UUID, notes string) error`
  - `Store.ListRevisionsByTarget(ctx, entityType string, entityID pgtype.UUID, limit, offset int32) ([]WikiRevision, int64, error)`
  - `Store.AdminListRevisions(ctx, kind, status string, limit, offset int32) ([]WikiRevision, int64, error)`
  - `Store.CountPendingByTarget(ctx, entityType string, entityID pgtype.UUID) (int64, error)`
  - `Store.FindHostPageID(ctx, spaceID pgtype.UUID, category string) (pgtype.UUID, error)` — returns first active page for space+category, `pgx.ErrNoRows` if none

- [ ] **Step 1: Write failing tests**

Add `internal/wiki/store_ops_test.go`. Use the existing test helpers in `internal/wiki` (check `spaces_test.go`-style helpers / `internal/testdb`). Cover: SubmitRevision with `kind="fields"`, ops bytes, and `wiki_page_id NULL`; `AdminListRevisions` kind filter; `CountPendingByTarget`.

```go
func TestSubmitRevision_PageLessFieldsRevision(t *testing.T) {
    // insert revision with nil page id, kind='fields', ops payload
    // read back via GetRevision — assert kind, ops, target round-trip
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/wiki/ -run TestSubmitRevision_PageLessFieldsRevision -v`
Expected: FAIL (columns/methods don't exist).

- [ ] **Step 3: Implement**

Update `WikiRevision` struct and every `INSERT INTO wiki_revisions` / `SELECT ... FROM wiki_revisions` in `store.go` to include `kind, ops, target_entity_type, target_entity_id`. `SubmitRevision` gains the new params — content `body` may be empty (`'{}'::jsonb`) for `fields` revisions. `ApproveRevision` keeps its signature for page-scoped callers; add `ApproveRevisionByID`/`RejectRevisionByID` that don't require `pageID`.

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/wiki/ && go build ./internal/...`
Expected: PASS; fix compile errors at the service call site (`Service.SubmitRevision` passes `"content"`, `nil` ops — extended in Task 6).

- [ ] **Step 5: Commit**

```bash
git add internal/wiki/
git commit -m "wiki store: ops/kind/target columns on revisions, revision-keyed approve"
```

---

### Task 4: Field ops types + validation

**Files:**
- Create: `internal/wiki/fieldops.go`
- Test: `internal/wiki/fieldops_test.go`

**Interfaces:**
- Consumes: `customfields.IsValidFieldType`, `customfields.FieldTypeValueColumn`, `customfields.ValidOptionsShape`.
- Produces:
  - `type FieldOp struct { Op, EntityType, EntityID, FieldKey, GroupSlug, EntityClass, SourceURL string; Value json.RawMessage; Field *FieldDefInput; Group *GroupDefInput }` (with json tags matching spec §5)
  - `const OpSetValue/OpProposeField/OpProposeGroup/OpUpsertField/OpAssignGroup`
  - `type FieldDefInput struct { Key, Label, FieldType string; Options []customfields.FieldOption; IsRequired bool; Position int32; Placeholder, DefaultValue string }`
  - `type GroupDefInput struct { Slug, Name string; Description string; Fields []FieldDefInput }`
  - `func ParseOps(raw []byte) ([]FieldOp, error)`
  - `func ValidateOps(ops []FieldOp, maxOps int) error` — structural only (no DB)
  - `func DeriveKind(hasContent bool, ops []FieldOp) string`

- [ ] **Step 1: Write failing tests**

```go
func TestValidateOps_RejectsUnknownOp(t *testing.T) { … }
func TestValidateOps_RejectsMissingEntityTargetOnSetValue(t *testing.T) { … }
func TestValidateOps_BoundsMaxOps(t *testing.T) { … }
func TestValidateOps_RejectsBadFieldTypeOnPropose(t *testing.T) { … }
func TestDeriveKind(t *testing.T) { /* content-only→content, ops-only→fields, both→mixed */ }
```

- [ ] **Step 2: Run to verify failure** — `go test ./internal/wiki/ -run TestValidateOps -v` → FAIL.

- [ ] **Step 3: Implement `fieldops.go`**

Rules: `set_value` requires `entity_type`, `entity_id` (uuid-parseable), `field_key`, non-empty `value`; `propose_field` requires `group_slug` + `field{key,label,field_type}` with `IsValidFieldType` + `ValidOptionsShape` for select/multiselect; `propose_group` requires `group{slug,name}` + `entity_class`; `upsert_field` requires `group_slug` + `field`; `assign_group` requires `entity_class` + `group_slug`. `len(ops) <= maxOps`; `maxOps <= 0` treated as unlimited only inside tests — service always passes the setting.

- [ ] **Step 4: Test + commit**

Run: `go test ./internal/wiki/ -run 'TestValidateOps|TestDeriveKind' -v` → PASS.

```bash
git add internal/wiki/fieldops.go internal/wiki/fieldops_test.go
git commit -m "wiki: field ops types and structural validation"
```

---

### Task 5: Apply ops — transactional writer

**Files:**
- Create: `internal/wiki/fieldops_apply.go`
- Test: `internal/wiki/fieldops_apply_test.go`

**Interfaces:**
- Consumes: `customfields.Service` methods (`GetFieldGroupBySlug`, `GetCustomFieldByKey`, `UpsertFieldValue`, `CreateField`, `CreateFieldGroup`, `CreateAssignment`, `CheckFieldPermissionForUser`) — accessed through a narrow interface defined here so wiki stays decoupled.
- Produces:
  - `type FieldStore interface { GetFieldGroupBySlug(ctx, string) (gen.CustomFieldGroup, error); GetCustomFieldByKey(ctx, string) (gen.GetCustomFieldByKeyRow, error); UpsertFieldValue(ctx, gen.UpsertCustomFieldValueParams) (gen.CustomFieldValue, error); CreateField(ctx, gen.CreateCustomFieldParams) (gen.CreateCustomFieldRow, error); UpdateField(ctx, gen.UpdateCustomFieldParams) (gen.UpdateCustomFieldRow, error); CreateFieldGroup(ctx, name, slug string, desc *string, isSystem bool) (gen.CustomFieldGroup, error); CreateAssignment(ctx, gen.CreateAssignmentParams) (gen.CustomFieldGroupAssignment, error); CheckFieldPermissionForUser(ctx, fieldID, userID pgtype.UUID) (bool, bool, error) }`
  - `func (s *Service) applyOps(ctx context.Context, ops []FieldOp) error` — executes ops in order; callers MUST invoke it inside `db.WithTx(ctx, s.db.Writer, ...)` so a revision applies all-or-nothing
  - `var entityFieldValuesTable = map[string]string{"space":"spaces","university":"spaces","country":"spaces","state":"spaces","city":"spaces","organization":"organizations","bank":"organizations","content":"contents"}` — whitelisted denorm targets; entity types not in the map skip the denorm write (canonical store is `custom_field_values`)
  - `func valueToUpsertParams(field gen.GetCustomFieldByKeyRow, entityType string, entityID pgtype.UUID, v json.RawMessage) (gen.UpsertCustomFieldValueParams, error)` — picks the value column via `customfields.FieldTypeValueColumn`

- [ ] **Step 1: Write failing tests**

Table-test `valueToUpsertParams`: text→ValueText, number→ValueNumber, bool→ValueBool, select→ValueJson, bad JSON→error. Test `applyOps` ordering with a fake FieldStore (propose_group → assign_group → set_value in one ops array applies cleanly).

- [ ] **Step 2: Run to verify failure** — `go test ./internal/wiki/ -run TestApplyOps -v` → FAIL.

- [ ] **Step 3: Implement**

`applyOps` switch per `op.Op`:
- `set_value`: resolve field via `GetCustomFieldByKey(op.FieldKey)` → `valueToUpsertParams` → `UpsertFieldValue`; then `UPDATE <table> SET field_values = field_values || jsonb_build_object(key, value) WHERE id = $1` when `entityFieldValuesTable[op.EntityType]` exists (table name from the whitelist map only — never from op input).
- `propose_field`: `GetFieldGroupBySlug` → `CreateField` (params from `FieldDefInput`; `is_system=false`).
- `propose_group`: `CreateFieldGroup` → `CreateField` per `group.Fields` → `CreateAssignment{EntityClass: op.EntityClass}`.
- `upsert_field`: `GetFieldGroupBySlug` + `GetCustomFieldByKey` → `UpdateField` (add `UpdateField` to FieldStore iface: `UpdateField(ctx, gen.UpdateCustomFieldParams)`).
- `assign_group`: `GetFieldGroupBySlug` → `CreateAssignment`.

- [ ] **Step 4: Test + commit**

```bash
go test ./internal/wiki/ -run TestApplyOps -v
git add internal/wiki/fieldops_apply.go internal/wiki/fieldops_apply_test.go
git commit -m "wiki: transactional field-ops apply (values, proposals, definitions)"
```

---

### Task 6: Wiki service — submit, suggest, fields schema, approve

**Files:**
- Modify: `internal/wiki/service.go` (SubmitRevision extension + new methods + SetFieldStore/SetEntityResolver injectors)
- Test: extend `internal/wiki/fieldops_test.go` or new `service_ops_test.go`

**Interfaces:**
- Consumes: Task 3 store methods, Task 4 `FieldOp`/`ValidateOps`/`DeriveKind`, Task 5 `applyOps`/`FieldStore`, `resolver.Registry.Resolve(ctx, map[string]any)`.
- Produces:
  - `Service.SetFieldStore(fs FieldStore)` and `Service.SetEntityResolver(r interface{ Resolve(context.Context, map[string]any) (resolver.ResolvedItem, error) })` — wire `*comparison.Service`'s resolver registry in server.go (comparison already exposes `SetResolver`; export a small `ResolveItemRef(ctx, entityType, entityID string) error` helper on comparison.Service if the registry isn't directly reachable — pick whichever compiles cleanly).
  - `Service.SubmitFieldOps(ctx, userID pgtype.UUID, pageID *pgtype.UUID, entityType string, entityID pgtype.UUID, ops []FieldOp, summary string, canManage bool) (revID string, autoApproved bool, err error)`
  - `Service.SuggestFields(ctx, userID pgtype.UUID, entityType string, entityID pgtype.UUID, ops []FieldOp, summary string, canManage bool) (string, bool, error)` — resolves entity via resolver (`{"type":entityType,"id":entityID.String()}`), best-effort `FindHostPageID` on the entity's `Metadata["geo_space_id"]`, delegates to `SubmitFieldOps` with `pageID=nil` when no host page.
  - `Service.GetEntityFields(ctx, userID, entityType string, entityID pgtype.UUID) (EntityFieldsDTO, error)` — field groups via `ListAssignmentsByEntityClass(entityClassFor(entityType, spaceTypeID))` + `ListFieldValuesWithDefsByEntity` + `CountPendingByTarget`; visible/canEdit flags via `CheckFieldPermissionForUser`.
  - `Service.ListEntityRevisions(ctx, entityType string, entityID pgtype.UUID, page, limit int32)` → `ListRevisionsByTarget`.
  - `Service.ApproveRevisionByID(ctx, revID, reviewerID pgtype.UUID, notes string) error` — store approve + `applyOps` inside `db.WithTx` + enqueue propagate.
  - `entityClassFor(entityType string, spaceTypeID pgtype.UUID) string` — `space|university|country|state|city → "space_type:"+id`; `organization|bank → "organization"`; `content:* → "content_type:*"`; everything else → `"jce_entity:"+entityType`.

- [ ] **Step 1: Write failing tests**

```go
func TestSubmitFieldOps_ManageTierAutoApproves(t *testing.T) { … }
func TestSubmitFieldOps_RejectsWhenCanEditFalse(t *testing.T) { … }
func TestSuggestFields_Pageless(t *testing.T) { … }
func TestApproveRevisionByID_AppliesOpsAndEnqueues(t *testing.T) { … }
```

- [ ] **Step 2: Run to verify failure** → FAIL (methods don't exist).

- [ ] **Step 3: Implement**

Submit path: `ValidateOps(ops, settings "wiki.suggest.max_ops_per_revision" default 50)` → for each `set_value`, `CheckFieldPermissionForUser` must return `canEdit` (skip check when `canManage`) → store `SubmitRevision` with `kind=DeriveKind(false, ops)`, `ops` marshaled, `target_*` set → if `canManage`, immediately run approve path. Edit lock: reuse the existing `jc:wiki_edit_lock:{page_id}:{user_id}` pattern only when a host page exists — page-less `suggest` submissions skip locking (ops are re-validated and applied idempotently at approve time, so a duplicate submission is harmless).

- [ ] **Step 4: Test + commit**

```bash
go test ./internal/wiki/ -v
git add internal/wiki/
git commit -m "wiki service: field-ops submit/suggest/approve with tiered auto-approve"
```

---

### Task 7: Worker — `wiki:fields_propagate`

**Files:**
- Modify: `internal/worker/tasks.go` (const + registry map), `internal/worker/handlers.go` (handler), `internal/wiki/workeradapter.go` (adapter)
- Modify: `internal/server/server.go` (registration)
- Test: `internal/worker/wiki_fields_propagate_test.go`

**Interfaces:**
- Consumes: `comparison.Service.DeleteJCEComparisonCaches(ctx, contentID)`; `gen.Queries.ListComparisonContentIDsByItemRef` (Task 2).
- Produces: `TypeWikiFieldsPropagate = "wiki:fields_propagate"`; payload `{entity_type, entity_id}`; `wiki.WorkerAdapter.PropagateFields(ctx, entityType, entityID string) error` implemented via a new `Service.PropagateFields` that runs the GIN lookup then calls the injected `comparisonCacheBuster` interface (`DeleteJCEComparisonCaches(ctx, pgtype.UUID)`).

- [ ] **Step 1: Write failing test** — handler with bad payload errors; valid payload calls adapter once (spy processor).

- [ ] **Step 2: Implement**

```go
// tasks.go
TypeWikiFieldsPropagate = "wiki:fields_propagate"
// registry: {TypeWikiFieldsPropagate, HandleWikiFieldsPropagate}

// handlers.go (mirror HandleWikiSeedStubPages shape, ~line 5228)
func HandleWikiFieldsPropagate(ctx context.Context, t *asynq.Task) error { … }
```

Add `WikiFieldsPropagator` processor interface + `RegisterWikiFieldsPropagator` setter, and in `server.go` near line 1270: `worker.RegisterWikiFieldsPropagator(wiki.NewWorkerAdapter(wikiService))` plus inject the cache buster into the wiki service (`wikiService.SetJCECacheBuster(comparisonService)` — check the actual comparison service variable name in server.go).

Enqueue site: `Service.approveRevisionOps` calls the injected enqueuer (`worker.Enqueuer` via `Service.SetEnqueuer`, mirroring how other services enqueue — check `internal/wiki/service.go` `SetDispatcher` for the existing injection pattern).

- [ ] **Step 3: Test + build + commit**

```bash
go test ./internal/worker/ -run TestWikiFieldsPropagate -v && go build ./...
git add internal/worker/ internal/wiki/ internal/server/server.go
git commit -m "worker: wiki:fields_propagate task busts JCE caches for edited entities"
```

---

### Task 8: Handlers + routes

**Files:**
- Modify: `internal/wiki/handlers.go` (new handlers + extended SubmitRevision body)
- Modify: `internal/server/server.go` (route wiring, ~line 6359)
- Modify: `internal/wiki/service.go` DTOs if handler shapes need them

**Interfaces:**
- Consumes: Task 6 service methods; `permissionChecker.HasPermission`; `pointsChecker` (reputation gate already in handler).
- Produces HTTP surface per spec §7.

- [ ] **Step 1: Extend `SubmitRevisionInput`** — add `Ops []FieldOp \`json:"ops"\``; `HandleSubmitRevision` derives kind and routes to `SubmitFieldOps` when ops present (page context known).

- [ ] **Step 2: New handlers**

```go
// HandleSuggestFields  POST /api/v1/wiki/suggest
//   body {entity_type, entity_id, ops, summary}; 403 unless wiki.fields.edit
//   or wiki.fields.propose covers every op kind in the payload.
// HandleGetEntityFields GET /api/v1/wiki/entities/{type}/{id}/fields
// HandleGetEntityRevisions GET /api/v1/wiki/entities/{type}/{id}/revisions
// HandleAdminListRevisions GET /api/v1/admin/wiki/revisions?kind=&status=&page=&limit=
// HandleApproveRevisionByID POST /api/v1/admin/wiki/revisions/{id}/approve
// HandleRejectRevisionByID  POST /api/v1/admin/wiki/revisions/{id}/reject
```

`canManage := h.perms.HasPermission(ctx, userID, "wiki.fields.manage")` passed into service calls.

- [ ] **Step 3: Wire routes in server.go** inside the existing `features.Gate(featureChecker, "wiki.enabled")` group, nested under a second gate for the new flag:

```go
r.With(features.Gate(featureChecker, "wiki.structured_edits")).Group(func(r chi.Router) {
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.fields.edit")).
        Post("/wiki/suggest", wikiHandlers.HandleSuggestFields)
    r.Get("/wiki/entities/{type}/{id}/fields", wikiHandlers.HandleGetEntityFields)
    r.Get("/wiki/entities/{type}/{id}/revisions", wikiHandlers.HandleGetEntityRevisions)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.review")).
        Get("/admin/wiki/revisions", wikiHandlers.HandleAdminListRevisions)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.review")).
        Post("/admin/wiki/revisions/{id}/approve", wikiHandlers.HandleApproveRevisionByID)
    r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "wiki.review")).
        Post("/admin/wiki/revisions/{id}/reject", wikiHandlers.HandleRejectRevisionByID)
})
```

Note chi route-conflict check: `/wiki/{space_slug}` vs `/wiki/suggest` — chi matches literals before wildcards at the same position, and `/wiki/entities/...` is a distinct prefix. Verify with `go build` + a quick `chi.WalkRoutes`-style sanity test if one exists; order registration so literal routes are added before `{space_slug}` if needed.

- [ ] **Step 4: Seed the feature flag** — follow the existing flag-seeding pattern (search `feature.jce` seeds in migrations) — add `wiki.structured_edits` default `false` in the same migration file style (separate `000732_…` migration if the seeder is a migration; check `system_settings` vs `feature_flags` table used by `features.Gate`).

- [ ] **Step 5: Build + smoke + commit**

```bash
go build ./... && go test ./internal/wiki/ ./internal/server/
git add internal/wiki/ internal/server/server.go migrations/000732*
git commit -m "wiki: structured field-ops API routes (suggest, entity fields, revision-keyed review)"
```

---

### Task 9: JCE live hydration

**Files:**
- Modify: `internal/comparison/service.go` (`GetComparisonMatrix` ~line 1412, `ComparisonMatrixDimension` ~line 1399)
- Modify: `internal/comparison/resolver/db.go` + `resolver/space.go`, `resolver/university.go`, `resolver/content.go` (expose `field_values` in `ResolvedItem.Metadata`)
- Test: `internal/comparison/service_test.go` or `comparison_test.go` additions

**Interfaces:**
- Consumes: Task 2 `source_field_id` on dimension rows; `resolver.Registry` already on `Service` (`s.resolver` — check field name at `comparison.go:129` `SetResolver`).
- Produces:
  - `ComparisonMatrixDimension.SourceFieldID pgtype.UUID \`json:"source_field_id,omitempty"\``, `HydratedFrom string \`json:"hydrated_from,omitempty"\`` (`"entity_field"`)
  - `ResolvedItem.Metadata["field_values"]` = `map[string]any` from the entity's field_values column (DBResolver: `parseDBFieldValues(rv.fieldValues)` raw, in addition to existing `score_inputs`).

- [ ] **Step 1: Write failing test**

```go
func TestGetComparisonMatrix_HydratesFromEntityField(t *testing.T) {
    // dimension with source_field_id, item with item_ref {type:'university',id:X},
    // no comparison_values row → cell value from resolved field_values[field.key],
    // HydratedFrom == "entity_field". Explicit comparison_values row wins.
}
```

- [ ] **Step 2: Run to verify failure** → FAIL.

- [ ] **Step 3: Implement**

In `GetComparisonMatrix`: collect dims with `SourceFieldID.Valid`; batch-load `custom_fields` rows (need `field.key` — add a `customfields` lookup the service already has via `s.cfSvc`, e.g. `GetField`/`GetCustomFieldByKey` — check available method on `cfSvc`); for each item with non-empty `item_ref`, call `s.resolver.Resolve` once and cache per item; fill empty cells from `fieldValues[field.Key]`, setting `HydratedFrom="entity_field"`. Resolver changes: in `dbResolverScoreInputs`/`toItem`, add `meta["field_values"] = fields` (raw map); space/university/content resolvers do the same from their entity's `field_values` column (universities live in `spaces`/`universities` — check which table the resolver reads and use that row's `field_values`).

- [ ] **Step 4: Test + commit**

```bash
go test ./internal/comparison/ -run TestGetComparisonMatrix -v
git add internal/comparison/
git commit -m "jce: hydrate matrix cells live from entity field_values (sub-spec 5)"
```

---

### Task 10: apps/main — wiki Data panel + SuggestEdit

**Files:**
- Create: `apps/main/src/lib/components/wiki/EntityFieldsPanel.svelte`, `apps/main/src/lib/components/wiki/SuggestEdit.svelte`
- Modify: `apps/main/src/routes/(app)/wiki/[space_slug]/[page_slug]/+page.svelte`, `+page.server.ts`
- Modify: `apps/main/src/lib/api.ts` (new endpoints), `apps/main/src/lib/i18n/en.json` + `fr.json`

**Interfaces:**
- Consumes: `GET /api/v1/wiki/entities/{type}/{id}/fields` response `{groups: [{name, fields: [{key,label,field_type,options,value,can_edit}]}], pending_count}`; `POST /api/v1/wiki/suggest`.
- Produces: `<SuggestEdit entityType entityId label?>` — fetches schema, renders field inputs per `field_type` (reuse the composer `CustomFieldGroup.svelte` renderer at `apps/main/src/lib/components/create/panel/CustomFieldGroup.svelte` — it already renders §06 field types), submits `{entity_type, entity_id, ops:[{op:'set_value',…}], summary}`.

- [ ] **Step 1: API client** — add `api.suggestFields`, `api.getEntityFields` in `apps/main/src/lib/api.ts` matching existing fetch wrapper conventions.

- [ ] **Step 2: `SuggestEdit.svelte`** — button → slide-over with field form; on submit POST; show "submitted for review" toast; if `pending_count > 0` badge "N pending". Gate render on a `wiki.structured_edits` flag already present in the feature-flags payload the app loads (check how `wiki.enabled` is consumed in `(app)/wiki` pages — same mechanism).

- [ ] **Step 3: Data panel on wiki page** — on `[page_slug]/+page.svelte`, below prose, render `<EntityFieldsPanel>` for the page's target: space itself when `data_entity_type` is null, else list child entities via `GET /api/v1/jce/entities/{type}?geo_space_id={space_id}` (existing endpoint) with per-row `<SuggestEdit>`. Server `load` fetches fields schema for the space entity.

- [ ] **Step 4: Verify** — `bun run check` in `apps/main`; manual smoke on a space wiki page.

- [ ] **Step 5: Commit**

```bash
cd apps/main && git add . && git commit -m "main: wiki data panel + SuggestEdit component"
```

---

### Task 11: apps/admin — review queue for fields revisions

**Files:**
- Modify: `apps/admin/src/routes/community/wiki/+page.svelte` (and its `+page.ts`/api bindings)
- Modify: `apps/admin/src/lib/api.ts`
- Create (if needed): `apps/admin/src/lib/components/wiki/RevisionDiff.svelte`

**Interfaces:**
- Consumes: `GET /api/v1/admin/wiki/revisions?kind=&status=`; revision rows include `ops`, `kind`, `target_entity_type`, `target_entity_id`; approve/reject via new revision-keyed routes.
- Produces: queue filter chip (all/content/fields/mixed); fields rows render diff table `field_key · old → new · target entity`; approve/reject call `/admin/wiki/revisions/{id}/approve|reject`.

- [ ] **Step 1: api.ts** — add `adminApi.listWikiRevisions(params)`, `approveWikiRevision(id)`, `rejectWikiRevision(id)` (keep page-scoped calls for the existing page list).

- [ ] **Step 2: Page update** — the existing `community/wiki` page: add the kind filter and, for `kind != 'content'` rows, render `RevisionDiff` (ops table: op, entity, field, old→new using `current_values` from the `fields` endpoint if cheap — otherwise show proposed value + `source_url`). Use the EntityPage/DataTable conventions from `apps/admin/AGENTS.md`; custom row rendering goes through `componentSnippet`/`viewSnippet`, not hand-rolled tables.

- [ ] **Step 3: Verify** — `bun run check` in `apps/admin`; `bun run test` if the page has a colocated test (add one per `AGENTS.md` conventions if missing).

- [ ] **Step 4: Commit**

```bash
git add apps/admin/
git commit -m "admin: wiki review queue kind filter + field-ops diff view"
```

---

## Execution order & dependencies

```
T1 migration ──┬── T2 sqlc ──┬── T9 hydration
               └── T3 store ── T4 ops types ── T5 apply ── T6 service ── T8 handlers/routes
                                                              │
T7 worker ◀──────┘ (needs T2 query + T6 enqueue site)
T10, T11 (frontend) after T8 routes exist.
```

## Definition of done

- `POST /wiki/suggest` on a university (space) with a `set_value` op → pending revision → admin approve → `custom_field_values` row + `spaces.field_values` updated → a JCE comparison containing that university shows the new value with `hydrated_from="entity_field"` after `wiki:fields_propagate` runs.
- `wiki.fields.manage` holder's submit applies immediately with an approved audit row.
- Community `propose_field` creates no live schema until approved.
- `go build ./... && go test ./...` green; `bun run check` green in touched apps.
