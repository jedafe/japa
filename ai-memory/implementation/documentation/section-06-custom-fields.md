# Section 06 - Universal Custom Field Architecture: Implementation Complete

**Date:** 2026-06-25
**Phase:** 2
**Section:** 06
**Status:** Complete

## Overview

Section 06 implements the universal custom field system that underlies every
entity on the JapaConnect platform. Space Types, Content Types, User Profiles,
Organization Profiles, Forms, Tags, and Tickets all pull from the same
`custom_field_groups` + `custom_fields` tables. Every field is RBAC-scoped per
User Group — visibility, editability, and filterability are all configurable
without a code deploy.

This section also introduced sqlc code generation to the project, replacing
hand-written SQL with type-safe generated queries. A platform embed URL
resolver validates `platform_embed` field values at write time across eight
supported platforms. A Valkey (go-redis) client wrapper was added with graceful
degradation, ready for the editor cache and edit-lock functionality in later
sections.

## Components Implemented

### Migration 000008 — Custom Field Architecture

9 tables created:

1. **custom_field_groups** — Container for related fields, tenant-scoped, unique on `(tenant_id, slug)`
2. **custom_fields** — Individual field definitions, unique on `(group_id, key)`, 16 supported types
3. **custom_field_group_assignments** — Attaches a group to any entity class, unique on `(group_id, entity_class, entity_id)`
4. **custom_field_permissions** — RBAC per field per User Group, composite PK `(field_id, user_group_id)`
5. **custom_field_values** — Polymorphic value storage, unique on `(field_id, entity_type, entity_id)`
6. **forms** — Tenant-scoped forms using the same custom_fields infrastructure
7. **form_steps** — Multi-step form support, unique on `(form_id, position)`
8. **form_submissions** — Submission snapshots with `field_values` JSONB, IP tracking
9. **form_submission_triggers** — Workflow triggers on form submission

Seed data:

- 7 system field groups (deterministic UUIDs 040-046): user_profile_fields,
  blog_post_fields, forum_fields, space_fields, university_fields,
  organization_fields, tag_fields
- 2 system forms (ID 047 Feedback, ID 048 Report)
- Japa Status select field (ID 049) with 8 options covering the diaspora journey
- user_profile group assignment (entity_id NULL = applies to all user_profile entities)

Conventions: `tenant_id` FK with `ON DELETE CASCADE`, `gen_random_uuid()` for
primary keys, `UNIQUE` constraints for natural keys, `idx_` prefixed indexes,
deterministic UUIDs for system seeds, `is_system` flag protecting seeded rows
from API deletion.

### sqlc Code Generation

Configured sqlc with the following settings (`sqlc.yaml`):

- Engine: `postgresql`, SQL package: `pgx/v5`
- Queries in `internal/db/queries/`, generated output in `internal/db/gen/`
- Package name `gen` (chosen over `generated` to avoid import conflicts with
  `internal/db`)
- `emit_pointers_for_null_types: true` and `emit_json_tags: true`

34 queries across 2 SQL files:

- `internal/db/queries/custom_fields.sql` — 21 queries (field groups, fields,
  assignments, permissions, values)
- `internal/db/queries/forms.sql` — 13 queries (forms, steps, submissions)

4 generated Go files:

- `internal/db/gen/models.go` — struct definitions for all 9 tables
- `internal/db/gen/db.go` — `DBTX` interface and `Queries` struct
- `internal/db/gen/custom_fields.sql.go` — generated query methods for custom fields
- `internal/db/gen/forms.sql.go` — generated query methods for forms

All generated types use `pgtype.UUID`, `pgtype.Text`, `pgtype.Numeric`, and
`pgtype.Timestamptz` as appropriate. The `DBTX` interface is satisfied by
`*pgxpool.Pool`, enabling the reader/writer pool split.

### internal/customfields Package (6 files, 2,731 lines)

**customfields.go** (81 lines)

- 16 field type constants: text, textarea, number, boolean, date, datetime,
  select, multiselect, url, email, phone, media, rich_text, platform_embed,
  color, rating
- `FieldOption` struct for select/multiselect options
- `validFieldTypes` map with `IsValidFieldType` helper
- `SupportedEmbedPlatforms` list (8 platforms)
- `Service` struct with reader/writer `*gen.Queries` pool split
- `NewService` constructor

**store.go** (430 lines)

- 34 `Service` methods wrapping sqlc-generated queries across 8 entity groups:
  field groups (6), custom fields (5), assignments (3), permissions (3),
  field values (4), forms (6), form steps (3), form submissions (4)
- Read methods use `s.read`, write methods use `s.write`
- All errors wrapped with `fmt.Errorf` for context

**embed.go** (459 lines)

- `EmbedData` struct with Platform, URL, EmbedURL, Title, Author,
  ThumbnailURL, HTML, and Raw fields
- `EmbedResolver` with injectable HTTP client and oEmbed URL overrides
- `NewEmbedResolver` (10-second timeout production client)
- `newEmbedResolverWithOverrides` (unexported, for test mocks)
- Platform identification by hostname (strips `www.` prefix)
- 8 supported platforms with three resolution strategies:
  - **Pure URL parsing** (no HTTP): GitHub, Figma, Google Maps
  - **oEmbed only** (hard-fail on error): Twitter/X, Vimeo, Spotify, CodePen
  - **Hybrid** (iframe + oEmbed enrichment, soft-fail): YouTube
- YouTube video ID extraction from 4 URL shapes: `watch?v=`, `youtu.be/`,
  `/embed/`, `/shorts/`

**handlers.go** (828 lines)

- `Handlers` struct with `*Service` and `*EmbedResolver` dependencies
- 19 HTTP handlers: field groups (5), fields (4), field values (2),
  permissions (1), forms (5), form submissions (2)
- Package-level helpers: `writeJSON`, `writeError`, `parseUUIDParam`,
  `clientIP`, `requireTenant`, `handleServiceError`
- `handleServiceError` maps `pgx.ErrNoRows` to 404, duplicate errors to 409,
  other errors to 500
- Platform embed validation at write time: `SetFieldValue` resolves the URL
  against the field's `embed_platforms` before persisting
- `SubmitForm` is public (optional auth), records submitter when present,
  captures client IP for abuse tracking

**embed_test.go** (747 lines)

- 28 test functions covering all 8 platforms, edge cases, and no-network
  verification
- Table-driven tests for YouTube URL shapes, platform identification, allowed
  platform filtering, and YouTube ID extraction
- Network-free verification tests confirm GitHub, Figma, and Google Maps make
  no HTTP calls
- Mock oEmbed server via `httptest.NewServer` with override-based injection
- `x.com` treated as alias for `twitter.com`

**customfields_test.go** (186 lines)

- 9 test functions for field type validation, JSON helpers, UUID parsing,
  handler request validation, and embed platform list integrity
- Handler tests verify 400 responses for missing fields, invalid bodies,
  invalid field types, and invalid form submissions

### internal/valkey Package (2 files, 177 lines)

**client.go** (100 lines)

- `Client` wrapper around `*redis.Client` (go-redis v9)
- `New(ctx, addr)` — fail-fast constructor with immediate Ping, returns error
  if server unreachable
- `NewWithFallback(addr)` — graceful degradation constructor, returns non-nil
  client even when server is down, 3-second dial timeout
- `Ping(ctx)` — verifies server responsiveness
- `Close()` — releases connection pool, safe on unreachable clients
- `Redis()` — returns underlying `*redis.Client` for direct access
- All keys must be namespaced with `jc:` prefix (documented, not enforced)

**client_test.go** (77 lines)

- 6 tests using `localhost:1` (guaranteed unreachable) with 2-second context
  timeout
- Tests cover: fallback returns non-nil client, ping fails on unreachable,
  New returns error on unreachable, Close does not panic, Close is idempotent,
  Redis() returns same instance

### Server Wiring

- RBAC resolver (`rbac.New`) and store (`rbac.NewStore`) created in `NewServer`
- `bootstrapPermissions` function runs at startup with a 10-second timeout
- 11 permission keys registered via `rbacStore.RegisterPermission` (idempotent)
- `super_admin_role` created with `ON CONFLICT DO NOTHING`
- All 11 permissions assigned to the role
- Role granted to `super_admin` system group
- All bootstrap operations are idempotent; failures are logged but never crash
  the server
- `customfields.NewService` and `customfields.NewEmbedResolver` created and
  wired into `customfields.NewHandlers`
- Valkey client created with `NewWithFallback` (graceful degradation), retained
  for process lifetime, not yet wired into routes

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/config | 4 | PASS |
| internal/server | 8 | PASS |
| internal/tenant | 3 | PASS |
| internal/auth | 22 | PASS |
| internal/rbac | 4 | PASS |
| internal/features | 5 | PASS |
| internal/geoip | 6 | PASS |
| internal/valkey | 6 | PASS |
| internal/customfields | 36 | PASS |
| **Total** | **94** | **ALL PASS** |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./...` | ok - all packages pass |
| `go build ./cmd/api/` | success |
| `go build ./cmd/migrate/` | success |
| `go vet ./...` | clean |

## API Routes Added

19 routes registered under `/api/v1`, all within the tenant-scoped route group:

| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| GET | `/custom-field-groups` | Required | — | List field groups |
| GET | `/custom-field-groups/{id}` | Required | — | Get group with fields |
| GET | `/custom-field-groups/{id}/fields` | Required | — | List fields in group |
| POST | `/custom-field-groups` | Required | `custom_fields.group.create` | Create group |
| PATCH | `/custom-field-groups/{id}` | Required | `custom_fields.group.edit` | Update group |
| DELETE | `/custom-field-groups/{id}` | Required | `custom_fields.group.delete` | Delete group |
| POST | `/custom-field-groups/{id}/fields` | Required | `custom_fields.field.create` | Create field |
| PATCH | `/custom-fields/{id}` | Required | `custom_fields.field.edit` | Update field |
| DELETE | `/custom-fields/{id}` | Required | `custom_fields.field.delete` | Delete field |
| PUT | `/custom-fields/{id}/permissions` | Required | `custom_fields.field.edit` | Set field permission |
| GET | `/custom-field-values/{entity_type}/{entity_id}` | Required | — | List values for entity |
| PUT | `/custom-field-values` | Required | `custom_fields.values.set` | Set field value |
| GET | `/forms` | Required | — | List forms |
| GET | `/forms/{id}` | Required | — | Get form with steps |
| POST | `/forms` | Required | `forms.create` | Create form |
| PATCH | `/forms/{id}` | Required | `forms.edit` | Update form |
| DELETE | `/forms/{id}` | Required | `forms.delete` | Delete form |
| POST | `/forms/{id}/submissions` | Optional | — | Submit form (public) |
| GET | `/forms/{id}/submissions` | Required | `forms.submissions.view` | List submissions |

## Permission Keys Registered

11 permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `custom_fields.group.create` | Create custom field groups |
| `custom_fields.group.edit` | Edit custom field groups |
| `custom_fields.group.delete` | Delete custom field groups |
| `custom_fields.field.create` | Create custom fields |
| `custom_fields.field.edit` | Edit custom fields |
| `custom_fields.field.delete` | Delete custom fields |
| `custom_fields.values.set` | Set custom field values |
| `forms.create` | Create forms |
| `forms.edit` | Edit forms |
| `forms.delete` | Delete forms |
| `forms.submissions.view` | View form submissions |

All 11 permissions are assigned to the `super_admin_role`, which is granted to
the `super_admin` system group. Every operation is idempotent.

## Files Created

| # | File | Purpose |
|---|------|---------|
| 1 | `migrations/000008_custom_field_architecture.sql` | 9 tables, seed data, indexes |
| 2 | `sqlc.yaml` | sqlc configuration (pgx/v5, package gen) |
| 3 | `internal/db/queries/custom_fields.sql` | 21 sqlc queries for custom fields |
| 4 | `internal/db/queries/forms.sql` | 13 sqlc queries for forms |
| 5 | `internal/db/gen/models.go` | Generated struct definitions |
| 6 | `internal/db/gen/db.go` | Generated DBTX interface and Queries struct |
| 7 | `internal/db/gen/custom_fields.sql.go` | Generated custom field query methods |
| 8 | `internal/db/gen/forms.sql.go` | Generated form query methods |
| 9 | `internal/customfields/customfields.go` | Field types, Service struct, NewService |
| 10 | `internal/customfields/store.go` | 34 Service methods wrapping sqlc queries |
| 11 | `internal/customfields/embed.go` | Platform embed URL resolver (8 platforms) |
| 12 | `internal/customfields/handlers.go` | 19 HTTP handlers with helpers |
| 13 | `internal/customfields/embed_test.go` | 28 embed resolver tests |
| 14 | `internal/customfields/customfields_test.go` | 9 validation and handler tests |
| 15 | `internal/valkey/client.go` | Valkey client wrapper with graceful degradation |
| 16 | `internal/valkey/client_test.go` | 6 client connection tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added customfields, valkey, rbac imports; RBAC resolver/store; bootstrapPermissions function; 19 custom field and form routes; Valkey client creation |
| 2 | `go.mod` | Added `github.com/redis/go-redis/v9 v9.21.0` |
| 3 | `go.sum` | Updated checksums for go-redis dependency |

## Known Issues

- **sqlc cursor pagination type inference:** The `ListFormSubmissionsByForm`
  query uses scalar comparisons with an `IS NULL` guard instead of tuple
  comparison `(created_at, id) < (cursor_created_at, cursor_id)` to work around
  a sqlc type-inference quirk. The generated `CursorCreatedAt` and `CursorID`
  param types may not perfectly match the expected `pgtype` types, but pgx
  handles encoding correctly at runtime by type.
- **Valkey client not yet wired into routes:** The client is created with
  `NewWithFallback` and retained for the process lifetime, but no routes
  currently use it. Infrastructure is ready for Sections 15 (Block Editor
  TipTap) and 45 (Post Create Page) which will use it for editor cache and
  edit locks.
- **Asynq task interfaces stubbed:** `space:auto_populate` and
  `scheduled_publish` background tasks are deferred to Phase 6. The
  `form_submission_triggers` table is in place but no trigger processing logic
  exists yet.

## Next Steps

Section 05 — Geo-First Space System. This section depends on Section 06:
`space_types.field_group_id` references `custom_field_groups`, so the custom
field architecture must be in place before space types can be defined.
