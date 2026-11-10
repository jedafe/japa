# Persona Profile Editing & Economy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admins can edit a persona's full profile (name, username, avatar, cover, profession, location, website, education, interests, disclosure, geo, tips) via `PATCH /admin/personas/{id}` — admin modal now, `/u/[username]` affordance as Phase B.

**Architecture:** `UpdatePersonaInput` gains profile fields that ride the existing update tx and the same read-merge-write pattern as `UpsertPersonaByline`/`PATCH /users/me`. Media ids resolve to URLs via an injected `MediaResolver`. `tips_enabled` upserts `tip_settings` for the backing user inside the same tx.

**Spec:** `docs/superpowers/specs/2026-11-10-persona-attributed-interactions-design.md` (Part 2 + §1.7)

**Tech Stack:** Go + sqlc + postgres; apps/admin SvelteKit (`MediaPickerModal` exists); apps/main Phase B (`MediaLibraryModal`).

## Global Constraints

- Never hand-edit `internal/db/gen/**` — edit `db/queries/*.sql` → `make sqlc-gen`.
- All profile fields live on **`user_profiles`** (avatar/cover/profession/location/website/education/interests/bio/display_name) — never `users`.
- Read-merge-write inside the update tx via `GetUserProfileForUpdate` — same pattern as `UpdatePersona`'s existing byline block (`service.go:807-839`).
- Commit on `main` per task; conventional messages, no attribution trailers.

---

### Task 1: sqlc — profile read/write + economy columns on persona queries

**Files:**
- Modify: `internal/db/queries/personas.sql`

**Interfaces:**
- Produces: `gen.UpsertPersonaProfile` (replaces/extends `UpsertPersonaByline`), `GetAIPersonaByIDRow`/`ListAIPersonasRow` gaining `avatar_url, cover_url, avatar_media_id, header_media_id, profession, location, website_url, education, interests, tips_enabled, points_balance`.

- [ ] **Step 1: Extend `GetAIPersonaByID` and `ListAIPersonas`** (lines 36-, 74-)

Add to both SELECT lists (they already join `users u`, `user_profiles up`, `spaces gs`):

```sql
 up.avatar_url, up.cover_url, up.avatar_media_id, up.header_media_id,
 up.profession, up.location, up.website_url, up.education, up.interests,
 COALESCE(ts.tips_enabled, false) AS tips_enabled,
 COALESCE((SELECT SUM(wl.amount) FROM wallet_ledger wl
   WHERE wl.user_id = u.id AND wl.ledger_type = 'points'), 0)::bigint AS points_balance,
```

and add `LEFT JOIN tip_settings ts ON ts.owner_type = 'user' AND ts.owner_id = u.id`.

- [ ] **Step 2: Extend the byline upsert into a full profile upsert**

Replace `UpsertPersonaByline` (line 285) with:

```sql
-- name: UpsertPersonaProfile :exec
-- Persona profile upsert — same read-merge-write contract as
-- UpsertUserProfile: the caller merges request fields against the
-- row-locked profile first; NULL/empty assignments are unconditional
-- except where noted (interests NULL preserves, [] clears).
INSERT INTO user_profiles (user_id, display_name, bio, avatar_url, avatar_media_id, header_media_id, cover_url, location, website_url, education, profession, interests)
VALUES (@user_id, sqlc.narg(display_name), sqlc.narg(bio), sqlc.narg(avatar_url), sqlc.narg(avatar_media_id), sqlc.narg(header_media_id), sqlc.narg(cover_url), sqlc.narg(location), sqlc.narg(website_url), sqlc.narg(education), sqlc.narg(profession), sqlc.narg(interests))
ON CONFLICT (user_id) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    bio = EXCLUDED.bio,
    avatar_url = EXCLUDED.avatar_url,
    avatar_media_id = EXCLUDED.avatar_media_id,
    header_media_id = EXCLUDED.header_media_id,
    cover_url = EXCLUDED.cover_url,
    location = EXCLUDED.location,
    website_url = EXCLUDED.website_url,
    education = EXCLUDED.education,
    profession = EXCLUDED.profession,
    interests = COALESCE(EXCLUDED.interests, user_profiles.interests),
    updated_at = now();
```

(Keep `UpsertPersonaByline` only if other callers remain — grep for its
use; provisioning writes the byline through this path too. If provisioning
uses it, leave it and add `UpsertPersonaProfile` alongside.)

- [ ] **Step 3: Regenerate + build**

```bash
make sqlc-gen && go build ./internal/db/... ./internal/personas/...
```

- [ ] **Step 4: Commit** — `feat(personas): profile + economy columns on persona queries`

---

### Task 2: `PATCH /admin/personas/{id}` — profile fields + tips + media resolution

**Files:**
- Modify: `internal/personas/service.go` (UpdatePersonaInput ~line 604, UpdatePersona ~line 631-853, Service struct ~line 158), `internal/personas/handlers.go` (HandleUpdate input parsing), `internal/server/server.go` (~line 1676 personas service wiring), `internal/personas/service_update_test.go`

**Interfaces:**
- Consumes: `gen.UpsertPersonaProfile`, `gen.UpsertTipSettings`, `GetUserProfileForUpdate` (existing).
- Produces:
  - `UpdatePersonaInput` gains `AvatarURL, AvatarMediaID, CoverURL, HeaderMediaID, Profession, Location, WebsiteURL, Education *string; Interests *[]string; TipsEnabled *bool`
  - `(*Service).SetMediaResolver(r MediaResolver)` — same interface shape as `users.MediaResolver` (`ResolveMediaURL`, `ResolveMediaMIME`)
  - `PersonaDetail`/`PersonaListItem` gain `AvatarURL, CoverURL, Profession, Location, WebsiteURL, Education string/*string; Interests []string; TipsEnabled bool; PointsBalance int64`

- [ ] **Step 1: Failing tests** (`service_update_test.go` conventions)

```go
func TestUpdatePersona_ProfileFields(t *testing.T) {
	// PATCH {profession:"Nurse", location:"London", avatar_media_id:<uuid>}
	// → user_profiles row updated; media id resolved to URL when resolver wired.
}
func TestUpdatePersona_TipsEnabled(t *testing.T) {
	// PATCH {tips_enabled:true} → tip_settings row for ('user', backing) upserted.
}
func TestUpdatePersona_InvalidMediaID(t *testing.T) {
	// PATCH {avatar_media_id:"nope"} → ErrInvalid
}
```

- [ ] **Step 2: Run — FAIL** (`go test ./internal/personas/ -run TestUpdatePersona -v`)

- [ ] **Step 3: Implement**

`service.go`:

```go
// MediaResolver resolves a media UUID to a URL + MIME — same contract as
// users.MediaResolver; wired at server time (nil = store ids without URLs).
type MediaResolver interface {
	ResolveMediaURL(ctx context.Context, mediaID pgtype.UUID) (string, error)
	ResolveMediaMIME(ctx context.Context, mediaID pgtype.UUID) (string, error)
}

func (s *Service) SetMediaResolver(r MediaResolver) { s.mediaResolver = r }
```

`UpdatePersonaInput` — append the new fields; extend the `changed` map +
`identityTouched` condition to include them.

In `UpdatePersona`, replace the `in.Name != nil || in.Tagline != nil` byline
block (~line 807-839) with a merged profile block that:

1. Row-locks `GetUserProfileForUpdate(detail.UserID)` (existing code — keep).
2. Merges every touched field: `*string` nil → current value; `""` → clear
   (nil for text cols); `interests` nil → current, `[]` → `[]`.
3. `avatar_media_id`/`header_media_id`: `""` clears (zero uuid + clear url);
   else `mid.Scan` → `400` on bad uuid → `mediaResolver.ResolveMediaMIME` for
   an image-MIME check mirroring `users.checkImageMedia` (allowed types via
   `users.allowed_avatar_types` for avatar; header unrestricted) →
   `ResolveMediaURL` fills the `*_url` field when resolver present.
4. `q.UpsertPersonaProfile(...)` with the merged params.
5. `tips_enabled`: inside the same tx —
   `q.UpsertTipSettings(owner_type:'user', owner_id:detail.UserID,
   tips_enabled:*in.TipsEnabled, accept_credits:true, accept_fiat:false,
   accept_external:false, min_tip_amount:<platform default>)`.
   First-upsert defaults: credits only (fiat/external need provider config
   the admin can add later via the tips UI).

`handlers.go` `HandleUpdate` — decode the new fields into `UpdatePersonaInput`
(mirror how `GeoSpace *string`/`Name *string` are decoded today).

`server.go` — after `personasService` construction: `personasService.SetMediaResolver(mediaService)` using the same resolver instance wired into `usersService` (find the existing `usersService.MediaResolver = ...` line and reuse).

`PersonaDetail`/`PersonaListItem` + `personaDetailFromRow`/`personaListItemFromRow` — map the new columns.

- [ ] **Step 4: Run — PASS** (`go test ./internal/personas/`)

- [ ] **Step 5: Commit** — `feat(personas): profile fields + tips_enabled on PATCH /admin/personas/{id}`

---

### Task 3: Admin personas edit modal (Phase A)

**Files:**
- Modify: `apps/admin/src/routes/(app)/personas/+page.svelte` (UpdateInput type, edit modal fields), media via `apps/admin/src/lib/components/ui/MediaPickerModal.svelte`

**Interfaces:**
- Consumes: PATCH response fields from Task 2; existing `editGeoSpace`/`editGeoCleared` flow is the model for the new fields.

- [ ] **Step 1: Extend `UpdateInput` + edit state**

```ts
// add to UpdateInput + the edit form state
avatar_media_id?: string; header_media_id?: string;
avatar_url?: string; cover_url?: string;
profession?: string; location?: string; website_url?: string;
education?: string; interests?: string[]; tips_enabled?: boolean;
```

Seed from `GetPersona` response when opening the edit modal (same place
`editGeoSpace`/`editTagline` are seeded today).

- [ ] **Step 2: Modal fields**

- Avatar + cover: `MediaPickerModal` buttons showing the current
  `avatar_url`/`cover_url` thumbnail; on pick → set `*_media_id`; "Remove"
  sends `""` (clear path).
- Text inputs: `profession`, `location`, `website_url`, `education`;
  `interests` chips input (check `apps/admin` for an existing chips/tag
  input component — reuse if present; else comma-separated input → `string[]`).
- "Accept tips" checkbox bound to `tips_enabled`; read-only display of
  `points_balance` in the modal header area.

- [ ] **Step 3: Verify** — `cd apps/admin && bunx svelte-check`; manual edit round-trip.

- [ ] **Step 4: Commit** — `feat(admin): persona profile fields + tips in edit modal`

---

### Task 4: Phase B — `/u/[username]` "Edit persona" affordance

**Files:**
- Modify: `apps/main/src/routes/(app)/u/[username]/` profile page, `apps/main/src/lib/api/personas.ts` (reuse client from Plan A Task 12), possibly `internal/users` profile payload (needs an `is_persona`/`persona_id` flag for the affordance check)

**Interfaces:**
- Consumes: `PATCH /admin/personas/{id}` (Task 2), viewer `persona.manage` permission (same mechanism page data exposes permissions today).

- [ ] **Step 1:** profile payload — check `GET /users/{username}` (or profile endpoint) for a bot/persona flag; add `persona_id` to the public profile payload when `u.is_bot` (query: join `ai_personas` on `user_id`) — implement in the profile read query + payload.
- [ ] **Step 2:** frontend — when `persona_id` present AND viewer has `persona.manage`, render "Edit persona" in the profile header menu; opens an editor sheet with the same fields as the admin modal (reuse `MediaLibraryModal` for media; submit `PATCH /admin/personas/{persona_id}`).
- [ ] **Step 3:** `bunx svelte-check`; commit — `feat(main): edit-persona affordance on persona profiles`

---

### Task 5: JEOS bookkeeping

- [ ] Update `.jeos/conventions/api-routes-catalog.md` (PATCH field list), `.jeos/sections/68-ai-personas.md` (profile-field + `tips_enabled` notes), `.jeos/sections/125-donations-tips.md` (persona `tips_enabled` note). Commit — `docs(jeos): persona profile fields + tips_enabled catalog updates`

---

## Post-implementation verification

- `go test ./internal/personas/ ./internal/tips/ ./internal/users/`
- `cd apps/admin && bunx svelte-check`
- Manual: PATCH with avatar_media_id → profile shows avatar; tips_enabled → persona `/u/[username]` shows tip CTA (PublicUserTipConfig flips); points_balance on detail.
