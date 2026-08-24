# Design: Dedicated AI Agent CRUD Tools for JapaConnect Use Cases

**Status:** Draft — pending approval
**Refs:** §103 (AI Agent Executor), §78 (Tools Web Surface), §73-§102 (v2 tools/community features), ADR-111

## 1. Problem

The AI agent can list and read the JapaConnect tool catalog via `tools.list` and `tools.read`, but it cannot create, update, delete, or geo-scope records through the agent layer. The generic `tools.create`, `tools.update`, and `tools.assign_geo` rows in `ai_tools` are disabled stubs that return a `use_dedicated_api` error.

This means the agent cannot act like a user/admin and actually maintain tool records. The user has requested full CRUD + assign-geo for all use cases, with real records and no HITL review.

## 2. Goal

Add dedicated, per-use-case agent tools so the AI can create, update, delete, and assign geo scope for each JapaConnect tool table. The agent inherits the user's RBAC, and each tool operation is gated by the appropriate permission.

Examples:
- `visa_routes.create`, `visa_routes.update`, `visa_routes.delete`, `visa_routes.assign_geo`
- `housing_guides.create`, `housing_guides.update`, `housing_guides.delete`, `housing_guides.assign_geo`
- Same pattern for all 28 catalog use cases.

## 3. Design

### 3.1. Tools registry gets write/assign methods

Extend the `ai.ToolsRegistryService` interface with:

```go
type ToolCreateFunc   func(ctx context.Context, tenantID pgtype.UUID, payload map[string]any) (map[string]any, error)
type ToolUpdateFunc   func(ctx context.Context, tenantID pgtype.UUID, id string, payload map[string]any) (map[string]any, error)
type ToolDeleteFunc   func(ctx context.Context, tenantID pgtype.UUID, id string) error
type ToolAssignGeoFunc func(ctx context.Context, tenantID pgtype.UUID, id string, payload map[string]any) (map[string]any, error)
```

These are generic `map[string]any` contracts so the `ai` package does not import every per-tool service. The concrete implementations live in `agent_tools_registry.go` and call the existing per-tool `Service` objects.

### 3.2. Per-tool service wiring

`internal/server/agent_tools_registry.go` gets a new `toolWriter` field with a service handle per use case:

```go
type toolWriter struct {
    Create   ai.ToolCreateFunc
    Update   ai.ToolUpdateFunc
    Delete   ai.ToolDeleteFunc
    AssignGeo ai.ToolAssignGeoFunc
}
```

Each `toolCatalogDef` gets an optional `writer *toolWriter` field. `read`-only use cases leave it `nil`. The registry's `newAgentToolsRegistry` is updated to accept the per-tool services wired in `server.go`.

For each tool, the writer converts `map[string]any` to the typed `Create...Input` / `Update...Input` of the per-tool service, calls it, and converts the returned DTO back to `map[string]any`.

### 3.3. ai_tools catalog

A new migration adds one `ai_tools` row per action per use case:

- `visa_routes.create`, `visa_routes.update`, `visa_routes.delete`, `visa_routes.assign_geo`
- `housing_guides.create`, `housing_guides.update`, `housing_guides.delete`, `housing_guides.assign_geo`
- ... for all 28 catalog use cases

Each row:
- `category`: `agent_tools_<use_case>` or `agent_<domain>`
- `is_enabled`: `true`
- `required_permission`: mapped to existing permission node (e.g. `visa_routes.create`, `housing_guides.edit`, `scholarships.delete` — exact nodes per §03 / tool sections)
- `config` JSON contains `function_name`, `description`, `parameters`, `handler`, `requires_hitl: false`, `writes_user_data: true`

### 3.4. Handlers

`internal/ai/tools_register.go` registers the handlers:

```go
registry.RegisterHandler("visa_routes.create", UseCaseCreateHandler(deps, "visa_routes"))
registry.RegisterHandler("visa_routes.update", UseCaseUpdateHandler(deps, "visa_routes"))
...
```

`UseCaseCreateHandler` / `UseCaseUpdateHandler` / `UseCaseDeleteHandler` / `UseCaseAssignGeoHandler` are generic functions that look up the `toolWriter` for the named use case from `deps.ToolsRegistry` and execute it.

### 3.5. Permission and audit

- The executor's existing RBAC gate checks `required_permission` on each `ai_tools` row.
- Each successful write is logged to `ai_agent_tool_calls` and `ai_usage_log` by the executor.
- No HITL review (user request). The tool configs set `requires_hitl: false`.

### 3.6. No-drafting / real records

Create and update operations create live records (or update existing ones). They do not create `contents` drafts. The per-tool services already enforce their own status/validation rules.

### 3.7. Geo assignment

`assign_geo` is a special update that only mutates the geo-scoping fields (`destination_space_id`, `city_space_id`, `origin_space_id` as applicable). The payload contains the target space ID. The writer calls the per-tool `Update` service with the relevant fields.

### 3.8. Content-based use cases

Some use cases (story wall, achievement feed, service directory, jobs board, wiki content, ask-an-expat AMA) are implemented as `contents` rows, not tool tables. They are already covered by the existing `content.create`, `content.update`, `content.delete`, `content.publish` tools with the right `content_type`. This design does **not** add dedicated content-type CRUD tools unless explicitly requested.

## 4. Phased rollout

| Phase | Scope |
|-------|-------|
| 1 | Infrastructure: extend `ToolsRegistryService`, add `toolWriter` to `agent_tools_registry.go`, add generic `UseCase*Handler` functions in `internal/ai`, wire services in `server.go`. |
| 2 | Migration: seed `ai_tools` rows for all 28 use cases × 4 actions, and seed/add required RBAC permission nodes. |
| 3 | Implement writers for use cases with existing `Create/Update` services (visa_routes, housing_guides, scholarships, arrival_checklists, attestation, language_test_requirements, tax_obligation_guides, remittance, digest, mutual_aid, planning_groups, scam_reports, etc.). |
| 4 | Add missing `Delete` and `AssignGeo` service methods for tools that do not yet expose them, then wire them. |
| 5 | Verification: Playwright check of `/ai/tools`, `go test ./internal/server ./internal/ai`, and direct calls to the agent capabilities endpoint / `POST /api/v1/ai/agent`. |

## 5. Open questions / risks

1. **Permission nodes:** Many v2 tools may not have a full set of `*.create`, `*.edit`, `*.delete` permission nodes in the permissions table. Phase 2 must audit and seed any missing nodes.
2. **Delete methods:** Not all per-tool services currently have a `Delete` method. Phase 4 may require adding soft-delete logic.
3. **Global reference tables:** `credential_professions`, `cv_standards`, and `language_tests` are not tenant-scoped. Their create/update operations are global (admin-only), which may need a different permission model.
4. **Assign-geo fields:** Different tables use `destination_space_id`, `city_space_id`, or `origin_space_id`. The `assign_geo` writers need per-tool field mapping.

## 6. Non-scope

- This design does **not** enable the generic `tools.create`, `tools.update`, or `tools.assign_geo` disabled stubs. They remain disabled.
- This design does **not** add HITL review for these writes (per user request).
- This design does **not** add new content-type CRUD for story/achievement/service-listing/etc., which remain covered by `content.*` tools.
