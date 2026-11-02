# Design — Geo Spaces for AI Personas

**Date:** 2026-11-02
**Status:** Approved (design review)
**Refs:** `.jeos/sections/68-ai-personas.md`, `.jeos/sections/05-geo-first-space-system.md`, ADR-068, ADR-064a

## Problem

A real user's profile carries geo identity: `space_members` rows for the
country → state → city chain (surfaced in the profile's "Spaces" sidebar) and
`users.current_geo_space_id` (the `is_primary` badge plus ADR-064a geo
personalization). Onboarding's `autoJoinGeoSpaces` produces both in one
transaction.

A persona's backing bot account gets neither — persona profiles render
geo-less, which breaks the "persona looks like a real member" contract.

## Goal

Persona profiles get a geo space like real profiles: membership in a geo
space chain plus `current_geo_space_id`, assignable at provisioning time and
changeable later via the existing admin PATCH endpoint, with an admin UI
picker.

## API contract

### `POST /api/v1/admin/personas` (mass provision)

`PersonaInput` gains one optional field:

```jsonc
{
  "name": "Amina Lagos",
  "slug": "amina-lagos",
  // ...existing fields...
  "geo_space": "lagos"            // space UUID or slug; optional
}
```

- Empty/absent → no geo assignment (current behavior, backward compatible).
- Resolution order: parse as UUID → `GetSpaceByID`; else `GetSpaceBySlug`.
- Validation: space must exist, not be soft-deleted, `status = 'active'`, and
  `level ∈ {country, state, city}` — the same gate as
  `users.Store.isValidGeoSpace`. Violations → `ErrInvalid` → 400
  `validation_error` (fails the whole batch; provisioning stays
  all-or-nothing per ADR-068 §7).

### `PATCH /api/v1/admin/personas/{id}`

Body gains `geo_space?: string`:

- Omitted (`null`) → unchanged.
- `""` → clear: geo-level memberships removed, `current_geo_space_id` → NULL.
- Value → set/replace: geo-level `space_members` rows outside the new chain
  are **deleted** (replace semantics — approved decision, unlike real users
  who accumulate), then the new chain is joined and `current_geo_space_id`
  set.

### Response shapes

`PersonaDetail` and `PersonaListItem` gain:

```jsonc
"geo_space_id":    "uuid-or-empty",
"geo_space_name":  "Lagos",        // omitted when unset
"geo_space_slug":  "lagos",        // omitted when unset
"geo_space_level": "city"          // omitted when unset
```

Read through `users.current_geo_space_id → spaces` via LEFT JOINs added to
`GetAIPersonaByID` and `ListAIPersonas` in `internal/db/queries/personas.sql`.

## Backend internals (`internal/personas`)

### Helpers

```go
// resolveGeoSpace resolves a UUID-or-slug ref to an active geo space row.
func (s *Service) resolveGeoSpace(ctx, q, ref string) (spaceRow, error)

// applyGeoSpace joins userID to space + geo ancestors and sets
// users.current_geo_space_id. Runs inside the caller's tx.
func (s *Service) applyGeoSpace(ctx, q, userID, spaceID pgtype.UUID) error
```

`applyGeoSpace` uses existing generated queries:

- `ListSpaceAncestors(spaceID)` — walks `parent_id ∪ space_parent_spaces`
  (≤3 hops); keep only ancestors with `level ∈ {country, state, city}`.
- `AddSpaceMember` — idempotent (`ON CONFLICT … DO UPDATE status='approved'`).
- `UpdateUserGeoSpace` — sets `current_geo_space_id`.

### New sqlc query

```sql
-- name: RemoveUserGeoMembershipsExcept :exec
-- Replace-semantics for persona geo assignment: drop the user's memberships
-- in geo-level spaces (country/state/city) that are not in the keep list.
DELETE FROM space_members sm
USING spaces s
WHERE sm.space_id = s.id
  AND sm.user_id = @user_id
  AND s.level IN ('country','state','city')
  AND s.id <> ALL(@keep_ids::uuid[]);
```

`space_members.member_count` self-corrects via the existing trigger
(migration 000967). Clearing is handled by `UpdateUserGeoSpace` itself —
its param is `pgtype.UUID`, so `Valid:false` writes NULL; no separate
clear query is needed.

### Wiring

- `ProvisionPersonas`: after the profile byline upsert and before the
  `ai_personas` row, apply geo for inputs with `geo_space != ""` — inside the
  same tx (partial provision remains impossible).
- `UpdatePersona`: `geo_space` rides the existing identity-touched tx path
  (`identityTouched || in.GeoSpace != nil`) and is added to the audit
  `fields` list. Geo replace runs before `UpdateAIPersona` commits — a failed
  replace rolls back the whole update.
- Validation errors → `errpkg.ErrInvalid` → 400 `validation_error`
  (consistent with existing mode/status validation).

## Admin UI (`apps/admin/src/routes/(app)/personas/+page.svelte`)

- **Provision form:** a cascading country → state → city picker per persona
  row (new shared `GeoSpaceCascade` component) driven by the existing
  `geoApi` (`/geo/countries`, `/geo/countries/{code}/states`,
  `/geo/states/{id}/cities`) — the same public endpoints real users get for
  location selection; the deepest selection's space UUID becomes
  `geo_space`. (Review note: `SpaceAutocomplete` was rejected — it relies on
  `/admin/spaces`, which ignores `level` server-side and requires the
  `spaces.admin` permission a persona manager may not hold.)
- **Edit form:** current geo space shown as text plus the same cascade to
  change it; sending `""` clears.
- **List:** a Geo column rendering `geo_space_name` (+ level).

## Error handling

| Condition | Result |
|---|---|
| Unknown slug/UUID, soft-deleted space, inactive space, non-geo level | 400 `validation_error` |
| Geo resolution/apply fails mid-provision | tx rollback → 422 `provisioning_failed` (nothing committed) |
| Geo replace fails mid-update | tx rollback → existing `handleServiceError` mapping |

## Testing

Extend `internal/personas` tests:

- Provision with `geo_space` slug → backing user is member of the space +
  geo ancestors; `current_geo_space_id` = deepest selected space.
- Provision with non-geo/unresolved ref → 400, nothing committed.
- PATCH `geo_space` → new chain joined, old geo memberships removed.
- PATCH `geo_space: ""` → geo memberships removed, `current_geo_space_id` NULL.
- Detail/list responses expose the four `geo_space_*` fields.

## Explicitly out of scope

- `destination_country` custom-field parity (onboarding sets it; not part of
  "profile has a geo space").
- VisitorContext cache invalidation (bot accounts never resolve a
  VisitorContext).
- Frontend main-app changes — the profile Spaces sidebar already renders
  from `space_members`; no UI work needed there.
