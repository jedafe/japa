# Design — Global Template Literals with Admin CRUD

**Date:** 2026-08-30  
**Author:** Devin (via user request)  
**Status:** Approved for implementation  
**JEOS refs:** §14 (content type system), ADR-064 (geo-aware copy), Rule 16 (no hardcoded frontend data)

## Goal

Allow authors and admins to embed dynamic, context-aware placeholders in any platform text. A title like `How to Japa to {{Destination_Country}} from {{Source_Country}} in {{Current_Year}}` is stored with the placeholders and resolved at render time to `How to Japa to the UK from Nigeria in 2026`. Admins can create, update, and delete reusable template literals from the admin panel.

## Decisions from clarification

- **Syntax**: `{{...}}` for both simple tokens and expressions, e.g. `{{Current_Year}}`, `{{Current_Year + 1}}`, `{{adjective(Destination_Country)}}`. Inline fallback applies to bare tokens: `{{Current_Year|2026}}`. Full expressions should use `if(...)` for conditional fallbacks.
- **Scope**: Phase 1 covers content titles/bodies and page-builder blocks. Phase 2 covers email templates, menus, notifications, ads, and per-user deadline/countdown tokens.
- **Dynamic values**: Server-side resolution using a whitelisted expression evaluator. Values depend on current time, visitor geo, and user profile context.
- **Admin-created dynamic tokens**: Admins create tokens of `kind: "expression"` and write a whitelisted expression in the `value` field. They cannot upload arbitrary code, but they can combine built-in variables, functions, and operators.
- **Country adjectives**: Built-in `adjective(country)` function uses the existing country demonym data (`packages/landing-blocks/src/geo/generatedCountryTokens.ts`, ported to Go) so `adjective("France")` returns `French`, not `Franceian`.
- **Multi-tenant**: Token definitions are tenant-scoped; built-in system tokens are seeded per tenant but cannot be deleted.
- **Single-brace alias**: `{...}` (the user's original example) can be supported as a parser alias in a follow-up if needed. Phase 1 uses `{{...}}` only.

## Data model

### `template_literals` table

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | `gen_random_uuid()` |
| `tenant_id` | uuid | FK `tenants(id)` ON DELETE CASCADE |
| `key` | text | Token name, e.g. `Current_Year`. Unique per tenant. |
| `kind` | text | `static` or `expression` |
| `value` | text | Raw replacement for `static`; expression for `expression` |
| `fallback` | text | Fallback if the token/expression cannot resolve |
| `label` | text | Admin UI display name |
| `description` | text | Admin UI help text |
| `category` | text | `time`, `geo`, `arithmetic`, `custom`, etc. |
| `is_system` | bool | `true` for built-in tokens seeded by migrations |
| `is_active` | bool | Toggle without deleting |
| `created_at` | timestamptz | now() |
| `updated_at` | timestamptz | now() |
| `deleted_at` | timestamptz | Soft delete |

- Unique index: `(tenant_id, key)`
- List queries default to `deleted_at IS NULL AND is_active = true`

### Built-in system tokens (seeded)

| Key | Kind | Value |
|---|---|---|
| `Current_Year` | `expression` | `year(Now)` |
| `Current_Date` | `expression` | `format(Now, "YYYY-MM-DD")` |
| `Current_Time` | `expression` | `format(Now, "HH:mm")` |
| `Current_Timestamp` | `expression` | `format(Now, "YYYY-MM-DD HH:mm")` |
| `Source_Country` | `expression` | `Source_Country` (variable passthrough) |
| `Destination_Country` | `expression` | `Destination_Country` (variable passthrough) |
| `Source_Code` | `expression` | `Source_Code` |
| `Destination_Code` | `expression` | `Destination_Code` |
| `Tenant_Name` | `expression` | `Tenant_Name` |
| `Locale` | `expression` | `Locale` |

Admins can override `label`, `description`, `fallback`, `category`, and `is_active` on system tokens, but not `key`, `kind`, or `value`.

## Architecture

### Core package: `internal/literals`

The resolver is a pure Go package with no database dependencies on the hot path (it receives the token list and render context as arguments).

- `RenderContext` struct:
  - `Now time.Time`
  - `Locale string`
  - `SourceCountry string` and `SourceCode string`
  - `DestinationCountry string` and `DestinationCode string`
  - `TenantName string`
  - `TenantID pgtype.UUID`
  - The engine maps these fields to snake_case expression variables (`Source_Country`, `Destination_Code`, etc.).
- `Resolver` struct:
  - Holds a map of built-in variable values and function evaluators.
  - `Resolve(ctx context.Context, text string, rc RenderContext, tokens []TemplateLiteral) (string, error)`
- `EvalContext` used by the expression engine:
  - Whitelisted variables and functions only.
  - Depth/recursion limit (max 32).
  - Evaluation timeout (50ms per call).
  - No reflection, no file/network access, no unbounded loops.

### Expression engine

The expression engine is a small, purpose-built parser/evaluator in `internal/literals/expr`.

**Types**
- numbers (int64 / float64)
- strings
- booleans
- null

**Variables**
- `Now` — `time.Time`
- `Current_Year`, `Current_Date`, `Current_Time`, `Current_Timestamp` — built-in aliases derived from `Now`
- `Source_Country`, `Source_Code`, `Destination_Country`, `Destination_Code`
- `Tenant_Name`, `Locale`

**Functions**
- `adjective(country)` — country demonym, e.g. `adjective("Nigeria")` → `"Nigerian"`
- `year(date)`, `month(date)`, `day(date)`, `weekday(date)`
- `format(date, "...")` — Go `time.Format` layout or a small alias set (`"YYYY"`, `"MM"`, `"DD"`, etc.)
- `upper(str)`, `lower(str)`, `title(str)`
- `concat(a, b, ...)`
- `if(cond, then, else)`
- `len(str)`

**Operators**
- Arithmetic: `+`, `-`, `*`, `/`, `%`
- Comparison: `==`, `!=`, `<`, `>`, `<=`, `>=`
- String concatenation: `+` is string concat when both operands are strings, numeric addition otherwise. Explicit `concat()` is preferred.
- Parentheses for grouping.

**Token lookup in expressions**
A bare identifier like `Current_Year` first resolves to a built-in variable; if not found, it resolves to a tenant-defined `template_literals` row of `kind = "expression"` or `kind = "static"`. Unknown identifiers are treated as null (and the token is left unreplaced if no fallback is provided).

### Integration points (phase 1)

The resolver is called at the final serialization boundary, never inside the store.

1. **Content service**
   - `ResolveContent(ctx, content, rc)` applies the resolver to `Title`, `Body`, `BodyHtml`, `MetaTitle`, `MetaDescription`.
   - Called from content list/get handlers before JSON encoding.
   - If a field contains no `{{...}}` placeholders, the resolver returns the original string with no allocation.

2. **Page builder / landing blocks**
   - Refactor `packages/landing-blocks/src/geo/resolveGeoCopy.ts` to call the server-resolved token map for SSR.
   - For client-side rendering, the server returns a `literals` map alongside the page payload; the existing `resolveGeoCopy` function uses that map.
   - Existing `{{diaspora_phrase}}` and `{{country}}` tokens are mapped to the new `Destination_Demonym` / `Destination_Country` system tokens for consistency.

3. **Admin panel**
   - New route `/literals` under `apps/admin` using the `EntityPage` scaffold.
   - `EntityPageConfig` fields:
     - `key` (text)
     - `kind` (select: `static` / `expression`)
     - `value` (textarea)
     - `fallback` (text)
     - `label` (text)
     - `description` (textarea)
     - `category` (select or text)
     - `is_active` (checkbox)
   - Expression editor shows a token/function helper (similar to the i18n key picker).

### Integration points (phase 2)

- Email templates: resolve subject and body before sending.
- Menu labels, notification copy, ad copy, widget copy.
- Per-user deadline/countdown tokens backed by a new `user_template_triggers` table.

## API behavior

### Admin endpoints

All under the admin router with tenant context.

- `GET /admin/literals?q=&page=&pageSize=&sort=&dir=` — paginated, searchable, sortable list.
- `POST /admin/literals` — create.
- `GET /admin/literals/:id` — get by ID.
- `PATCH /admin/literals/:id` — update.
- `DELETE /admin/literals/:id` — soft delete.

### Validation

- `key` must match `[A-Za-z_][A-Za-z0-9_]*`.
- `key` cannot collide with existing system keys unless the row is the system token itself.
- `value` for `kind = "expression"` is validated server-side by parsing it before save.
- `key` for new tokens cannot shadow built-in variables (`Now`, `Source_Country`, etc.).

### Public endpoints

No new public endpoints. Content and page APIs return resolved text. A read-only endpoint like `GET /site/literals` can be added later to expose a token map for client-side rendering.

## UI changes

- `apps/admin/src/routes/literals/+page.svelte` — new `EntityPage`-based CRUD page.
- Update admin sidebar/navigation to include "Literals" under the Content section.
- Add token helper dropdown in the create/edit slide-over.
- No changes to public content authoring UI unless authors want a token picker; they can type `{{...}}` directly.

## Migrations

1. Migration (next sequential number) `create_template_literals`:
   - `up.sql`: Create `template_literals` table.
   - `down.sql`: Drop `template_literals` table.
   - Seed system tokens for the default tenant (`00000000-0000-0000-0000-000000000001`).
2. Migration (next sequential number) `template_literals_permission`:
   - `up.sql`: Insert `content.literals.manage` into `permissions`; grant to `admin` and `super_admin` for the default tenant.
   - `down.sql`: Remove the permission row.
3. (Optional) Migration (next sequential number) `port_geo_tokens`:
   - `up.sql`: Backfill `{{diaspora_phrase}}` usage in existing pages to use new `Destination_Demonym` / `Destination_Country` tokens.
   - `down.sql`: Restore the original tokens.

## Catalog updates

- `api-routes-catalog.md` — add `/admin/literals` CRUD routes.
- `permission-key-naming.md` — add `content.literals.manage`.
- `valkey-keys-catalog.md` — add `literals:{tenant_id}` cache key.
- `asynq-tasks.md` — none.
- `system-settings-catalog.md` — none.

## Caching

- Tenant token lists are cached in Valkey under `literals:{tenant_id}` with a 5-minute TTL.
- The resolver receives the cached list as an argument, so no store calls happen during per-request resolution.
- Cache is invalidated on `POST/PATCH/DELETE` of any literal for that tenant.

## Testing plan

- `internal/literals/expr_test.go` — unit tests for the expression parser: literals, operators, functions, fallbacks, errors (unknown function, division by zero, timeout).
- `internal/literals/resolver_test.go` — token lookup, system vs. custom tokens, unknown tokens, multi-tenant isolation.
- `internal/content/literals_test.go` — content title/body resolution with sample contexts.
- `apps/admin/src/routes/literals/page.test.ts` — `EntityPage` renders, create/edit/delete flows mocked.
- End-to-end: create a post with title `How to Japa to {{adjective(Destination_Country)}} from {{Source_Country}} in {{Current_Year}}`; assert the rendered title is correct.
- `go build ./...` and `go vet ./...` clean.
- Admin app typecheck/build clean.

## Open questions / none

All clarification questions resolved before this design was written.

## Phase 2 notes

- Extend resolver to email templates, menus, notifications, and ads.
- Add per-user deadline/countdown tokens with a `user_template_triggers` table and `Triggered_At` / `Expires_At` variables.
